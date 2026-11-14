package beacon

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/islandweave/islandweave/core/identity"
	"golang.org/x/net/ipv4"
)

const (
	DefaultMulticastAddr = "239.255.73.87:47470"
	beaconDomain         = "IWP-BEACON-1"
	maxClockSkewSec      = 60
)

// Mode controls how a node advertises itself locally.
type Mode string

const (
	ModeSilent     Mode = "silent"
	ModeRestricted Mode = "restricted"
	ModePublic     Mode = "public"
)

// Announcement is the signed IWP-BEACON / IWP-DISCOVER datagram payload.
type Announcement struct {
	Version      int      `json:"version"`
	NodeID       string   `json:"node_id"`
	NodePub      string   `json:"node_pub"`
	NodeName     string   `json:"node_name"`
	ListenPort   int      `json:"listen_port"`
	BeaconMode   Mode     `json:"beacon_mode"`
	IslandID     string   `json:"island_id,omitempty"`
	IslandName   string   `json:"island_name,omitempty"`
	Timestamp    int64    `json:"timestamp"`
	TTLSec       int64    `json:"ttl_sec"`
	Capabilities []string `json:"capabilities"`
	Signature    string   `json:"signature"`
}

// DiscoveredPeer holds a verified announcement and its source IP.
type DiscoveredPeer struct {
	Announcement Announcement `json:"announcement"`
	SourceAddr   string       `json:"source_addr"`
	DialAddr     string       `json:"dial_addr"`
	SeenAt       int64        `json:"seen_at"`
}

// Config configures the Beacon service.
type Config struct {
	Mode          Mode          `json:"mode"`
	Interval      time.Duration `json:"interval"`
	TTL           time.Duration `json:"ttl"`
	IslandID      string        `json:"island_id,omitempty"`
	IslandName    string        `json:"island_name,omitempty"`
	ListenPort    int           `json:"listen_port"`
	MulticastAddr string        `json:"multicast_addr"`
}

// Status reports current Beacon state.
type Status struct {
	Mode          Mode   `json:"mode"`
	IntervalSec   int    `json:"interval_sec"`
	TTLSec        int    `json:"ttl_sec"`
	IslandID      string `json:"island_id,omitempty"`
	IslandName    string `json:"island_name,omitempty"`
	MulticastAddr string `json:"multicast_addr"`
	Active        bool   `json:"active"`
}

// SignAnnouncement populates and signs an Announcement.
func SignAnnouncement(id *identity.NodeIdentity, cfg Config, now time.Time) (*Announcement, error) {
	if id == nil {
		return nil, errors.New("nil identity")
	}
	ttlSec := int64(cfg.TTL.Seconds())
	if ttlSec <= 0 {
		ttlSec = 30
	}
	ann := &Announcement{
		Version:      1,
		NodeID:       id.ID,
		NodePub:      base64.StdEncoding.EncodeToString(id.PublicKey),
		NodeName:     id.Name,
		ListenPort:   cfg.ListenPort,
		BeaconMode:   cfg.Mode,
		IslandID:     cfg.IslandID,
		IslandName:   cfg.IslandName,
		Timestamp:    now.Unix(),
		TTLSec:       ttlSec,
		Capabilities: []string{"iwp.msg/1"},
	}
	if cfg.Mode == ModeRestricted {
		// In restricted mode, do not broadcast human-readable Island name publicly.
		ann.IslandName = ""
	}
	body, err := canonicalAnnouncementBody(ann)
	if err != nil {
		return nil, err
	}
	sig := identity.SignDomain(id.PrivateKey, beaconDomain, body)
	ann.Signature = base64.StdEncoding.EncodeToString(sig)
	return ann, nil
}

