package dtn

import (
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/islandweave/islandweave/core/identity"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

const (
	bundleDomain      = "IWP-DTN-BUNDLE-1"
	DefaultMaxHops    = 8
	DefaultBundleTTL  = 7 * 24 * time.Hour
	MaxBundleBytes    = 1024 * 1024      // 1 MiB max ciphertext
	DefaultQuotaBytes = 64 * 1024 * 1024 // 64 MiB default courier storage quota

	TypeBundle      = "dtn.bundle"
	TypeCustodyAck  = "dtn.custody_ack"
	TypeDeliveryAck = "dtn.delivery_ack"
)

// Bundle is an end-to-end encrypted, signed delay-tolerant store-and-forward unit.
type Bundle struct {
	Version       int    `json:"version"`
	BundleID      string `json:"bundle_id"`
	SourceID      string `json:"source_id"`
	SourcePub     string `json:"source_pub"`
	DestinationID string `json:"destination_id"`
	IslandID      string `json:"island_id,omitempty"`
	Kind          string `json:"kind"` // "msg", "mail", "bulletin", "ack"
	Priority      int    `json:"priority"`
	HopCount      int    `json:"hop_count"`
	MaxHops       int    `json:"max_hops"`
	CreatedAt     int64  `json:"created_at"`
	ExpiresAt     int64  `json:"expires_at"`
	EphemeralPub  string `json:"ephemeral_pub"`
	Ciphertext    string `json:"ciphertext"`
	Signature     string `json:"signature"`
}

// CustodyAckPayload acknowledges that a courier accepted responsibility for a bundle.
type CustodyAckPayload struct {
	BundleID      string `json:"bundle_id"`
	CustodyHolder string `json:"custody_holder"`
	Timestamp     int64  `json:"timestamp"`
}

// DeliveryAckPayload is the inner plaintext of a kind="ack" bundle returned to the original sender.
type DeliveryAckPayload struct {
	OriginalBundleID string `json:"original_bundle_id"`
	RecipientID      string `json:"recipient_id"`
	DeliveredAt      int64  `json:"delivered_at"`
}

// CourierPack is the portable file format for physical removable storage transport.
type CourierPack struct {
	Version    int      `json:"version"`
	ExportedBy string   `json:"exported_by"`
	ExportedAt int64    `json:"exported_at"`
	Bundles    []Bundle `json:"bundles"`
}

// CreateBundle encrypts plaintext for destEncPub (X25519) and signs the bundle with sender's Ed25519 key.
func CreateBundle(sender *identity.NodeIdentity, destID string, destEncPub []byte, islandID, kind string, priority, maxHops int, ttl time.Duration, plaintext []byte) (*Bundle, error) {
	if sender == nil {
		return nil, errors.New("nil sender identity")
	}
	if len(destEncPub) != 32 {
		return nil, errors.New("invalid recipient X25519 encryption public key")
	}
	if len(plaintext) > MaxBundleBytes {
		return nil, fmt.Errorf("payload exceeds max bundle size (%d > %d)", len(plaintext), MaxBundleBytes)
	}
	if maxHops <= 0 {
		maxHops = DefaultMaxHops
	}
	if ttl <= 0 {
		ttl = DefaultBundleTTL
	}
	if kind == "" {
		kind = "msg"
	}

	recipKey, err := ecdh.X25519().NewPublicKey(destEncPub)
	if err != nil {
		return nil, fmt.Errorf("parse recipient X25519 key: %w", err)
	}
	ephPriv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	shared, err := ephPriv.ECDH(recipKey)
	if err != nil {
		return nil, err
	}

	bundleID := uuid.NewString()
	aeadKey, err := deriveBundleKey(shared, bundleID)
	if err != nil {
		return nil, err
	}
	aead, err := chacha20poly1305.New(aeadKey)
	if err != nil {
		return nil, err
	}
	var nonce [chacha20poly1305.NonceSize]byte
	ct := aead.Seal(nil, nonce[:], plaintext, []byte(bundleID))

	now := time.Now()
	b := &Bundle{
		Version:       1,
		BundleID:      bundleID,
		SourceID:      sender.ID,
		SourcePub:     base64.StdEncoding.EncodeToString(sender.PublicKey),
		DestinationID: destID,
		IslandID:      islandID,
		Kind:          kind,
		Priority:      priority,
		HopCount:      0,
		MaxHops:       maxHops,
		CreatedAt:     now.Unix(),
		ExpiresAt:     now.Add(ttl).Unix(),
		EphemeralPub:  base64.StdEncoding.EncodeToString(ephPriv.PublicKey().Bytes()),
		Ciphertext:    base64.StdEncoding.EncodeToString(ct),
	}
	body, err := canonicalBundleBody(b)
	if err != nil {
		return nil, err
	}
	sig := identity.SignDomain(sender.PrivateKey, bundleDomain, body)
	b.Signature = base64.StdEncoding.EncodeToString(sig)
	return b, nil
}

// VerifyBundle validates signature, source ID binding, expiration, and hop count without decrypting.
func VerifyBundle(b *Bundle, now time.Time) error {
	if b == nil {
		return errors.New("nil bundle")
	}
	if b.Version != 1 {
		return fmt.Errorf("unsupported bundle version: %d", b.Version)
	}
	if b.ExpiresAt <= now.Unix() {
		return errors.New("bundle expired")
	}
	if b.MaxHops <= 0 || b.HopCount > b.MaxHops {
		return errors.New("bundle hop limit exceeded")
	}
	pubBytes, err := base64.StdEncoding.DecodeString(b.SourcePub)
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return errors.New("invalid source public key")
	}
	pub := ed25519.PublicKey(pubBytes)
	if identity.DeriveNodeID(pub) != b.SourceID {
		return errors.New("source_id does not match source_pub")
	}
	sig, err := base64.StdEncoding.DecodeString(b.Signature)
	if err != nil {
		return errors.New("invalid signature encoding")
	}
	body, err := canonicalBundleBody(b)
	if err != nil {
		return err
	}
	if !identity.VerifyDomain(pub, bundleDomain, body, sig) {
		return errors.New("bundle signature verification failed")
	}
	return nil
}

