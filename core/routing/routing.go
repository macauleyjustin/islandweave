package routing

import (
	"errors"
	"sort"
	"strings"

	"github.com/islandweave/islandweave/core/beacon"
	"github.com/islandweave/islandweave/core/storage"
	"github.com/islandweave/islandweave/transports/lan"
	"github.com/islandweave/islandweave/transports/relay"
	"github.com/islandweave/islandweave/transports/tor"
)

// TransportKind identifies a peer reachability transport class under IWP-ROUTE.
type TransportKind string

const (
	TransportLanTCP   TransportKind = "lan-tcp"
	TransportWanTCP   TransportKind = "wan-tcp"
	TransportTorOnion TransportKind = "tor-onion"
	TransportRelay    TransportKind = "relay"
	TransportDTNOnly  TransportKind = "dtn-only"
)

// RouteEntry describes a candidate communication path to a peer NodeID.
type RouteEntry struct {
	NodeID     string        `json:"node_id"`
	NodeName   string        `json:"node_name,omitempty"`
	Transport  TransportKind `json:"transport"`
	Endpoint   string        `json:"endpoint"`
	Priority   int           `json:"priority"` // Lower number = higher preference
	ActiveLink bool          `json:"active_link"`
	ViaRelayID string        `json:"via_relay_id,omitempty"`
	Trusted    bool          `json:"trusted"`
	QueuedDTN  int           `json:"queued_dtn"`
}

// ClassifyAddress classifies an endpoint string into its TransportKind and base priority.
func ClassifyAddress(addr string, torPolicy tor.Policy) (TransportKind, int) {
	if relay.IsOnionAddr(addr) {
		if torPolicy == tor.PolicyTorPreferred || torPolicy == tor.PolicyTorOnly {
			return TransportTorOnion, 10
		}
		return TransportTorOnion, 35
	}
	if torPolicy == tor.PolicyTorOnly {
		return TransportTorOnion, 15
	}
	if lan.IsLocalAddress(addr) {
		return TransportLanTCP, 10
	}
	return TransportWanTCP, 20
}

// ComputeRoutes builds a prioritized, Tor-policy-compliant multi-transport routing table.
func ComputeRoutes(peers []*storage.PeerRecord, activePeerIDs map[string]bool, beacons []beacon.DiscoveredPeer, dtn []*storage.DTNBundleRecord, torCfg tor.Config) []RouteEntry {
	dtnCount := make(map[string]int)
	for _, b := range dtn {
		if b.State == "queued" || b.State == "carrying" {
			dtnCount[b.DestinationID]++
		}
	}

	beaconByPeer := make(map[string]beacon.DiscoveredPeer)
	for _, dp := range beacons {
		beaconByPeer[dp.Announcement.NodeID] = dp
	}

	routes := make([]RouteEntry, 0)
	seen := make(map[string]bool)

	addRoute := func(r RouteEntry) {
		key := r.NodeID + "|" + string(r.Transport) + "|" + r.Endpoint
		if seen[key] {
			return
		}
		seen[key] = true
		routes = append(routes, r)
	}

	for _, p := range peers {
		if p.Revoked {
			continue
		}
		active := activePeerIDs[p.NodeID]
		for _, addr := range p.Addrs {
			if addr == "" {
				continue
			}
			// Under strict tor-only, exclude direct LAN/WAN routes unless dialable via Tor
			kind, prio := ClassifyAddress(addr, torCfg.Policy)
			if torCfg.Policy == tor.PolicyTorOnly && (kind == TransportLanTCP || kind == TransportWanTCP) {
				continue
			}
			if active {
				prio -= 5
			}
			addRoute(RouteEntry{
				NodeID:     p.NodeID,
				NodeName:   p.Name,
				Transport:  kind,
				Endpoint:   addr,
				Priority:   prio,
				ActiveLink: active,
				Trusted:    p.Trusted,
				QueuedDTN:  dtnCount[p.NodeID],
			})
		}
		if len(p.Addrs) == 0 && !active {
			addRoute(RouteEntry{
				NodeID:     p.NodeID,
				NodeName:   p.Name,
				Transport:  TransportDTNOnly,
				Endpoint:   "dtn-store-and-forward",
				Priority:   80,
				ActiveLink: false,
				Trusted:    p.Trusted,
				QueuedDTN:  dtnCount[p.NodeID],
			})
		}
	}

	// Include freshly Beacon-discovered peers even before peer table persistence
	if torCfg.Policy != tor.PolicyTorOnly {
		for _, dp := range beacons {
			addRoute(RouteEntry{
				NodeID:     dp.Announcement.NodeID,
				NodeName:   dp.Announcement.NodeName,
				Transport:  TransportLanTCP,
				Endpoint:   dp.DialAddr,
				Priority:   12,
				ActiveLink: activePeerIDs[dp.Announcement.NodeID],
				QueuedDTN:  dtnCount[dp.Announcement.NodeID],
			})
		}
	}

	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Priority != routes[j].Priority {
			return routes[i].Priority < routes[j].Priority
		}
		if routes[i].NodeID != routes[j].NodeID {
			return routes[i].NodeID < routes[j].NodeID
		}
		return routes[i].Endpoint < routes[j].Endpoint
	})
	return routes
}

// SelectDialTarget picks the highest-priority dialable direct/onion address for peerID.
func SelectDialTarget(routes []RouteEntry, peerID string) (directAddr string, onionAddr string, err error) {
	for _, r := range routes {
		if r.NodeID != peerID {
			continue
		}
		switch r.Transport {
		case TransportTorOnion:
			if onionAddr == "" {
				onionAddr = r.Endpoint
			}
		case TransportLanTCP, TransportWanTCP:
			if directAddr == "" && !strings.HasPrefix(r.Endpoint, "dtn") {
				directAddr = r.Endpoint
			}
		}
	}
	if directAddr == "" && onionAddr == "" {
		return "", "", errors.New("no live dialable route for peer")
	}
	return directAddr, onionAddr, nil
}
