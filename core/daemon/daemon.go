package daemon

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/islandweave/islandweave/apps/gateway"
	"github.com/islandweave/islandweave/core/beacon"
	"github.com/islandweave/islandweave/core/identity"
	"github.com/islandweave/islandweave/core/objects"
	"github.com/islandweave/islandweave/core/services"
	"github.com/islandweave/islandweave/core/storage"
	"github.com/islandweave/islandweave/core/transport"
	"github.com/islandweave/islandweave/protocols/iwtp"
)

// Config configures an islandd instance.
type Config struct {
	DataDir                  string
	NodeName                 string
	ListenAddr               string // P2P TCP address, e.g. "0.0.0.0:47472" or "127.0.0.1:0"
	ControlAddr              string // Local control API address, e.g. "127.0.0.1:47471"
	GatewayAddr              string // Local browser gateway address, e.g. "127.0.0.1:47475"
	MulticastAddr            string // UDP multicast address, default "239.255.73.87:47470"
	DisableBeacon            bool   // Optional flag for isolated unit tests
	DisableCourierForwarding bool   // Disable intermediate DTN bundle forwarding
	MaxCourierBytes          int64  // Courier storage quota in bytes (default 64 MiB)
}

// Daemon is the core IslandWeave networking engine (islandd).
type Daemon struct {
	cfg          Config
	id           *identity.NodeIdentity
	store        *storage.Store
	objStore     *objects.Store
	beacon       *beacon.Service
	gw           *gateway.Server
	controlToken string

	p2pLn     net.Listener
	controlLn net.Listener
	httpSrv   *http.Server

	mu    sync.RWMutex
	links map[string]*transport.Link

	waitMu     sync.Mutex
	objWaiters map[string]chan iwtp.ObjDataPayload
	svcWaiters map[string]chan []services.ServiceRecord

	runCtx context.Context
	cancel context.CancelFunc
}

// New creates and initializes a Daemon (without starting network listeners yet).
func New(cfg Config) (*Daemon, error) {
	if cfg.DataDir == "" {
		return nil, errors.New("data_dir is required")
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = "0.0.0.0:47472"
	}
	if cfg.ControlAddr == "" {
		cfg.ControlAddr = "127.0.0.1:47471"
	}
	if cfg.GatewayAddr == "" {
		cfg.GatewayAddr = "127.0.0.1:47475"
	}
	if cfg.MulticastAddr == "" {
		cfg.MulticastAddr = beacon.DefaultMulticastAddr
	}

	id, _, err := identity.LoadOrCreateNodeIdentity(cfg.DataDir, cfg.NodeName)
	if err != nil {
		return nil, fmt.Errorf("node identity: %w", err)
	}
	store, err := storage.Open(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	objStore, err := objects.NewStore(cfg.DataDir)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("open object store: %w", err)
	}
	token, err := loadOrCreateToken(cfg.DataDir)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("control token: %w", err)
	}

	d := &Daemon{
		cfg:          cfg,
		id:           id,
		store:        store,
		objStore:     objStore,
		controlToken: token,
		links:        make(map[string]*transport.Link),
		objWaiters:   make(map[string]chan iwtp.ObjDataPayload),
		svcWaiters:   make(map[string]chan []services.ServiceRecord),
	}
	d.gw = gateway.New(store, objStore, d.FetchSite)
	return d, nil
}

// Start binds the P2P listener, Beacon service, and local Control API.
func (d *Daemon) Start(ctx context.Context) error {
	p2pLn, err := net.Listen("tcp", d.cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen p2p %s: %w", d.cfg.ListenAddr, err)
	}
	d.p2pLn = p2pLn

	tcpAddr := p2pLn.Addr().(*net.TCPAddr)
	d.beacon = beacon.NewService(d.id, beacon.Config{
		Mode:          beacon.ModeSilent,
		Interval:      3 * time.Second,
		TTL:           30 * time.Second,
		ListenPort:    tcpAddr.Port,
		MulticastAddr: d.cfg.MulticastAddr,
	}, d.onPeerDiscovered)

	ctrlLn, err := net.Listen("tcp", d.cfg.ControlAddr)
	if err != nil {
		_ = p2pLn.Close()
		return fmt.Errorf("listen control %s: %w", d.cfg.ControlAddr, err)
	}
	d.controlLn = ctrlLn

	_ = os.WriteFile(filepath.Join(d.cfg.DataDir, "control.addr"), []byte(ctrlLn.Addr().String()), 0o600)
	_ = os.WriteFile(filepath.Join(d.cfg.DataDir, "islandd.pid"), []byte(fmt.Sprintf("%d", os.Getpid())), 0o600)

	runCtx, cancel := context.WithCancel(ctx)
	d.runCtx = runCtx
	d.cancel = cancel

	if !d.cfg.DisableBeacon {
		if err := d.beacon.Start(runCtx); err != nil {
			_ = ctrlLn.Close()
			_ = p2pLn.Close()
			cancel()
			return fmt.Errorf("start beacon: %w", err)
		}
	}

	mux := http.NewServeMux()
	d.registerRoutes(mux)
	d.httpSrv = &http.Server{
		Handler:           d.authMiddleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	if err := d.gw.Start(d.cfg.GatewayAddr); err != nil {
		_ = ctrlLn.Close()
		_ = p2pLn.Close()
		cancel()
		return fmt.Errorf("start gateway %s: %w", d.cfg.GatewayAddr, err)
	}
	_ = os.WriteFile(filepath.Join(d.cfg.DataDir, "gateway.addr"), []byte(d.gw.Addr()), 0o600)

	go d.acceptLoop(runCtx)
	go func() {
		_ = d.httpSrv.Serve(ctrlLn)
	}()

	return nil
}

// Stop gracefully shuts down all listeners, links, and storage.
func (d *Daemon) Stop() error {
	if d.cancel != nil {
		d.cancel()
	}
	if d.beacon != nil {
		d.beacon.Stop()
	}
	if d.gw != nil {
		d.gw.Stop()
	}
	if d.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = d.httpSrv.Shutdown(ctx)
		cancel()
	}
	if d.p2pLn != nil {
		_ = d.p2pLn.Close()
	}
	d.mu.Lock()
	for id, link := range d.links {
		_ = link.Close()
		delete(d.links, id)
	}
	d.mu.Unlock()

	_ = os.Remove(filepath.Join(d.cfg.DataDir, "islandd.pid"))
	return d.store.Close()
}

