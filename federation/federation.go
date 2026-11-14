package federation

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/islandweave/islandweave/core/identity"
	"github.com/islandweave/islandweave/core/islands"
)

const federationDomain = "IWP-FED-1"

// Agreement defines a signed, scoped federation policy between two independent Islands.
type Agreement struct {
	Version           int      `json:"version"`
	AgreementID       string   `json:"agreement_id"`
	LocalIslandID     string   `json:"local_island_id"`
	LocalIslandPub    string   `json:"local_island_pub"`
	RemoteIslandID    string   `json:"remote_island_id"`
	SharedServices    []string `json:"shared_services,omitempty"`
	AllowedIdentities []string `json:"allowed_identities,omitempty"`
	AllowBulletinSync bool     `json:"allow_bulletin_sync"`
	AllowDTNForward   bool     `json:"allow_dtn_forward"`
	CreatedAt         int64    `json:"created_at"`
	ExpiresAt         int64    `json:"expires_at"`
	Revoked           bool     `json:"revoked"`
	Signature         string   `json:"signature"`
}

// CreateAgreement creates and signs a federation agreement using the local Island's admin key.
func CreateAgreement(local *islands.Island, remoteIslandID string, sharedServices, allowedIdentities []string, allowBulletins, allowDTN bool, ttl time.Duration) (*Agreement, error) {
	if local == nil || len(local.PrivateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("local island admin private key required")
	}
	if remoteIslandID == "" {
		return nil, errors.New("remote_island_id is required")
	}
	if ttl <= 0 {
		ttl = 30 * 24 * time.Hour
	}
	now := time.Now()
	ag := &Agreement{
		Version:           1,
		AgreementID:       uuid.NewString(),
		LocalIslandID:     local.ID,
		LocalIslandPub:    base64.StdEncoding.EncodeToString(local.PublicKey),
		RemoteIslandID:    remoteIslandID,
		SharedServices:    sharedServices,
		AllowedIdentities: allowedIdentities,
		AllowBulletinSync: allowBulletins,
		AllowDTNForward:   allowDTN,
		CreatedAt:         now.Unix(),
		ExpiresAt:         now.Add(ttl).Unix(),
	}
	raw, err := canonicalBody(ag)
	if err != nil {
		return nil, err
	}
	sig := identity.SignDomain(local.PrivateKey, federationDomain, raw)
	ag.Signature = base64.StdEncoding.EncodeToString(sig)
	return ag, nil
}

// VerifyAgreement validates the signature, Island key binding, revocation, and expiration.
func VerifyAgreement(ag *Agreement, now time.Time) error {
	if ag == nil {
		return errors.New("nil agreement")
	}
	if ag.Version != 1 {
		return fmt.Errorf("unsupported agreement version: %d", ag.Version)
	}
	if ag.Revoked {
		return errors.New("federation agreement is revoked")
	}
	if ag.ExpiresAt > 0 && now.Unix() > ag.ExpiresAt {
		return errors.New("federation agreement expired")
	}
	pubBytes, err := base64.StdEncoding.DecodeString(ag.LocalIslandPub)
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return errors.New("invalid local island public key")
	}
	pub := ed25519.PublicKey(pubBytes)
	if identity.DeriveIslandID(pub) != ag.LocalIslandID {
		return errors.New("local_island_id does not match local_island_pub")
	}
	sig, err := base64.StdEncoding.DecodeString(ag.Signature)
	if err != nil {
		return errors.New("invalid signature encoding")
	}
	raw, err := canonicalBody(ag)
	if err != nil {
		return err
	}
	if !identity.VerifyDomain(pub, federationDomain, raw, sig) {
		return errors.New("federation agreement signature verification failed")
	}
	return nil
}

// AllowsService checks whether serviceName is permitted under an active agreement.
func (ag *Agreement) AllowsService(serviceName string, now time.Time) bool {
	if ag.Revoked || (ag.ExpiresAt > 0 && now.Unix() > ag.ExpiresAt) {
		return false
	}
	for _, s := range ag.SharedServices {
		if s == "*" || s == serviceName {
			return true
		}
	}
	return false
}

func canonicalBody(ag *Agreement) ([]byte, error) {
	cp := *ag
	cp.Revoked = false
	cp.Signature = ""
	return json.Marshal(cp)
}
