package services

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/islandweave/islandweave/core/identity"
)

const (
	serviceDomain   = "IWP-SERVICE-1"
	ServiceIDPrefix = "iw:service:"
)

// ServiceRecord is a signed, transport-independent service descriptor.
type ServiceRecord struct {
	Version      int    `json:"version"`
	ServiceID    string `json:"service_id"`
	Name         string `json:"name"`
	Type         string `json:"type"` // "website"
	IslandID     string `json:"island_id,omitempty"`
	PublisherID  string `json:"publisher_id"`
	PublisherPub string `json:"publisher_pub"`
	SiteVersion  int    `json:"site_version"`
	ManifestHash string `json:"manifest_hash"`
	CreatedAt    int64  `json:"created_at"`
	ExpiresAt    int64  `json:"expires_at,omitempty"`
	Signature    string `json:"signature"`
}

// SiteFileEntry describes a single static file in a hosted website.
type SiteFileEntry struct {
	Path        string   `json:"path"`
	Size        int64    `json:"size"`
	ContentType string   `json:"content_type"`
	FileHash    string   `json:"file_hash"`
	Chunks      []string `json:"chunks"`
}

// SiteManifest describes the full file tree of a static website.
type SiteManifest struct {
	Version     int             `json:"version"`
	ServiceID   string          `json:"service_id"`
	Name        string          `json:"name"`
	SiteVersion int             `json:"site_version"`
	PublisherID string          `json:"publisher_id"`
	Files       []SiteFileEntry `json:"files"`
	CreatedAt   int64           `json:"created_at"`
}

// DeriveServiceID deterministically derives a persistent service identity from publisher + service name.
func DeriveServiceID(publisherID, name string) string {
	h := sha256.New()
	h.Write([]byte(publisherID))
	h.Write([]byte{0})
	h.Write([]byte(name))
	sum := h.Sum(nil)
	return ServiceIDPrefix + hex.EncodeToString(sum[:16])
}

// SignServiceRecord populates and signs a ServiceRecord using the publisher's NodeIdentity.
func SignServiceRecord(id *identity.NodeIdentity, name, svcType, islandID, manifestHash string, siteVersion int, ttl time.Duration) (*ServiceRecord, error) {
	if id == nil {
		return nil, errors.New("nil identity")
	}
	if name == "" {
		return nil, errors.New("service name is required")
	}
	now := time.Now().Unix()
	var exp int64
	if ttl > 0 {
		exp = now + int64(ttl.Seconds())
	}
	rec := &ServiceRecord{
		Version:      1,
		ServiceID:    DeriveServiceID(id.ID, name),
		Name:         name,
		Type:         svcType,
		IslandID:     islandID,
		PublisherID:  id.ID,
		PublisherPub: base64.StdEncoding.EncodeToString(id.PublicKey),
		SiteVersion:  siteVersion,
		ManifestHash: manifestHash,
		CreatedAt:    now,
		ExpiresAt:    exp,
	}
	body, err := canonicalServiceBody(rec)
	if err != nil {
		return nil, err
	}
	sig := identity.SignDomain(id.PrivateKey, serviceDomain, body)
	rec.Signature = base64.StdEncoding.EncodeToString(sig)
	return rec, nil
}

// VerifyServiceRecord verifies signature, publisher binding, service ID derivation, and expiration.
func VerifyServiceRecord(rec *ServiceRecord, now time.Time) error {
	if rec == nil {
		return errors.New("nil service record")
	}
	if rec.Version != 1 {
		return fmt.Errorf("unsupported service version: %d", rec.Version)
	}
	pubBytes, err := base64.StdEncoding.DecodeString(rec.PublisherPub)
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return errors.New("invalid publisher public key")
	}
	pub := ed25519.PublicKey(pubBytes)
	if identity.DeriveNodeID(pub) != rec.PublisherID {
		return errors.New("publisher_id does not match publisher_pub")
	}
	if DeriveServiceID(rec.PublisherID, rec.Name) != rec.ServiceID {
		return errors.New("service_id does not match publisher_id and name")
	}
	if rec.ExpiresAt > 0 && now.Unix() > rec.ExpiresAt {
		return errors.New("service record expired")
	}
	sig, err := base64.StdEncoding.DecodeString(rec.Signature)
	if err != nil {
		return errors.New("invalid service signature encoding")
	}
	body, err := canonicalServiceBody(rec)
	if err != nil {
		return err
	}
	if !identity.VerifyDomain(pub, serviceDomain, body, sig) {
		return errors.New("service signature verification failed")
	}
	return nil
}

func canonicalServiceBody(rec *ServiceRecord) ([]byte, error) {
	cp := *rec
	cp.Signature = ""
	return json.Marshal(cp)
}
