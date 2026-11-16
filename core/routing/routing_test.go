package routing

import (
	"testing"

	"github.com/islandweave/islandweave/core/beacon"
	"github.com/islandweave/islandweave/core/storage"
	"github.com/islandweave/islandweave/transports/tor"
)

func TestComputeRoutesAndTorOnlyExclusion(t *testing.T) {
	peers := []*storage.PeerRecord{
		{NodeID: "iw:node:1111", Name: "alice", Addrs: []string{"192.168.1.5:47472", "alice1234567890.onion:47472"}, Trusted: true},
		{NodeID: "iw:node:2222", Name: "bob-offline"},
		{NodeID: "iw:node:3333", Name: "mallory", Revoked: true, Addrs: []string{"10.0.0.5:47472"}},
	}
	routes := ComputeRoutes(peers, map[string]bool{"iw:node:1111": true}, []beacon.DiscoveredPeer{
		{Announcement: beacon.Announcement{NodeID: "iw:node:4444", NodeName: "nearby"}, DialAddr: "192.168.1.99:47472"},
	}, nil, tor.Config{Policy: tor.PolicyDirectFirst})

	for _, r := range routes {
		if r.NodeID == "iw:node:3333" {
			t.Fatalf("revoked peer should not appear in routes")
		}
	}
	direct, onion, err := SelectDialTarget(routes, "iw:node:1111")
	if err != nil || direct != "192.168.1.5:47472" || onion == "" {
		t.Fatalf("unexpected dial target: direct=%q onion=%q err=%v", direct, onion, err)
	}

	torRoutes := ComputeRoutes(peers, nil, nil, nil, tor.Config{Policy: tor.PolicyTorOnly})
	for _, r := range torRoutes {
		if r.Transport == TransportLanTCP || r.Transport == TransportWanTCP {
			t.Fatalf("tor-only mode leaked clearnet route: %+v", r)
		}
	}
}