func (d *Daemon) NodeID() string      { return d.id.ID }
func (d *Daemon) NodeName() string    { return d.id.Name }
func (d *Daemon) P2PAddr() string     { return d.p2pLn.Addr().String() }
func (d *Daemon) ControlAddr() string { return d.controlLn.Addr().String() }
func (d *Daemon) GatewayAddr() string { return d.gw.Addr() }
func (d *Daemon) Token() string       { return d.controlToken }

func (d *Daemon) onPeerDiscovered(dp beacon.DiscoveredPeer, pub ed25519.PublicKey) {
	var isl []string
	if dp.Announcement.IslandID != "" {
		isl = []string{dp.Announcement.IslandID}
	}
	_ = d.store.UpsertPeer(&storage.PeerRecord{
		NodeID:    dp.Announcement.NodeID,
		Name:      dp.Announcement.NodeName,
		PublicKey: pub,
		Addrs:     []string{dp.DialAddr},
		Islands:   isl,
		LastSeen:  dp.SeenAt,
		Source:    "beacon",
	})
}

func (d *Daemon) currentIslandIDs() []string {
	list, err := d.store.ListIslands()
	if err != nil {
		return nil
	}
	ids := make([]string, 0, len(list))
	for _, isl := range list {
		ids = append(ids, isl.ID)
	}
	return ids
}

func (d *Daemon) authorizePeer(peerID string, _ ed25519.PublicKey, sharedIslands []string) bool {
	p, err := d.store.GetPeer(peerID)
	if err == nil {
		if p.Revoked {
			return false
		}
		if p.Trusted {
			return true
		}
	}
	// Allow if nodes share at least one Island membership.
	return len(sharedIslands) > 0
}

func (d *Daemon) acceptLoop(ctx context.Context) {
	for {
		conn, err := d.p2pLn.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				continue
			}
		}
		go d.handleInbound(ctx, conn)
	}
}

func (d *Daemon) handleInbound(ctx context.Context, conn net.Conn) {
	link, err := transport.HandshakeResponder(conn, transport.HandshakeConfig{
		Identity:      d.id,
		Capabilities:  []string{transport.DefaultCapabilityMsg},
		IslandIDs:     d.currentIslandIDs(),
		AuthorizePeer: d.authorizePeer,
	})
	if err != nil {
		_ = conn.Close()
		return
	}
	d.registerAndServeLink(ctx, link, "inbound")
}

// ConnectPeer dials addr, completes IWP-LINK/1 handshake, and registers the active link.
func (d *Daemon) ConnectPeer(ctx context.Context, addr string) (*transport.Link, error) {
	link, err := transport.DialAndHandshake(addr, transport.HandshakeConfig{
		Identity:      d.id,
		Capabilities:  []string{transport.DefaultCapabilityMsg},
		IslandIDs:     d.currentIslandIDs(),
		AuthorizePeer: d.authorizePeer,
	})
	if err != nil {
		return nil, err
	}
	_ = d.store.UpsertPeer(&storage.PeerRecord{
		NodeID:    link.PeerID(),
		PublicKey: link.PeerPub(),
		Addrs:     []string{addr},
		Islands:   link.SharedIslands(),
		LastSeen:  time.Now().Unix(),
		Source:    "manual",
	})
	d.registerAndServeLink(ctx, link, "outbound")
	return link, nil
}

func (d *Daemon) registerAndServeLink(ctx context.Context, link *transport.Link, source string) {
	_ = d.store.UpsertPeer(&storage.PeerRecord{
		NodeID:       link.PeerID(),
		PublicKey:    link.PeerPub(),
		EncPublicKey: link.PeerEncPub(),
		Addrs:        []string{link.RemoteAddr().String()},
		Islands:      link.SharedIslands(),
		LastSeen:     time.Now().Unix(),
		Source:       source,
	})

	d.mu.Lock()
	if old, ok := d.links[link.PeerID()]; ok {
		_ = old.Close()
	}
	d.links[link.PeerID()] = link
	d.mu.Unlock()

	go d.readLinkLoop(d.runCtx, link)
	go d.resumePendingTransfers(link)
	go d.syncDTNBundles(link)
}