// VerifyAnnouncement validates signature, NodeID binding, and expiration.
func VerifyAnnouncement(ann *Announcement, now time.Time) (ed25519.PublicKey, error) {
	if ann == nil {
		return nil, errors.New("nil announcement")
	}
	if ann.Version != 1 {
		return nil, fmt.Errorf("unsupported beacon version: %d", ann.Version)
	}
	pubBytes, err := base64.StdEncoding.DecodeString(ann.NodePub)
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return nil, errors.New("invalid node public key")
	}
	pub := ed25519.PublicKey(pubBytes)
	if identity.DeriveNodeID(pub) != ann.NodeID {
		return nil, errors.New("node_id does not match node_pub")
	}
	nowUnix := now.Unix()
	if ann.Timestamp > nowUnix+maxClockSkewSec {
		return nil, errors.New("announcement timestamp too far in future")
	}
	if ann.TTLSec <= 0 || ann.Timestamp+ann.TTLSec+maxClockSkewSec < nowUnix {
		return nil, errors.New("announcement expired")
	}
	sig, err := base64.StdEncoding.DecodeString(ann.Signature)
	if err != nil {
		return nil, errors.New("invalid signature encoding")
	}
	body, err := canonicalAnnouncementBody(ann)
	if err != nil {
		return nil, err
	}
	if !identity.VerifyDomain(pub, beaconDomain, body, sig) {
		return nil, errors.New("beacon signature verification failed")
	}
	return pub, nil
}

func canonicalAnnouncementBody(ann *Announcement) ([]byte, error) {
	cp := *ann
	cp.Signature = ""
	return json.Marshal(cp)
}

// Service manages UDP multicast discovery and beacon announcements.
type Service struct {
	id         *identity.NodeIdentity
	onDiscover func(DiscoveredPeer, ed25519.PublicKey)

	mu         sync.RWMutex
	cfg        Config
	discovered map[string]DiscoveredPeer
	running    bool
	cancel     context.CancelFunc
	pc         net.PacketConn
	sendConn   *net.UDPConn
}

// NewService creates a Beacon & Discovery service.
func NewService(id *identity.NodeIdentity, cfg Config, onDiscover func(DiscoveredPeer, ed25519.PublicKey)) *Service {
	if cfg.Mode == "" {
		cfg.Mode = ModeSilent
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 5 * time.Second
	}
	if cfg.TTL <= 0 {
		cfg.TTL = 30 * time.Second
	}
	if cfg.MulticastAddr == "" {
		cfg.MulticastAddr = DefaultMulticastAddr
	}
	return &Service{
		id:         id,
		cfg:        cfg,
		onDiscover: onDiscover,
		discovered: make(map[string]DiscoveredPeer),
	}
}

