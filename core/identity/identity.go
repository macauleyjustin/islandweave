package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	NodeIDPrefix   = "iw:node:"
	IslandIDPrefix = "iw:island:"
)

// NodeIdentity holds a node's cryptographic Ed25519 identity and display name.
type NodeIdentity struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	PublicKey  ed25519.PublicKey  `json:"public_key"`
	PrivateKey ed25519.PrivateKey `json:"private_key"`
	CreatedAt  int64              `json:"created_at"`
}

type persistedIdentity struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
	CreatedAt  int64  `json:"created_at"`
}

// DeriveNodeID computes the canonical iw:node:<hex16> identifier from an Ed25519 public key.
func DeriveNodeID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return NodeIDPrefix + hex.EncodeToString(sum[:16])
}

// DeriveIslandID computes the canonical iw:island:<hex16> identifier from an Island Ed25519 public key.
func DeriveIslandID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return IslandIDPrefix + hex.EncodeToString(sum[:16])
}

// GenerateNodeIdentity creates a fresh Ed25519 node identity.
func GenerateNodeIdentity(name string) (*NodeIdentity, error) {
	if name == "" {
		hostname, err := os.Hostname()
		if err != nil || hostname == "" {
			name = "islandweave-node"
		} else {
			name = hostname
		}
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ed25519 key: %w", err)
	}
	return &NodeIdentity{
		ID:         DeriveNodeID(pub),
		Name:       name,
		PublicKey:  pub,
		PrivateKey: priv,
		CreatedAt:  time.Now().Unix(),
	}, nil
}

// SaveNodeIdentity persists the node identity to <dataDir>/keys/node.json with 0600 permissions.
func SaveNodeIdentity(dataDir string, id *NodeIdentity) error {
	keysDir := filepath.Join(dataDir, "keys")
	if err := os.MkdirAll(keysDir, 0o700); err != nil {
		return fmt.Errorf("create keys dir: %w", err)
	}
	p := persistedIdentity{
		ID:         id.ID,
		Name:       id.Name,
		PublicKey:  base64.StdEncoding.EncodeToString(id.PublicKey),
		PrivateKey: base64.StdEncoding.EncodeToString(id.PrivateKey),
		CreatedAt:  id.CreatedAt,
	}
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal identity: %w", err)
	}
	path := filepath.Join(keysDir, "node.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write node identity: %w", err)
	}
	return nil
}

// LoadNodeIdentity loads an existing node identity from <dataDir>/keys/node.json.
func LoadNodeIdentity(dataDir string) (*NodeIdentity, error) {
	path := filepath.Join(dataDir, "keys", "node.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p persistedIdentity
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("unmarshal node identity: %w", err)
	}
	pub, err := base64.StdEncoding.DecodeString(p.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("decode public key: %w", err)
	}
	priv, err := base64.StdEncoding.DecodeString(p.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("decode private key: %w", err)
	}
	if len(pub) != ed25519.PublicKeySize || len(priv) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid ed25519 key size")
	}
	if DeriveNodeID(pub) != p.ID {
		return nil, errors.New("node id does not match public key")
	}
	return &NodeIdentity{
		ID:         p.ID,
		Name:       p.Name,
		PublicKey:  ed25519.PublicKey(pub),
		PrivateKey: ed25519.PrivateKey(priv),
		CreatedAt:  p.CreatedAt,
	}, nil
}

// LoadOrCreateNodeIdentity loads the node identity if present, or creates and saves a new one.
func LoadOrCreateNodeIdentity(dataDir, name string) (*NodeIdentity, bool, error) {
	id, err := LoadNodeIdentity(dataDir)
	if err == nil {
		return id, false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, false, err
	}
	id, err = GenerateNodeIdentity(name)
	if err != nil {
		return nil, false, err
	}
	if err := SaveNodeIdentity(dataDir, id); err != nil {
		return nil, false, err
	}
	return id, true, nil
}

// SignDomain signs SHA256(domain || 0x00 || data) with an Ed25519 private key.
func SignDomain(priv ed25519.PrivateKey, domain string, data []byte) []byte {
	digest := domainDigest(domain, data)
	return ed25519.Sign(priv, digest[:])
}

// VerifyDomain verifies an Ed25519 signature created by SignDomain.
func VerifyDomain(pub ed25519.PublicKey, domain string, data, sig []byte) bool {
	if len(pub) != ed25519.PublicKeySize || len(sig) != ed25519.SignatureSize {
		return false
	}
	digest := domainDigest(domain, data)
	return ed25519.Verify(pub, digest[:], sig)
}

func domainDigest(domain string, data []byte) [32]byte {
	h := sha256.New()
	h.Write([]byte(domain))
	h.Write([]byte{0})
	h.Write(data)
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}