func (d *Daemon) readLinkLoop(ctx context.Context, link *transport.Link) {
	defer func() {
		_ = link.Close()
		d.mu.Lock()
		if cur, ok := d.links[link.PeerID()]; ok && cur == link {
			delete(d.links, link.PeerID())
		}
		d.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		env, err := link.RecvEnvelope()
		if err != nil {
			return
		}
		switch env.Type {
		case "chat.msg":
			var p transport.ChatPayload
			if err := json.Unmarshal(env.Payload, &p); err != nil {
				continue
			}
			_ = d.store.SaveMessage(&storage.MessageRecord{
				ID:          env.ID,
				IslandID:    env.IslandID,
				SenderID:    link.PeerID(),
				RecipientID: d.id.ID,
				Body:        p.Body,
				Direction:   "inbound",
				Status:      "received",
				Timestamp:   env.Timestamp,
			})
			ackBytes, _ := json.Marshal(transport.AckPayload{MsgID: env.ID})
			_ = link.SendEnvelope(&transport.Envelope{
				Version:   transport.ProtocolVersion,
				Type:      "chat.ack",
				ID:        uuid.NewString(),
				IslandID:  env.IslandID,
				SenderID:  d.id.ID,
				Timestamp: time.Now().Unix(),
				Payload:   ackBytes,
			})
		case "chat.ack":
			var p transport.AckPayload
			if err := json.Unmarshal(env.Payload, &p); err == nil && p.MsgID != "" {
				_ = d.store.UpdateMessageStatus(p.MsgID, "acked")
			}
		case "ping":
			_ = link.SendEnvelope(&transport.Envelope{
				Version:   transport.ProtocolVersion,
				Type:      "pong",
				ID:        uuid.NewString(),
				SenderID:  d.id.ID,
				Timestamp: time.Now().Unix(),
				Payload:   env.Payload,
			})
		default:
			if !d.handleDTNEnvelope(link, env) {
				d.handlePhase2Envelope(link, env)
			}
		}
	}
}

// SendMessage sends an encrypted chat.msg to peerID (auto-dialing if a known address exists).
func (d *Daemon) SendMessage(ctx context.Context, peerID, islandID, body string) (*storage.MessageRecord, error) {
	p, err := d.store.GetPeer(peerID)
	if err == nil && p.Revoked {
		return nil, fmt.Errorf("peer %s is revoked", peerID)
	}

	d.mu.RLock()
	link := d.links[peerID]
	d.mu.RUnlock()

	if link == nil {
		var dialAddr string
		if p != nil && len(p.Addrs) > 0 {
			dialAddr = p.Addrs[0]
		} else {
			for _, dp := range d.beacon.Discovered() {
				if dp.Announcement.NodeID == peerID {
					dialAddr = dp.DialAddr
					break
				}
			}
		}
		if dialAddr == "" {
			return nil, fmt.Errorf("peer %s is not connected and has no known address", peerID)
		}
		link, err = d.ConnectPeer(ctx, dialAddr)
		if err != nil {
			return nil, fmt.Errorf("connect to %s (%s): %w", peerID, dialAddr, err)
		}
	}

	if islandID == "" && len(link.SharedIslands()) > 0 {
		islandID = link.SharedIslands()[0]
	}

	msgID := uuid.NewString()
	now := time.Now().Unix()
	payloadBytes, _ := json.Marshal(transport.ChatPayload{Body: body})
	env := &transport.Envelope{
		Version:   transport.ProtocolVersion,
		Type:      "chat.msg",
		ID:        msgID,
		IslandID:  islandID,
		SenderID:  d.id.ID,
		Timestamp: now,
		Payload:   payloadBytes,
	}
	if err := link.SendEnvelope(env); err != nil {
		return nil, fmt.Errorf("send envelope: %w", err)
	}
	rec := &storage.MessageRecord{
		ID:          msgID,
		IslandID:    islandID,
		SenderID:    d.id.ID,
		RecipientID: peerID,
		Body:        body,
		Direction:   "outbound",
		Status:      "sent",
		Timestamp:   now,
	}
	_ = d.store.SaveMessage(rec)
	return rec, nil
}

func loadOrCreateToken(dataDir string) (string, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dataDir, "control.token")
	if raw, err := os.ReadFile(path); err == nil {
		tok := strings.TrimSpace(string(raw))
		if len(tok) >= 32 {
			return tok, nil
		}
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	tok := hex.EncodeToString(buf)
	if err := os.WriteFile(path, []byte(tok+"\n"), 0o600); err != nil {
		return "", err
	}
	return tok, nil
}

func (d *Daemon) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(auth, prefix) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		got := strings.TrimPrefix(auth, prefix)
		if subtle.ConstantTimeCompare([]byte(got), []byte(d.controlToken)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
