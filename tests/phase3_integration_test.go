package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/islandweave/islandweave/core/daemon"
	"github.com/islandweave/islandweave/core/storage"
)

func TestPhase3CourierRelayNeverSimultaneouslyOnline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dirC := t.TempDir()

	// Pre-initialize Node C so we know its public keys, then STOP Node C before Node A even starts!
	dCInit, err := daemon.New(daemon.Config{
		DataDir:       dirC,
		NodeName:      "node-carol",
		ListenAddr:    "127.0.0.1:0",
		ControlAddr:   "127.0.0.1:0",
		GatewayAddr:   "127.0.0.1:0",
		DisableBeacon: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := dCInit.Start(ctx); err != nil {
		t.Fatal(err)
	}
	statusCRaw := apiCall(t, dCInit.ControlAddr(), dCInit.Token(), http.MethodGet, "/v1/status", nil, http.StatusOK)
	var statusC struct {
		NodeID       string `json:"node_id"`
		PublicKey    string `json:"public_key"`
		EncPublicKey string `json:"enc_public_key"`
	}
	_ = json.Unmarshal(statusCRaw, &statusC)
	_ = dCInit.Stop() // Node C is OFFLINE!

	// Start Node A (sender) and Node B (courier)
	dA, err := daemon.New(daemon.Config{
		DataDir:       t.TempDir(),
		NodeName:      "node-alice",
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

	dB, err := daemon.New(daemon.Config{
		DataDir:       t.TempDir(),
		NodeName:      "node-courier",
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

	// Create Island on A and invite B (and save invite for C)
	islRaw := apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/islands", map[string]string{"name": "DTN Net"}, http.StatusCreated)
	var isl struct{ ID string `json:"id"` }
	_ = json.Unmarshal(islRaw, &isl)
	invRaw := apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/islands/invite", map[string]any{"island_id": isl.ID}, http.StatusOK)
	var inv struct{ Token string `json:"token"` }
	_ = json.Unmarshal(invRaw, &inv)

	apiCall(t, dB.ControlAddr(), dB.Token(), http.MethodPost, "/v1/islands/join", map[string]string{"token": inv.Token}, http.StatusOK)

	// Node A imports Node C's public keys and queues an encrypted DTN message for Node C
	apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/peers/import-key", map[string]string{
		"name":           "node-carol",
		"public_key":     statusC.PublicKey,
		"enc_public_key": statusC.EncPublicKey,
	}, http.StatusOK)

	apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/courier/send", map[string]any{
		"to":        statusC.NodeID,
		"island_id": isl.ID,
		"body":      "Delay-tolerant secret from Alice to Carol",
	}, http.StatusOK)

	// Node A connects to Courier B and hands off the encrypted bundle
	apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/peers/connect", map[string]string{
		"addr": dB.P2PAddr(),
	}, http.StatusOK)

	// Wait for Courier B to hold the bundle in "carrying" state
	deadline := time.Now().Add(5 * time.Second)
	var queueB []storage.DTNBundleRecord
	for time.Now().Before(deadline) {
		qRaw := apiCall(t, dB.ControlAddr(), dB.Token(), http.MethodGet, "/v1/courier/queue", nil, http.StatusOK)
		_ = json.Unmarshal(qRaw, &queueB)
		if len(queueB) > 0 && queueB[0].State == "carrying" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if len(queueB) == 0 || queueB[0].State != "carrying" {
		t.Fatalf("Courier B did not accept bundle: %+v", queueB)
	}

	// Courier B must NOT be able to read the message plaintext
	msgsBRaw := apiCall(t, dB.ControlAddr(), dB.Token(), http.MethodGet, "/v1/messages", nil, http.StatusOK)
	var msgsB []storage.MessageRecord
	_ = json.Unmarshal(msgsBRaw, &msgsB)
	if len(msgsB) != 0 {
		t.Fatalf("Courier B should have 0 decrypted messages, got %d", len(msgsB))
	}

	// Node A goes completely OFFLINE before Node C ever comes online!
	_ = dA.Stop()

	// Now Node C comes online, joins the Island, and connects to Courier B
	dC, err := daemon.New(daemon.Config{
		DataDir:       dirC,
		NodeName:      "node-carol",
		ListenAddr:    "127.0.0.1:0",
		ControlAddr:   "127.0.0.1:0",
		GatewayAddr:   "127.0.0.1:0",
		DisableBeacon: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := dC.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer dC.Stop()

	apiCall(t, dC.ControlAddr(), dC.Token(), http.MethodPost, "/v1/islands/join", map[string]string{"token": inv.Token}, http.StatusOK)
	apiCall(t, dC.ControlAddr(), dC.Token(), http.MethodPost, "/v1/peers/connect", map[string]string{"addr": dB.P2PAddr()}, http.StatusOK)

	// Wait for Node C to receive and decrypt the DTN message!
	var msgsC []storage.MessageRecord
	msgDeadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(msgDeadline) {
		rawC := apiCall(t, dC.ControlAddr(), dC.Token(), http.MethodGet, "/v1/messages", nil, http.StatusOK)
		_ = json.Unmarshal(rawC, &msgsC)
		if len(msgsC) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if len(msgsC) == 0 || msgsC[0].Body != "Delay-tolerant secret from Alice to Carol" {
		t.Fatalf("Node C did not receive expected DTN message: %+v", msgsC)
	}
}

func TestPhase3PhysicalCourierExportImportAndQuota(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dirC := t.TempDir()
	dC, err := daemon.New(daemon.Config{
		DataDir:       dirC,
		NodeName:      "usb-carol",
		ListenAddr:    "127.0.0.1:0",
		ControlAddr:   "127.0.0.1:0",
		GatewayAddr:   "127.0.0.1:0",
		DisableBeacon: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := dC.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer dC.Stop()

	statusCRaw := apiCall(t, dC.ControlAddr(), dC.Token(), http.MethodGet, "/v1/status", nil, http.StatusOK)
	var statusC struct {
		NodeID       string `json:"node_id"`
		PublicKey    string `json:"public_key"`
		EncPublicKey string `json:"enc_public_key"`
	}
	_ = json.Unmarshal(statusCRaw, &statusC)

	dA, err := daemon.New(daemon.Config{
		DataDir:       t.TempDir(),
		NodeName:      "usb-alice",
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

	apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/peers/import-key", map[string]string{
		"name":           "usb-carol",
		"public_key":     statusC.PublicKey,
		"enc_public_key": statusC.EncPublicKey,
	}, http.StatusOK)

	apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/courier/send", map[string]any{
		"to":   statusC.NodeID,
		"body": "Carried on a USB drive",
	}, http.StatusOK)

	packPath := filepath.Join(t.TempDir(), "usb.iwcourier")
	expRaw := apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/courier/export", map[string]string{
		"file_path": packPath,
	}, http.StatusOK)
	var expResp struct {
		ExportedBundles int `json:"exported_bundles"`
	}
	_ = json.Unmarshal(expRaw, &expResp)
	if expResp.ExportedBundles != 1 {
		t.Fatalf("expected 1 exported bundle, got %d", expResp.ExportedBundles)
	}

	// Import on Node C without any network connection between A and C
	impRaw := apiCall(t, dC.ControlAddr(), dC.Token(), http.MethodPost, "/v1/courier/import", map[string]string{
		"file_path": packPath,
	}, http.StatusOK)
	var impResp struct {
		Delivered int `json:"delivered"`
	}
	_ = json.Unmarshal(impRaw, &impResp)
	if impResp.Delivered != 1 {
		t.Fatalf("expected 1 delivered bundle on import, got %s", string(impRaw))
	}

	msgsRaw := apiCall(t, dC.ControlAddr(), dC.Token(), http.MethodGet, "/v1/messages", nil, http.StatusOK)
	var msgs []storage.MessageRecord
	_ = json.Unmarshal(msgsRaw, &msgs)
	if len(msgs) != 1 || msgs[0].Body != "Carried on a USB drive" {
		t.Fatalf("unexpected imported messages: %+v", msgs)
	}

	// Test Courier Storage Quota enforcement on a tiny-quota node
	dTiny, err := daemon.New(daemon.Config{
		DataDir:         t.TempDir(),
		NodeName:        "tiny-courier",
		ListenAddr:      "127.0.0.1:0",
		ControlAddr:     "127.0.0.1:0",
		GatewayAddr:     "127.0.0.1:0",
		DisableBeacon:   true,
		MaxCourierBytes: 10, // 10 bytes max!
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := dTiny.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer dTiny.Stop()

	impTinyRaw := apiCall(t, dTiny.ControlAddr(), dTiny.Token(), http.MethodPost, "/v1/courier/import", map[string]string{
		"file_path": packPath,
	}, http.StatusOK)
	var impTiny struct {
		Carrying int `json:"carrying"`
		Skipped  int `json:"skipped"`
	}
	_ = json.Unmarshal(impTinyRaw, &impTiny)
	if impTiny.Carrying != 0 || impTiny.Skipped != 1 {
		t.Fatalf("expected tiny-quota courier to reject oversized bundle, got %s", string(impTinyRaw))
	}
}