// Start begins listening for multicast announcements and broadcasting if not silent.
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	mcastAddr := s.cfg.MulticastAddr
	s.mu.Unlock()

	udpAddr, err := net.ResolveUDPAddr("udp4", mcastAddr)
	if err != nil {
		return fmt.Errorf("resolve multicast addr: %w", err)
	}

	lc := net.ListenConfig{Control: reuseControl}
	pc, err := lc.ListenPacket(ctx, "udp4", fmt.Sprintf(":%d", udpAddr.Port))
	if err != nil {
		return fmt.Errorf("listen multicast udp: %w", err)
	}

	pconn := ipv4.NewPacketConn(pc)
	_ = pconn.SetMulticastLoopback(true)

	joined := false
	if ifaces, err := net.Interfaces(); err == nil {
		for i := range ifaces {
			ifi := &ifaces[i]
			if (ifi.Flags&net.FlagUp) != 0 && (ifi.Flags&net.FlagMulticast) != 0 {
				if err := pconn.JoinGroup(ifi, udpAddr); err == nil {
					joined = true
				}
			}
		}
	}
	if !joined {
		if err := pconn.JoinGroup(nil, udpAddr); err != nil {
			pc.Close()
			return fmt.Errorf("join multicast group: %w", err)
		}
	}

	sendConn, err := net.DialUDP("udp4", nil, udpAddr)
	if err != nil {
		pc.Close()
		return fmt.Errorf("dial multicast udp: %w", err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.pc = pc
	s.sendConn = sendConn
	s.cancel = cancel
	s.running = true
	s.mu.Unlock()

	go s.readLoop(runCtx, pc)
	go s.broadcastLoop(runCtx)
	return nil
}

// Stop shuts down the Beacon service.
func (s *Service) Stop() {
	s.mu.Lock()
	cancel := s.cancel
	pc := s.pc
	sc := s.sendConn
	s.running = false
	s.cancel = nil
	s.pc = nil
	s.sendConn = nil
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if pc != nil {
		_ = pc.Close()
	}
	if sc != nil {
		_ = sc.Close()
	}
}

// UpdateConfig updates beacon mode, island info, or interval at runtime.
func (s *Service) UpdateConfig(fn func(*Config)) {
	s.mu.Lock()
	fn(&s.cfg)
	shouldBroadcast := s.running && s.cfg.Mode != ModeSilent
	s.mu.Unlock()
	if shouldBroadcast {
		s.BroadcastOnce()
	}
}

// Status returns the current beacon status.
func (s *Service) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Status{
		Mode:          s.cfg.Mode,
		IntervalSec:   int(s.cfg.Interval.Seconds()),
		TTLSec:        int(s.cfg.TTL.Seconds()),
		IslandID:      s.cfg.IslandID,
		IslandName:    s.cfg.IslandName,
		MulticastAddr: s.cfg.MulticastAddr,
		Active:        s.running && s.cfg.Mode != ModeSilent,
	}
}

// Discovered returns non-expired discovered peers.
func (s *Service) Discovered() []DiscoveredPeer {
	now := time.Now().Unix()
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []DiscoveredPeer
	for _, dp := range s.discovered {
		if dp.Announcement.Timestamp+dp.Announcement.TTLSec+maxClockSkewSec >= now {
			out = append(out, dp)
		}
	}
	return out
}

// BroadcastOnce sends a single signed beacon announcement if not in silent mode.
func (s *Service) BroadcastOnce() {
	s.mu.RLock()
	cfg := s.cfg
	sc := s.sendConn
	s.mu.RUnlock()

	if sc == nil || cfg.Mode == ModeSilent {
		return
	}
	ann, err := SignAnnouncement(s.id, cfg, time.Now())
	if err != nil {
		return
	}
	raw, err := json.Marshal(ann)
	if err != nil || len(raw) > 4096 {
		return
	}
	_, _ = sc.Write(raw)
}

func (s *Service) broadcastLoop(ctx context.Context) {
	s.BroadcastOnce()
	for {
		s.mu.RLock()
		interval := s.cfg.Interval
		s.mu.RUnlock()
		if interval <= 0 {
			interval = 5 * time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
			s.BroadcastOnce()
		}
	}
}

func (s *Service) readLoop(ctx context.Context, pc net.PacketConn) {
	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		_ = pc.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return
		}
		var ann Announcement
		if err := json.Unmarshal(buf[:n], &ann); err != nil {
			continue
		}
		if ann.NodeID == s.id.ID {
			continue
		}
		pub, err := VerifyAnnouncement(&ann, time.Now())
		if err != nil {
			continue
		}
		host, _, err := net.SplitHostPort(addr.String())
		if err != nil {
			host = addr.String()
		}
		dialAddr := fmt.Sprintf("%s:%d", host, ann.ListenPort)
		dp := DiscoveredPeer{
			Announcement: ann,
			SourceAddr:   addr.String(),
			DialAddr:     dialAddr,
			SeenAt:       time.Now().Unix(),
		}
		s.mu.Lock()
		s.discovered[ann.NodeID] = dp
		s.mu.Unlock()
		if s.onDiscover != nil {
			s.onDiscover(dp, pub)
		}
	}
}
