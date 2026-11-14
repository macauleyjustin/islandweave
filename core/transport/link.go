package transport

import (
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/islandweave/islandweave/core/identity"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

const (
	ProtocolVersion      = 1
	MaxHandshakeFrame    = 64 * 1024
	MaxRecordPlaintext   = 1024 * 1024
	HandshakeTimeout     = 10 * time.Second
	DefaultCapabilityMsg = "iwp.msg/1"

	serverTranscriptDomain = "IWP-LINK-1-SERVER"
	clientTranscriptDomain = "IWP-LINK-1-CLIENT"
)

// Envelope is the decrypted application-layer message exchanged over an IWP-LINK session.
type Envelope struct {
	Version   int             `json:"version"`
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	IslandID  string          `json:"island_id,omitempty"`
	SenderID  string          `json:"sender_id"`
	Timestamp int64           `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

// ChatPayload is the payload for "chat.msg".
type ChatPayload struct {
	Body    string `json:"body"`
	ReplyTo string `json:"reply_to,omitempty"`
}

// AckPayload is the payload for "chat.ack".
type AckPayload struct {
	MsgID string `json:"msg_id"`
}

// HandshakeConfig configures an IWP-LINK handshake.
type HandshakeConfig struct {
	Identity      *identity.NodeIdentity
	Capabilities  []string
	IslandIDs     []string
	AuthorizePeer func(peerID string, peerPub ed25519.PublicKey, sharedIslands []string) bool
}

type clientHello struct {
	Version      int      `json:"version"`
	EphemeralPub string   `json:"ephemeral_pub"`
	NodePub      string   `json:"node_pub"`
	EncPub       string   `json:"enc_pub,omitempty"`
	Nonce        string   `json:"nonce"`
	Capabilities []string `json:"capabilities"`
	IslandIDs    []string `json:"island_ids"`
}

type serverHelloBody struct {
	Version      int      `json:"version"`
	EphemeralPub string   `json:"ephemeral_pub"`
	NodePub      string   `json:"node_pub"`
	EncPub       string   `json:"enc_pub,omitempty"`
	Nonce        string   `json:"nonce"`
	Capabilities []string `json:"capabilities"`
	IslandIDs    []string `json:"island_ids"`
}

type serverHello struct {
	serverHelloBody
	Signature string `json:"signature"`
}

type clientAuth struct {
	Signature string `json:"signature"`
}

// Link is an authenticated, ChaCha20-Poly1305 encrypted peer connection.
type Link struct {
	conn          net.Conn
	peerID        string
	peerPub       ed25519.PublicKey
	peerEncPub    []byte
	capabilities  []string
	sharedIslands []string

	sendMu   sync.Mutex
	sendAEAD cipher.AEAD
	sendSeq  uint64

	recvMu   sync.Mutex
	recvAEAD cipher.AEAD
	recvSeq  uint64
}

func (l *Link) PeerID() string               { return l.peerID }
func (l *Link) PeerPub() ed25519.PublicKey   { return l.peerPub }
func (l *Link) PeerEncPub() []byte           { return append([]byte(nil), l.peerEncPub...) }
func (l *Link) Capabilities() []string       { return append([]string(nil), l.capabilities...) }
func (l *Link) SharedIslands() []string      { return append([]string(nil), l.sharedIslands...) }
func (l *Link) RemoteAddr() net.Addr         { return l.conn.RemoteAddr() }
func (l *Link) LocalAddr() net.Addr          { return l.conn.LocalAddr() }
func (l *Link) Close() error                 { return l.conn.Close() }

// DialAndHandshake connects to addr and performs the initiator side of IWP-LINK/1.
func DialAndHandshake(addr string, cfg HandshakeConfig) (*Link, error) {
	conn, err := net.DialTimeout("tcp", addr, HandshakeTimeout)
	if err != nil {
		return nil, err
	}
	link, err := HandshakeInitiator(conn, cfg)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return link, nil
}

// HandshakeInitiator performs the initiator side of IWP-LINK/1 over an existing net.Conn.
func HandshakeInitiator(conn net.Conn, cfg HandshakeConfig) (*Link, error) {
	if cfg.Identity == nil {
		return nil, errors.New("missing node identity")
	}
	_ = conn.SetDeadline(time.Now().Add(HandshakeTimeout))
	defer func() { _ = conn.SetDeadline(time.Time{}) }()

	ephPriv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	cNonce := make([]byte, 32)
	if _, err := rand.Read(cNonce); err != nil {
		return nil, err
	}

	caps := cfg.Capabilities
	if len(caps) == 0 {
		caps = []string{DefaultCapabilityMsg}
	}

	ch := clientHello{
		Version:      ProtocolVersion,
		EphemeralPub: base64.StdEncoding.EncodeToString(ephPriv.PublicKey().Bytes()),
		NodePub:      base64.StdEncoding.EncodeToString(cfg.Identity.PublicKey),
		EncPub:       base64.StdEncoding.EncodeToString(cfg.Identity.EncPublicKey),
		Nonce:        base64.StdEncoding.EncodeToString(cNonce),
		Capabilities: caps,
		IslandIDs:    cfg.IslandIDs,
	}
	chBytes, err := json.Marshal(ch)
	if err != nil {
		return nil, err
	}
	if err := writeFrame(conn, chBytes, MaxHandshakeFrame); err != nil {
		return nil, fmt.Errorf("send ClientHello: %w", err)
	}

	shBytes, err := readFrame(conn, MaxHandshakeFrame)
	if err != nil {
		return nil, fmt.Errorf("read ServerHello: %w", err)
	}
	var sh serverHello
	if err := json.Unmarshal(shBytes, &sh); err != nil {
		return nil, fmt.Errorf("unmarshal ServerHello: %w", err)
	}
	if sh.Version != ProtocolVersion {
		return nil, fmt.Errorf("unsupported protocol version: %d", sh.Version)
	}

	peerPubBytes, err := base64.StdEncoding.DecodeString(sh.NodePub)
	if err != nil || len(peerPubBytes) != ed25519.PublicKeySize {
		return nil, errors.New("invalid server node public key")
	}
	peerPub := ed25519.PublicKey(peerPubBytes)
	peerID := identity.DeriveNodeID(peerPub)

	peerEphBytes, err := base64.StdEncoding.DecodeString(sh.EphemeralPub)
	if err != nil {
		return nil, errors.New("invalid server ephemeral key")
	}
	peerEph, err := ecdh.X25519().NewPublicKey(peerEphBytes)
	if err != nil {
		return nil, fmt.Errorf("parse server ephemeral key: %w", err)
	}

	sNonce, err := base64.StdEncoding.DecodeString(sh.Nonce)
	if err != nil || len(sNonce) != 32 {
		return nil, errors.New("invalid server nonce")
	}

	shBodyBytes, err := json.Marshal(sh.serverHelloBody)
	if err != nil {
		return nil, err
	}
	transcript := append(append([]byte(nil), chBytes...), shBodyBytes...)

	sig, err := base64.StdEncoding.DecodeString(sh.Signature)
	if err != nil || !identity.VerifyDomain(peerPub, serverTranscriptDomain, transcript, sig) {
		return nil, errors.New("server handshake signature verification failed")
	}

	if cfg.AuthorizePeer != nil && !cfg.AuthorizePeer(peerID, peerPub, sh.IslandIDs) {
		return nil, fmt.Errorf("peer %s rejected by authorization policy", peerID)
	}

	cSig := identity.SignDomain(cfg.Identity.PrivateKey, clientTranscriptDomain, transcript)
	caBytes, err := json.Marshal(clientAuth{
		Signature: base64.StdEncoding.EncodeToString(cSig),
	})
	if err != nil {
		return nil, err
	}
	if err := writeFrame(conn, caBytes, MaxHandshakeFrame); err != nil {
		return nil, fmt.Errorf("send ClientAuth: %w", err)
	}

	shared, err := ephPriv.ECDH(peerEph)
	if err != nil {
		return nil, fmt.Errorf("ecdh: %w", err)
	}
	i2r, r2i, err := deriveSessionKeys(shared, cNonce, sNonce)
	if err != nil {
		return nil, err
	}
	sendAEAD, err := chacha20poly1305.New(i2r)
	if err != nil {
		return nil, err
	}
	recvAEAD, err := chacha20poly1305.New(r2i)
	if err != nil {
		return nil, err
	}

	var peerEncPub []byte
	if sh.EncPub != "" {
		peerEncPub, _ = base64.StdEncoding.DecodeString(sh.EncPub)
	}

	return &Link{
		conn:          conn,
		peerID:        peerID,
		peerPub:       peerPub,
		peerEncPub:    peerEncPub,
		capabilities:  sh.Capabilities,
		sharedIslands: sh.IslandIDs,
		sendAEAD:      sendAEAD,
		recvAEAD:      recvAEAD,
	}, nil
}

// HandshakeResponder performs the responder side of IWP-LINK/1 over an accepted net.Conn.
func HandshakeResponder(conn net.Conn, cfg HandshakeConfig) (*Link, error) {
	if cfg.Identity == nil {
		return nil, errors.New("missing node identity")
	}
	_ = conn.SetDeadline(time.Now().Add(HandshakeTimeout))
	defer func() { _ = conn.SetDeadline(time.Time{}) }()

	chBytes, err := readFrame(conn, MaxHandshakeFrame)
	if err != nil {
		return nil, fmt.Errorf("read ClientHello: %w", err)
	}
	var ch clientHello
	if err := json.Unmarshal(chBytes, &ch); err != nil {
		return nil, fmt.Errorf("unmarshal ClientHello: %w", err)
	}
	if ch.Version != ProtocolVersion {
		return nil, fmt.Errorf("unsupported protocol version: %d", ch.Version)
	}

	peerPubBytes, err := base64.StdEncoding.DecodeString(ch.NodePub)
	if err != nil || len(peerPubBytes) != ed25519.PublicKeySize {
		return nil, errors.New("invalid client node public key")
	}
	peerPub := ed25519.PublicKey(peerPubBytes)
	peerID := identity.DeriveNodeID(peerPub)

	peerEphBytes, err := base64.StdEncoding.DecodeString(ch.EphemeralPub)
	if err != nil {
		return nil, errors.New("invalid client ephemeral key")
	}
	peerEph, err := ecdh.X25519().NewPublicKey(peerEphBytes)
	if err != nil {
		return nil, fmt.Errorf("parse client ephemeral key: %w", err)
	}

	cNonce, err := base64.StdEncoding.DecodeString(ch.Nonce)
	if err != nil || len(cNonce) != 32 {
		return nil, errors.New("invalid client nonce")
	}

	localCaps := cfg.Capabilities
	if len(localCaps) == 0 {
		localCaps = []string{DefaultCapabilityMsg}
	}
	negCaps := intersectStrings(ch.Capabilities, localCaps)
	sharedIslands := intersectStrings(ch.IslandIDs, cfg.IslandIDs)

	if cfg.AuthorizePeer != nil && !cfg.AuthorizePeer(peerID, peerPub, sharedIslands) {
		return nil, fmt.Errorf("peer %s rejected by authorization policy", peerID)
	}

	ephPriv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	sNonce := make([]byte, 32)
	if _, err := rand.Read(sNonce); err != nil {
		return nil, err
	}

	body := serverHelloBody{
		Version:      ProtocolVersion,
		EphemeralPub: base64.StdEncoding.EncodeToString(ephPriv.PublicKey().Bytes()),
		NodePub:      base64.StdEncoding.EncodeToString(cfg.Identity.PublicKey),
		EncPub:       base64.StdEncoding.EncodeToString(cfg.Identity.EncPublicKey),
		Nonce:        base64.StdEncoding.EncodeToString(sNonce),
		Capabilities: negCaps,
		IslandIDs:    sharedIslands,
	}
	shBodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	transcript := append(append([]byte(nil), chBytes...), shBodyBytes...)
	sSig := identity.SignDomain(cfg.Identity.PrivateKey, serverTranscriptDomain, transcript)

	sh := serverHello{
		serverHelloBody: body,
		Signature:       base64.StdEncoding.EncodeToString(sSig),
	}
	shBytes, err := json.Marshal(sh)
	if err != nil {
		return nil, err
	}
	if err := writeFrame(conn, shBytes, MaxHandshakeFrame); err != nil {
		return nil, fmt.Errorf("send ServerHello: %w", err)
	}

	caBytes, err := readFrame(conn, MaxHandshakeFrame)
	if err != nil {
		return nil, fmt.Errorf("read ClientAuth: %w", err)
	}
	var ca clientAuth
	if err := json.Unmarshal(caBytes, &ca); err != nil {
		return nil, fmt.Errorf("unmarshal ClientAuth: %w", err)
	}
	cSig, err := base64.StdEncoding.DecodeString(ca.Signature)
	if err != nil || !identity.VerifyDomain(peerPub, clientTranscriptDomain, transcript, cSig) {
		return nil, errors.New("client handshake signature verification failed")
	}

	shared, err := ephPriv.ECDH(peerEph)
	if err != nil {
		return nil, fmt.Errorf("ecdh: %w", err)
	}
	i2r, r2i, err := deriveSessionKeys(shared, cNonce, sNonce)
	if err != nil {
		return nil, err
	}
	sendAEAD, err := chacha20poly1305.New(r2i)
	if err != nil {
		return nil, err
	}
	recvAEAD, err := chacha20poly1305.New(i2r)
	if err != nil {
		return nil, err
	}

	var peerEncPub []byte
	if ch.EncPub != "" {
		peerEncPub, _ = base64.StdEncoding.DecodeString(ch.EncPub)
	}

	return &Link{
		conn:          conn,
		peerID:        peerID,
		peerPub:       peerPub,
		peerEncPub:    peerEncPub,
		capabilities:  negCaps,
		sharedIslands: sharedIslands,
		sendAEAD:      sendAEAD,
		recvAEAD:      recvAEAD,
	}, nil
}

// SendEnvelope encrypts and transmits an Envelope over the link.
func (l *Link) SendEnvelope(env *Envelope) error {
	plain, err := json.Marshal(env)
	if err != nil {
		return err
	}
	if len(plain) > MaxRecordPlaintext {
		return fmt.Errorf("envelope exceeds max plaintext size (%d > %d)", len(plain), MaxRecordPlaintext)
	}
	l.sendMu.Lock()
	defer l.sendMu.Unlock()

	var nonce [chacha20poly1305.NonceSize]byte
	var aad [8]byte
	binary.BigEndian.PutUint64(nonce[4:], l.sendSeq)
	binary.BigEndian.PutUint64(aad[:], l.sendSeq)
	l.sendSeq++

	ct := l.sendAEAD.Seal(nil, nonce[:], plain, aad[:])
	return writeFrame(l.conn, ct, MaxRecordPlaintext+chacha20poly1305.Overhead)
}

// RecvEnvelope reads and decrypts the next Envelope from the link.
func (l *Link) RecvEnvelope() (*Envelope, error) {
	l.recvMu.Lock()
	defer l.recvMu.Unlock()

	ct, err := readFrame(l.conn, MaxRecordPlaintext+chacha20poly1305.Overhead)
	if err != nil {
		return nil, err
	}
	var nonce [chacha20poly1305.NonceSize]byte
	var aad [8]byte
	binary.BigEndian.PutUint64(nonce[4:], l.recvSeq)
	binary.BigEndian.PutUint64(aad[:], l.recvSeq)
	l.recvSeq++

	plain, err := l.recvAEAD.Open(nil, nonce[:], ct, aad[:])
	if err != nil {
		return nil, fmt.Errorf("decrypt record: %w", err)
	}
	var env Envelope
	if err := json.Unmarshal(plain, &env); err != nil {
		return nil, fmt.Errorf("unmarshal envelope: %w", err)
	}
	return &env, nil
}

func deriveSessionKeys(shared, cNonce, sNonce []byte) (i2r, r2i []byte, err error) {
	h := sha256.New()
	h.Write(cNonce)
	h.Write(sNonce)
	salt := h.Sum(nil)

	i2r = make([]byte, chacha20poly1305.KeySize)
	if _, err := io.ReadFull(hkdf.New(sha256.New, shared, salt, []byte("iwp-link-1-i2r")), i2r); err != nil {
		return nil, nil, err
	}
	r2i = make([]byte, chacha20poly1305.KeySize)
	if _, err := io.ReadFull(hkdf.New(sha256.New, shared, salt, []byte("iwp-link-1-r2i")), r2i); err != nil {
		return nil, nil, err
	}
	return i2r, r2i, nil
}

func writeFrame(w io.Writer, data []byte, maxLen int) error {
	if len(data) > maxLen {
		return fmt.Errorf("frame too large: %d > %d", len(data), maxLen)
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(data)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}

func readFrame(r io.Reader, maxLen int) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := int(binary.BigEndian.Uint32(hdr[:]))
	if n <= 0 || n > maxLen {
		return nil, fmt.Errorf("invalid frame length: %d", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func intersectStrings(a, b []string) []string {
	set := make(map[string]struct{}, len(b))
	for _, item := range b {
		set[item] = struct{}{}
	}
	var out []string
	seen := make(map[string]struct{})
	for _, item := range a {
		if _, ok := set[item]; ok {
			if _, dup := seen[item]; !dup {
				out = append(out, item)
				seen[item] = struct{}{}
			}
		}
	}
	return out
}
