package tests

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/islandweave/islandweave/core/daemon"
)

func TestPhase8CompleteEcosystemFeatures(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	makeNode := func(name string) *daemon.Daemon {
		d, err := daemon.New(daemon.Config{
			DataDir:       t.TempDir(),
			NodeName:      name,
			ListenAddr:    "127.0.0.1:0",
			ControlAddr:   "127.0.0.1:0",
			GatewayAddr:   "127.0.0.1:0",
			DisableBeacon: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := d.Start(ctx); err != nil {
			t.Fatal(err)
		}
		return d
	}

	d1 := makeNode("node-alpha")
	defer d1.Stop()

	d2 := makeNode("node-beta")
	defer d2.Stop()

	// Publish a Shared File Library on node-alpha (no index.html -> browsable directory listing via gateway)
	shareDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(shareDir, "notes.txt"), []byte("shared knowledge base"), 0o644)
	apiCall(t, d1.ControlAddr(), d1.Token(), http.MethodPost, "/v1/shares", map[string]string{
		"dir":  shareDir,
		"name": "field-notes",
	}, http.StatusCreated)

	resp, err := http.Get("http://" + d1.GatewayAddr() + "/site/field-notes/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "notes.txt") {
		t.Fatalf("expected shared directory index containing notes.txt, got %d %q", resp.StatusCode, string(body))
	}

	// Create an Island on node-alpha and join from node-beta so mutual authorization passes
	islRaw := apiCall(t, d1.ControlAddr(), d1.Token(), http.MethodPost, "/v1/islands", map[string]string{
		"name": "Phase8 Net",
	}, http.StatusCreated)
	var islResp struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(islRaw, &islResp)
	invRaw := apiCall(t, d1.ControlAddr(), d1.Token(), http.MethodPost, "/v1/islands/invite", map[string]any{
		"island_id": islResp.ID,
	}, http.StatusOK)
	var invResp struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(invRaw, &invResp)
	apiCall(t, d2.ControlAddr(), d2.Token(), http.MethodPost, "/v1/islands/join", map[string]string{
		"token": invResp.Token,
	}, http.StatusOK)

	// Publish a signed bulletin on node-alpha, connect node-beta, verify auto IWP-SYNC reconciles it
	apiCall(t, d1.ControlAddr(), d1.Token(), http.MethodPost, "/v1/bulletins/publish", map[string]any{
		"channel": "mesh-updates",
		"title":   "Auto-Sync Test",
		"body":    "IWP-SYNC works on link bring-up",
	}, http.StatusCreated)

	apiCall(t, d2.ControlAddr(), d2.Token(), http.MethodPost, "/v1/peers/connect", map[string]string{
		"addr": d1.P2PAddr(),
	}, http.StatusOK)

	deadline := time.Now().Add(4 * time.Second)
	synced := false
	for time.Now().Before(deadline) {
		raw := apiCall(t, d2.ControlAddr(), d2.Token(), http.MethodGet, "/v1/bulletins", nil, http.StatusOK)
		if strings.Contains(string(raw), "Auto-Sync Test") {
			synced = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !synced {
		t.Fatal("expected Auto-Sync Test bulletin to replicate to node-beta")
	}

	// Trigger manual IWP-SYNC + inspect routes, gateway, security, bandwidth, diagnostics, bookmarks, satellite sim
	apiCall(t, d2.ControlAddr(), d2.Token(), http.MethodPost, "/v1/sync", map[string]string{}, http.StatusOK)

	routesRaw := apiCall(t, d2.ControlAddr(), d2.Token(), http.MethodGet, "/v1/routes", nil, http.StatusOK)
	if !strings.Contains(string(routesRaw), d1.NodeID()) {
		t.Fatalf("expected route to %s in /v1/routes, got %s", d1.NodeID(), string(routesRaw))
	}

	apiCall(t, d1.ControlAddr(), d1.Token(), http.MethodGet, "/v1/gateway", nil, http.StatusOK)
	apiCall(t, d1.ControlAddr(), d1.Token(), http.MethodGet, "/v1/security/status", nil, http.StatusOK)
	apiCall(t, d1.ControlAddr(), d1.Token(), http.MethodGet, "/v1/security/audit", nil, http.StatusOK)
	apiCall(t, d1.ControlAddr(), d1.Token(), http.MethodGet, "/v1/bandwidth", nil, http.StatusOK)
	apiCall(t, d1.ControlAddr(), d1.Token(), http.MethodGet, "/v1/activity", nil, http.StatusOK)
	apiCall(t, d1.ControlAddr(), d1.Token(), http.MethodGet, "/v1/diagnostics", nil, http.StatusOK)

	apiCall(t, d1.ControlAddr(), d1.Token(), http.MethodPost, "/v1/navigator/bookmarks", map[string]string{
		"uri":   "island://field-notes/",
		"title": "Field Notes",
	}, http.StatusCreated)
	bms := apiCall(t, d1.ControlAddr(), d1.Token(), http.MethodGet, "/v1/navigator/bookmarks", nil, http.StatusOK)
	if !strings.Contains(string(bms), "island://field-notes/") {
		t.Fatalf("expected persisted bookmark, got %s", string(bms))
	}

	simRaw := apiCall(t, d1.ControlAddr(), d1.Token(), http.MethodPost, "/v1/satellite/simulate", map[string]any{
		"satellite_name": "IW-TEST-LEO",
		"max_bytes":      4096,
	}, http.StatusOK)
	var simResp map[string]any
	_ = json.Unmarshal(simRaw, &simResp)
	if simResp["satellite_name"] != "IW-TEST-LEO" {
		t.Fatalf("unexpected satellite simulate response: %s", string(simRaw))
	}
}
