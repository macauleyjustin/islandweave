package discovery

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net"
	"sync"
	"time"

	"github.com/islandweave/islandweave/core/beacon"
	"github.com/islandweave/islandweave/core/identity"
	"github.com/islandweave/islandweave/core/storage"
)

const TypePeerExchange = "peer.exchange"

// PeerHint is a verified peer address shared during authenticated IWP-LINK peer exchange or bootstrap config.
type PeerHint struct {
	NodeID       string   `json:"node_id"`
	NodeName     string   `json:"node_name,omitempty"`
	PublicKeyB64 string   `json:"public_key_b64,omitempty"`
	Addrs        []string `json:"addrs"`
	Islands      []string `json:"islands,omitempty"`
	LastSeen     int64    `json:"last_seen"`
}

// ExchangePayload carries consented peer hints across an authenticated IWP-LINK session.
type ExchangePayload struct {
	Peers []PeerHint `json:"peers"`
}

// DiscoveredEntry unifies Beacon UDP multicast, bootstrap peers, and peer-exchange sightings.
type DiscoveredEntry struct {
	NodeID     string   `json:"node_id"`
	NodeName   string   `json:"node_name,omitempty"`
	DialAddr   string   `json:"dial_addr"`
	Source     string   `json:"source"` // "beacon", "bootstrap", "peer-exchange"
	BeaconMode string   `json:"beacon_mode,omitempty"`
	IslandID   string   `json:"island_id,omitempty"`
	SeenAt     int64    `json:"seen_at"`
	Addrs      []string `json:"addrs,omitempty"`
}

// Coordinator manages non-beacon discovery sources (bootstraps + peer-exchange) alongside Beacon Service.
type Coordinator struct {
	selfID     string
	mu         sync.RWMutex
	bootstraps []string
	hints      map[string]DiscoveredEntry
}

// NewCoordinator initializes a discovery Coordinator for selfID.
func NewCoordinator(selfID string, bootstrapAddrs []string) *Coordinator {
	cp := make([]string, len(bootstrapAddrs))
	copy(cp, bootstrapAddrs)
	return &Coordinator{
		selfID:     selfID,
		bootstraps: cp,
		hints:      make(map[string]DiscoveredEntry),
	}
}

// AddBootstrap registers a consent-configured bootstrap host:port address.
func (c *Coordinator) AddBootstrap(addr string) {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, b := range c.bootstraps {
		if b == addr {
			return
		}
	}
	c.bootstraps = append(c.bootstraps, addr)
}

// BootstrapAddrs returns configured bootstrap addresses.
func (c *Coordinator) BootstrapAddrs() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, len(c.bootstraps))
	copy(out, c.bootstraps)
	return out
}

// RecordHints validates and stores PeerHints received over authenticated links.
func (c *Coordinator) RecordHints(hints []PeerHint) []PeerHint {
	var accepted []PeerHint
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now().Unix()
	for _, h := range hints {
		if h.NodeID == "" || h.NodeID == c.selfID || len(h.Addrs) == 0 {
			continue
		}
		if h.PublicKeyB64 != "" {
			pub, err := base64.StdEncoding.DecodeString(h.PublicKeyB64)
			if err != nil || len(pub) != ed25519.PublicKeySize {
				continue
			}
			if identity.DeriveNodeID(pub) != h.NodeID {
				continue
			}
		}
		seenAt := h.LastSeen
		if seenAt <= 0 {
			seenAt = now
		}
		c.hints[h.NodeID] = DiscoveredEntry{
			NodeID:   h.NodeID,
			NodeName: h.NodeName,
			DialAddr: h.Addrs[0],
			Addrs:    h.Addrs,
			Source:   "peer-exchange",
			SeenAt:   seenAt,
		}
		accepted = append(accepted, h)
	}
	return accepted
}

// BuildExchangePayload builds a shareable list of non-revoked known peers for an authenticated neighbor.
func (c *Coordinator) BuildExchangePayload(peers []*storage.PeerRecord) ([]byte, error) {
	var list []PeerHint
	for _, p := range peers {
		if p.Revoked || p.NodeID == c.selfID || len(p.Addrs) == 0 {
			continue
		}
		var pkB64 string
		if len(p.PublicKey) == ed25519.PublicKeySize {
			pkB64 = base64.StdEncoding.EncodeToString(p.PublicKey)
		}
		list = append(list, PeerHint{
			NodeID:       p.NodeID,
			NodeName:     p.Name,
			PublicKeyB64: pkB64,
			Addrs:        p.Addrs,
			Islands:      p.Islands,
			LastSeen:     p.LastSeen,
		})
	}
	return json.Marshal(ExchangePayload{Peers: list})
}

// MergedList combines live Beacon announcements and authenticated peer-exchange hints.
func (c *Coordinator) MergedList(beacons []beacon.DiscoveredPeer) []DiscoveredEntry {
	byID := make(map[string]DiscoveredEntry)
	c.mu.RLock()
	for id, h := range c.hints {
		byID[id] = h
	}
	c.mu.RUnlock()
	for _, b := range beacons {
		byID[b.Announcement.NodeID] = DiscoveredEntry{
			NodeID:     b.Announcement.NodeID,
			NodeName:   b.Announcement.NodeName,
			DialAddr:   b.DialAddr,
			Source:     "beacon",
			BeaconMode: string(b.Announcement.BeaconMode),
			IslandID:   b.Announcement.IslandID,
			SeenAt:     b.SeenAt,
			Addrs:      []string{b.DialAddr},
		}
	}
	out := make([]DiscoveredEntry, 0, len(byID))
	for _, v := range byID {
		out = append(out, v)
	}
	return out
}