// DecryptBundle decrypts a verified bundle's ciphertext using the recipient's X25519 private key.
func DecryptBundle(b *Bundle, recipient *identity.NodeIdentity) ([]byte, error) {
	if recipient == nil || len(recipient.EncPrivateKey) != 32 {
		return nil, errors.New("recipient encryption private key required")
	}
	if b.DestinationID != recipient.ID {
		return nil, fmt.Errorf("bundle destination %s does not match local node %s", b.DestinationID, recipient.ID)
	}
	ephBytes, err := base64.StdEncoding.DecodeString(b.EphemeralPub)
	if err != nil {
		return nil, errors.New("invalid ephemeral public key")
	}
	ephPub, err := ecdh.X25519().NewPublicKey(ephBytes)
	if err != nil {
		return nil, err
	}
	privKey, err := ecdh.X25519().NewPrivateKey(recipient.EncPrivateKey)
	if err != nil {
		return nil, err
	}
	shared, err := privKey.ECDH(ephPub)
	if err != nil {
		return nil, err
	}
	aeadKey, err := deriveBundleKey(shared, b.BundleID)
	if err != nil {
		return nil, err
	}
	aead, err := chacha20poly1305.New(aeadKey)
	if err != nil {
		return nil, err
	}
	ct, err := base64.StdEncoding.DecodeString(b.Ciphertext)
	if err != nil {
		return nil, err
	}
	var nonce [chacha20poly1305.NonceSize]byte
	return aead.Open(nil, nonce[:], ct, []byte(b.BundleID))
}

func deriveBundleKey(shared []byte, bundleID string) ([]byte, error) {
	key := make([]byte, chacha20poly1305.KeySize)
	r := hkdf.New(sha256.New, shared, []byte(bundleID), []byte("iwp-dtn-1-e2e"))
	if _, err := io.ReadFull(r, key); err != nil {
		return nil, err
	}
	return key, nil
}

func canonicalBundleBody(b *Bundle) ([]byte, error) {
	cp := *b
	cp.HopCount = 0 // mutable per hop
	cp.Signature = ""
	return json.Marshal(cp)
}
