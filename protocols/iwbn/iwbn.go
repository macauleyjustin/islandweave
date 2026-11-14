package iwbn

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/islandweave/islandweave/core/identity"
)

const (
	bulletinDomain = "IWP-IWBN-1"

	TypeSync     = "iwbn.sync"
	TypeBulletin = "iwbn.bulletin"
)

// Bulletin is a signed, versioned community publication.
type Bulletin struct {
	Version           int    `json:"version"`
	BulletinID        string `json:"bulletin_id"`
	Channel           string `json:"channel"`
	Category          string `json:"category"` // "news", "article", "notice", "event", "alert"
	OfficialEmergency bool   `json:"official_emergency"`
	Title             string `json:"title"`
	Summary           string `json:"summary"`
	Body              string `json:"body"`
	IslandID          string `json:"island_id,omitempty"`
	PublisherID       string `json:"publisher_id"`
	PublisherPub      string `json:"publisher_pub"`
	BulletinVersion   int    `json:"bulletin_version"`
	CreatedAt         int64  `json:"created_at"`
	ExpiresAt         int64  `json:"expires_at,omitempty"`
	Signature         string `json:"signature"`
}

// CreateBulletin creates and signs a new Bulletin.
func CreateBulletin(pub *identity.NodeIdentity, channel, category, title, summary, body, islandID string, officialEmergency bool, version int, ttl time.Duration) (*Bulletin, error) {
	if pub == nil {
		return nil, errors.New("nil publisher identity")
	}
	if channel == "" || title == "" {
		return nil, errors.New("channel and title are required")
	}
	if category == "" {
		category = "news"
	}
	if version <= 0 {
		version = 1
	}
	now := time.Now()
	var exp int64
	if ttl > 0 {
		exp = now.Add(ttl).Unix()
	}
	b := &Bulletin{
		Version:           1,
		BulletinID:        uuid.NewString(),
		Channel:           channel,
		Category:          category,
		OfficialEmergency: officialEmergency,
		Title:             title,
		Summary:           summary,
		Body:              body,
		IslandID:          islandID,
		PublisherID:       pub.ID,
		PublisherPub:      base64.StdEncoding.EncodeToString(pub.PublicKey),
		BulletinVersion:   version,
		CreatedAt:         now.Unix(),
		ExpiresAt:         exp,
	}
	raw, err := canonicalBulletinBody(b)
	if err != nil {
		return nil, err
	}
	sig := identity.SignDomain(pub.PrivateKey, bulletinDomain, raw)
	b.Signature = base64.StdEncoding.EncodeToString(sig)
	return b, nil
}

// VerifyBulletin validates signature, publisher binding, and expiration.
func VerifyBulletin(b *Bulletin, now time.Time) error {
	if b == nil {
		return errors.New("nil bulletin")
	}
	if b.Version != 1 {
		return fmt.Errorf("unsupported bulletin version: %d", b.Version)
	}
	if b.ExpiresAt > 0 && now.Unix() > b.ExpiresAt {
		return errors.New("bulletin expired")
	}
	pubBytes, err := base64.StdEncoding.DecodeString(b.PublisherPub)
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return errors.New("invalid publisher public key")
	}
	pub := ed25519.PublicKey(pubBytes)
	if identity.DeriveNodeID(pub) != b.PublisherID {
		return errors.New("publisher_id does not match publisher_pub")
	}
	sig, err := base64.StdEncoding.DecodeString(b.Signature)
	if err != nil {
		return errors.New("invalid signature encoding")
	}
	raw, err := canonicalBulletinBody(b)
	if err != nil {
		return err
	}
	if !identity.VerifyDomain(pub, bulletinDomain, raw, sig) {
		return errors.New("bulletin signature verification failed")
	}
	return nil
}

// IsVerifiedEmergency returns true ONLY if OfficialEmergency is set AND PublisherID is in authorizedEmergencyPublishers.
// Ordinary notices cannot claim official emergency authority without explicit verification.
func IsVerifiedEmergency(b *Bulletin, authorizedEmergencyPublishers map[string]bool) bool {
	if b == nil || !b.OfficialEmergency {
		return false
	}
	return authorizedEmergencyPublishers[b.PublisherID]
}

func canonicalBulletinBody(b *Bulletin) ([]byte, error) {
	cp := *b
	cp.Signature = ""
	return json.Marshal(cp)
}
