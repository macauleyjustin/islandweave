package discovery

import (
	"encoding/base64"
	"testing"

	"github.com/islandweave/islandweave/core/beacon"
	"github.com/islandweave/islandweave/core/identity"
)

func TestCoordinatorVerifiesPeerHints(t *testing.T) {
	idOther, _, err := identity.LoadOrCreateNodeIdentity(t.TempDir(), "other")
	if err != nil {
		t.Fatal(err)
	}
	c := NewCoordinator("iw:node:self", []string{"127.0.0.1:47472"})
	c.AddBootstrap("192.168.1.50:47472")
	if len(c.BootstrapAddrs()) != 2 {
		t.Fatalf("expected 2 bootstraps, got %v", c.BootstrapAddrs())
	}

	b64 := base64.StdEncoding.EncodeToString(idOther.PublicKey)
	accepted := c.RecordHints([]PeerHint{
		{NodeID: idOther.ID, NodeName: "other", PublicKeyB64: b64, Addrs: []string{"10.0.0.2:47472"}},
		{NodeID: "iw:node:badmismatch", NodeName: "forged", PublicKeyB64: b64, Addrs: []string{"10.0.0.9:47472"}},
	})
	if len(accepted) != 1 || accepted[0].NodeID != idOther.ID {
		t.Fatalf("expected only valid hint accepted, got %+v", accepted)
	}

	merged := c.MergedList([]beacon.DiscoveredPeer{
		{Announcement: beacon.Announcement{NodeID: "iw:node:beacon1", NodeName: "b1"}, DialAddr: "192.168.1.7:47472"},
	})
	if len(merged) != 2 {
		t.Fatalf("expected 2 merged discovery entries, got %d", len(merged))
	}
}
