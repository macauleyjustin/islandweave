package islands

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/islandweave/islandweave/core/identity"
)

const inviteDomain = "IWP-SPACE-INVITE-1"

// Visibility defines how an Island is advertised and joined.
type Visibility string

const (
	VisibilityPublic     Visibility = "public"
	VisibilityPrivate    Visibility = "private"
	VisibilityInviteOnly Visibility = "invite-only"
)

// Island represents an IslandWeave network space and local membership.
type Island struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	PublicKey  ed25519.PublicKey  `json:"public_key"`
	PrivateKey ed25519.PrivateKey `json:"private_key,omitempty"`
	Role       string             `json:"role"` // "admin" or "member"
	Visibility Visibility         `json:"visibility"`
	CreatedAt  int64              `json:"created_at"`
}

// InviteToken is a signed capability allowing a node to join an Island.
type InviteToken struct {
	Version        int      `json:"version"`
	IslandID       string   `json:"island_id"`
	IslandName     string   `json:"island_name"`
	IslandPub      string   `json:"island_pub"`
	InviterID      string   `json:"inviter_id"`
	BootstrapAddrs []string `json:"bootstrap_addrs,omitempty"`
	ExpiresAt      int64    `json:"expires_at"`
	MaxUses        int      `json:"max_uses"`
	Nonce          string   `json:"nonce"`
	Signature      string   `json:"signature"`
}

// CreateIsland generates a new Island with its own Ed25519 root keypair.
func CreateIsland(name string, vis Visibility) (*Island, error) {
	if name == "" {
		return nil, errors.New("island name is required")
	}
	if vis == "" {
		vis = VisibilityInviteOnly
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate island key: %w", err)
	}
	return &Island{
		ID:         identity.DeriveIslandID(pub),
		Name:       name,
		PublicKey:  pub,
		PrivateKey: priv,
		Role:       "admin",
		Visibility: vis,
		CreatedAt:  time.Now().Unix(),
	}, nil
}

// CreateInvite issues a signed InviteToken for an Island (requires Island private key).
func CreateInvite(isl *Island, inviterID string, bootstrapAddrs []string, ttl time.Duration, maxUses int) (*InviteToken, error) {
	if isl == nil || len(isl.PrivateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("island admin private key required to create invite")
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, err
	}
	tok := &InviteToken{
		Version:        1,
		IslandID:       isl.ID,
		IslandName:     isl.Name,
		IslandPub:      base64.StdEncoding.EncodeToString(isl.PublicKey),
		InviterID:      inviterID,
		BootstrapAddrs: bootstrapAddrs,
		ExpiresAt:      time.Now().Add(ttl).Unix(),
		MaxUses:        maxUses,
		Nonce:          hex.EncodeToString(nonceBytes),
	}
	body, err := canonicalInviteBody(tok)
	if err != nil {
		return nil, err
	}
	sig := identity.SignDomain(isl.PrivateKey, inviteDomain, body)
	tok.Signature = base64.StdEncoding.EncodeToString(sig)
	return tok, nil
}

// VerifyInvite validates an InviteToken's cryptographic signature, ID binding, and expiration.
func VerifyInvite(tok *InviteToken, now time.Time) (*Island, error) {
	if tok == nil {
		return nil, errors.New("nil invite token")
	}
	if tok.Version != 1 {
		return nil, fmt.Errorf("unsupported invite version: %d", tok.Version)
	}
	pubBytes, err := base64.StdEncoding.DecodeString(tok.IslandPub)
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return nil, errors.New("invalid island public key in invite")
	}
	pub := ed25519.PublicKey(pubBytes)
	if identity.DeriveIslandID(pub) != tok.IslandID {
		return nil, errors.New("island_id does not match island_pub")
	}
	if tok.ExpiresAt > 0 && now.Unix() > tok.ExpiresAt {
		return nil, errors.New("invite token has expired")
	}
	sig, err := base64.StdEncoding.DecodeString(tok.Signature)
	if err != nil {
		return nil, errors.New("invalid invite signature encoding")
	}
	body, err := canonicalInviteBody(tok)
	if err != nil {
		return nil, err
	}
	if !identity.VerifyDomain(pub, inviteDomain, body, sig) {
		return nil, errors.New("invite signature verification failed")
	}
	return &Island{
		ID:         tok.IslandID,
		Name:       tok.IslandName,
		PublicKey:  pub,
		Role:       "member",
		Visibility: VisibilityInviteOnly,
		CreatedAt:  now.Unix(),
	}, nil
}

// EncodeInvite serializes an InviteToken as a compact base64url string.
func EncodeInvite(tok *InviteToken) (string, error) {
	raw, err := json.Marshal(tok)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// DecodeInvite parses a base64url-encoded InviteToken.
func DecodeInvite(encoded string) (*InviteToken, error) {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode invite token: %w", err)
	}
	var tok InviteToken
	if err := json.Unmarshal(raw, &tok); err != nil {
		return nil, fmt.Errorf("unmarshal invite token: %w", err)
	}
	return &tok, nil
}

func canonicalInviteBody(tok *InviteToken) ([]byte, error) {
	cp := *tok
	cp.Signature = ""
	return json.Marshal(cp)
}
