package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/islandweave/islandweave/core/daemon"
	"github.com/islandweave/islandweave/core/islands"
	"github.com/islandweave/islandweave/federation"
	"github.com/islandweave/islandweave/protocols/iwst"
	"github.com/islandweave/islandweave/transports/satellite"
)

func main() {
	tmpRoot, err := os.MkdirTemp("", "islandweave-sim-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpRoot)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startNode := func(name string, relay bool) *daemon.Daemon {
		d, err := daemon.New(daemon.Config{
			DataDir:       filepath.Join(tmpRoot, name),
			NodeName:      name,
			ListenAddr:    "127.0.0.1:0",
			ControlAddr:   "127.0.0.1:0",
			GatewayAddr:   "127.0.0.1:0",
			RelayEnabled:  relay,
			DisableBeacon: true,
		})
		if err != nil {
			log.Fatalf("init %s: %v", name, err)
		}
		if err := d.Start(ctx); err != nil {
			log.Fatalf("start %s: %v", name, err)
		}
		return d
	}

	harbor := startNode("harbor-hub", false)
	defer harbor.Stop()
	hilltop := startNode("hilltop-outpost", false)
	defer hilltop.Stop()
	relay := startNode("vps-relay", true)
	defer relay.Stop()

	fmt.Println("=== IslandWeave Full Ecosystem Scenario Simulator ===")
	fmt.Printf("1. Started Harbor (%s), Hilltop (%s), Relay (%s)\n", harbor.NodeID(), hilltop.NodeID(), relay.NodeID())

	// Exchange public keys and connect Harbor <-> Relay
	linkHR, err := harbor.ConnectPeer(ctx, relay.P2PAddr())
	if err != nil {
		log.Fatalf("Harbor -> Relay link failed: %v", err)
	}
	fmt.Printf("2. IWP-LINK/1 authenticated Harbor <-> Relay (%s)\n", linkHR.PeerID())

	// Host example website if present, or synthesize one
	siteDir := filepath.Join("examples", "harbor-news")
	if _, err := os.Stat(siteDir); err != nil {
		siteDir = filepath.Join(tmpRoot, "site")
		_ = os.MkdirAll(siteDir, 0o755)
		_ = os.WriteFile(filepath.Join(siteDir, "index.html"), []byte("<h1>Harbor News Offline</h1>"), 0o644)
	}
	rec, _, err := harbor.HostSite(siteDir, "harbor-news", "")
	if err != nil {
		log.Fatalf("host site: %v", err)
	}
	fmt.Printf("3. Hosted signed website harbor-news -> %s (v%d)\n", rec.ServiceID, rec.SiteVersion)

	// Publish signed IWBN bulletin
	bul, err := harbor.PublishBulletin("weather", "alert", "Storm Advisory", "Wind gusts", "Secure harbor lines.", "", false, 24*time.Hour)
	if err != nil {
		log.Fatalf("publish bulletin: %v", err)
	}
	fmt.Printf("4. Published IWBN bulletin %q (%s)\n", bul.Title, bul.BulletinID)

	// Trigger IWP-SYNC Harbor -> Relay
	if _, err := harbor.SyncNow(relay.NodeID()); err != nil {
		log.Fatalf("sync failed: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	// Exchange Hilltop's public keys out-of-band and queue E2E encrypted DTN message for offline courier & satellite
	_, hName, hPub, hEnc := hilltop.PublicIdentity()
	if _, err := harbor.ImportPeerKey(hName, hPub, hEnc); err != nil {
		log.Fatalf("import Hilltop key failed: %v", err)
	}
	if _, err := harbor.QueueDTNMessage(hilltop.NodeID(), "", "Supply Manifest via Offline Courier + Satellite", 1, 8, time.Hour); err != nil {
		log.Fatalf("queue DTN message failed: %v", err)
	}

	packPath := filepath.Join(tmpRoot, "shuttle.iwcourier")
	if count, err := harbor.ExportCourierPack(packPath); err == nil {
		deliv, carry, skip, err := hilltop.ImportCourierPack(packPath)
		if err != nil {
			log.Fatalf("courier import failed: %v", err)
		}
		fmt.Printf("5. Physical .iwcourier shuttle exported %d bundle(s) -> imported on Hilltop (delivered=%d carrying=%d skipped=%d)\n",
			count, deliv, carry, skip)
	} else {
		log.Fatalf("courier export failed: %v", err)
	}

	// Simulate IWST satellite passes
	now := time.Now().Unix()
	logs, err := satellite.RunContactSimulation(harbor, hilltop, []iwst.ContactWindow{
		{PassID: "sim-leo-1", SatelliteName: "IW-SIM-1", StartTime: now, EndTime: now + 60, BandwidthBps: 9600, MaxBytes: 100000},
	})
	if err != nil {
		log.Fatalf("satellite sim failed: %v", err)
	}
	fmt.Printf("6. IWST satellite pass completed: %d log(s), bytes=%d\n", len(logs), logs[0].BytesTransferred)

	// Federation signature verification demo
	islA, _ := islands.CreateIsland("HarborIsland", islands.VisibilityInviteOnly)
	islB, _ := islands.CreateIsland("HilltopIsland", islands.VisibilityInviteOnly)
	ag, err := federation.CreateAgreement(islA, islB.ID, []string{rec.ServiceID}, nil, true, true, 24*time.Hour)
	if err != nil {
		log.Fatalf("federation agreement failed: %v", err)
	}
	if err := federation.VerifyAgreement(ag, time.Now()); err != nil {
		log.Fatalf("federation verify failed: %v", err)
	}
	fmt.Printf("7. Verified IWP-FED-1 bilateral agreement %s (%s <-> %s)\n", ag.AgreementID, islA.Name, islB.Name)
	fmt.Printf("8. Routing table entries on Harbor: %d routes; Traffic stats: %+v\n", len(harbor.ComputeRoutes()), harbor.TrafficSnapshot())
	fmt.Println("=== Scenario Complete: All Systems Operational ===")
}
