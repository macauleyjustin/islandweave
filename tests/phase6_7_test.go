package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/islandweave/islandweave/core/daemon"
	"github.com/islandweave/islandweave/core/storage"
	"github.com/islandweave/islandweave/protocols/iwmp"
	"github.com/islandweave/islandweave/protocols/iwst"
	"github.com/islandweave/islandweave/transports/satellite"
)

func TestPhase7SatelliteContactSimulatorInterruptAndResume(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dA, err := daemon.New(daemon.Config{
		DataDir:       t.TempDir(),
		NodeName:      "sat-ground-a",
		ListenAddr:    "127.0.0.1:0",
		ControlAddr:   "127.0.0.1:0",
		GatewayAddr:   "127.0.0.1:0",
		DisableBeacon: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := dA.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer dA.Stop()

	dB, err := daemon.New(daemon.Config{
		DataDir:       t.TempDir(),
		NodeName:      "sat-ground-b",
		ListenAddr:    "127.0.0.1:0",
		ControlAddr:   "127.0.0.1:0",
		GatewayAddr:   "127.0.0.1:0",
		DisableBeacon: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := dB.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer dB.Stop()

	// Exchange public keys offline (no network link between A and B)
	stBRaw := apiCall(t, dB.ControlAddr(), dB.Token(), http.MethodGet, "/v1/status", nil, http.StatusOK)
	var stB struct {
		NodeID       string `json:"node_id"`
		PublicKey    string `json:"public_key"`
		EncPublicKey string `json:"enc_public_key"`
	}
	_ = json.Unmarshal(stBRaw, &stB)

	apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/peers/import-key", map[string]string{
		"name":           "sat-ground-b",
		"public_key":     stB.PublicKey,
		"enc_public_key": stB.EncPublicKey,
	}, http.StatusOK)

	// Queue both a DTN message and an IWMP mail on Node A
	apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/courier/send", map[string]any{
		"to":   stB.NodeID,
		"body": "Telemetry report via satellite pass",
	}, http.StatusOK)

	apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/mail/send", map[string]any{
		"to":      stB.NodeID,
		"subject": "Orbit Mail",
		"body":    "Greetings across orbital passes",
	}, http.StatusOK)

	// Simulate 4 narrow, bandwidth-limited passes (450 bytes each) so bundles are interrupted mid-transfer and resume!
	now := time.Now().Unix()
	passes := []iwst.ContactWindow{
		{PassID: "pass-1", SatelliteName: "IW-SIM-1", StartTime: now, EndTime: now + 60, BandwidthBps: 1200, MaxBytes: 450},
		{PassID: "pass-2", SatelliteName: "IW-SIM-1", StartTime: now + 600, EndTime: now + 660, BandwidthBps: 1200, MaxBytes: 450},
		{PassID: "pass-3", SatelliteName: "IW-SIM-1", StartTime: now + 1200, EndTime: now + 1260, BandwidthBps: 1200, MaxBytes: 450},
		{PassID: "pass-4", SatelliteName: "IW-SIM-1", StartTime: now + 1800, EndTime: now + 1860, BandwidthBps: 1200, MaxBytes: 2000},
	}

	logs, err := satellite.RunContactSimulation(dA, dB, passes)
	if err != nil {
		t.Fatalf("RunContactSimulation: %v", err)
	}
	if len(logs) < 2 || !logs[0].Interrupted || logs[0].BundlesDelivered != 0 {
		t.Fatalf("expected pass-1 to be interrupted mid-bundle, got logs=%+v", logs)
	}

	// Verify both message and mail arrived decrypted on Node B
	msgsRaw := apiCall(t, dB.ControlAddr(), dB.Token(), http.MethodGet, "/v1/messages", nil, http.StatusOK)
	var msgs []storage.MessageRecord
	_ = json.Unmarshal(msgsRaw, &msgs)
	if len(msgs) != 1 || msgs[0].Body != "Telemetry report via satellite pass" {
		t.Fatalf("expected satellite DTN message on Node B, got %+v", msgs)
	}

	inboxRaw := apiCall(t, dB.ControlAddr(), dB.Token(), http.MethodGet, "/v1/mail?folder=inbox", nil, http.StatusOK)
	var inbox []iwmp.MailRecord
	_ = json.Unmarshal(inboxRaw, &inbox)
	if len(inbox) != 1 || inbox[0].Subject != "Orbit Mail" {
		t.Fatalf("expected satellite mail on Node B, got %+v", inbox)
	}

	// Check satellite status endpoint
	apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodGet, "/v1/satellite/status", nil, http.StatusOK)
}
