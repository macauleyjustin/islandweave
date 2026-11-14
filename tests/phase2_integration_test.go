package tests

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/islandweave/islandweave/core/daemon"
	"github.com/islandweave/islandweave/core/storage"
)

func setupTwoConnectedNodes(t *testing.T) (context.Context, context.CancelFunc, *daemon.Daemon, *daemon.Daemon) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())

	dA, err := daemon.New(daemon.Config{
		DataDir:       t.TempDir(),
		NodeName:      "node-a",
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
		NodeName:      "node-b",
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

	islRaw := apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/islands", map[string]string{
		"name": "Phase2 Net",
	}, http.StatusCreated)
	var islResp struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(islRaw, &islResp)

	invRaw := apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/islands/invite", map[string]any{
		"island_id": islResp.ID,
	}, http.StatusOK)
	var invResp struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(invRaw, &invResp)

	apiCall(t, dB.ControlAddr(), dB.Token(), http.MethodPost, "/v1/islands/join", map[string]string{
		"token": invResp.Token,
	}, http.StatusOK)

	apiCall(t, dB.ControlAddr(), dB.Token(), http.MethodPost, "/v1/peers/connect", map[string]string{
		"addr": dA.P2PAddr(),
	}, http.StatusOK)

	return ctx, cancel, dA, dB
}

func TestPhase2FileTransferIWTP(t *testing.T) {
	_, cancel, dA, dB := setupTwoConnectedNodes(t)
	defer cancel()
	defer dA.Stop()
	defer dB.Stop()

	// Create a 650 KiB test file (3 chunks at 256 KiB)
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "report.bin")
	raw := make([]byte, 650*1024)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(srcFile, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	wantSum := sha256.Sum256(raw)
	wantHex := hex.EncodeToString(wantSum[:])

	trRaw := apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/transfers/send", map[string]string{
		"to":        dB.NodeID(),
		"file_path": srcFile,
	}, http.StatusOK)
	var tr storage.TransferRecord
	_ = json.Unmarshal(trRaw, &tr)
	if tr.ID == "" {
		t.Fatalf("expected transfer ID")
	}

	// Wait for both sides to mark transfer completed
	deadline := time.Now().Add(8 * time.Second)
	var recB *storage.TransferRecord
	for time.Now().Before(deadline) {
		rawB := apiCall(t, dB.ControlAddr(), dB.Token(), http.MethodGet, "/v1/transfers", nil, http.StatusOK)
		var listB []storage.TransferRecord
		_ = json.Unmarshal(rawB, &listB)
		for i := range listB {
			if listB[i].ID == tr.ID && listB[i].Status == "completed" {
				recB = &listB[i]
				break
			}
		}
		if recB != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if recB == nil {
		t.Fatalf("transfer did not complete on Node B")
	}

	gotBytes, err := os.ReadFile(recB.SavePath)
	if err != nil {
		t.Fatalf("read downloaded file %s: %v", recB.SavePath, err)
	}
	gotSum := sha256.Sum256(gotBytes)
	if hex.EncodeToString(gotSum[:]) != wantHex || !bytes.Equal(raw, gotBytes) {
		t.Fatalf("downloaded file hash/content mismatch")
	}
}

func TestPhase2StaticWebsiteHostingAndOfflineCache(t *testing.T) {
	_, cancel, dA, dB := setupTwoConnectedNodes(t)
	defer cancel()
	defer dB.Stop()

	siteDir := t.TempDir()
	indexHTML := "<html><head><link rel=\"stylesheet\" href=\"style.css\"></head><body><h1>Welcome to Harbor Net</h1></body></html>"
	styleCSS := "body { font-family: sans-serif; }"
	if err := os.WriteFile(filepath.Join(siteDir, "index.html"), []byte(indexHTML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(siteDir, "style.css"), []byte(styleCSS), 0o644); err != nil {
		t.Fatal(err)
	}

	apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/services/host", map[string]string{
		"dir":  siteDir,
		"name": "harbor-news",
	}, http.StatusCreated)

	// Node B browses the website via its local HTTP Gateway (auto-fetches from Node A)
	gwURL := "http://" + dB.GatewayAddr() + "/site/harbor-news/"
	resp, err := http.Get(gwURL)
	if err != nil {
		t.Fatalf("gateway GET %s: %v", gwURL, err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(body) != indexHTML {
		t.Fatalf("unexpected gateway response: code=%d body=%s", resp.StatusCode, string(body))
	}
	if resp.Header.Get("X-IslandWeave-Service-ID") == "" {
		t.Fatalf("missing X-IslandWeave-Service-ID header")
	}

	// Gateway must NOT expose privileged daemon control endpoints
	privResp, err := http.Get("http://" + dB.GatewayAddr() + "/v1/status")
	if err != nil {
		t.Fatal(err)
	}
	privResp.Body.Close()
	if privResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 on gateway /v1/status, got %d", privResp.StatusCode)
	}

	// Disconnect/stop Node A; Node B's gateway must still serve the cached site offline!
	_ = dA.Stop()
	respOffline, err := http.Get("http://" + dB.GatewayAddr() + "/site/harbor-news/style.css")
	if err != nil {
		t.Fatalf("offline gateway GET: %v", err)
	}
	cssBody, _ := io.ReadAll(respOffline.Body)
	respOffline.Body.Close()
	if respOffline.StatusCode != http.StatusOK || string(cssBody) != styleCSS {
		t.Fatalf("offline cached site failed: code=%d body=%s", respOffline.StatusCode, string(cssBody))
	}
}

func TestPhase2TransferResumeAfterReconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start Node A and Node B without connecting yet, join same Island
	dA, err := daemon.New(daemon.Config{
		DataDir:       t.TempDir(),
		NodeName:      "resume-a",
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
		NodeName:      "resume-b",
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

	islRaw := apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/islands", map[string]string{"name": "ResumeNet"}, http.StatusCreated)
	var isl struct{ ID string `json:"id"` }
	_ = json.Unmarshal(islRaw, &isl)
	invRaw := apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/islands/invite", map[string]any{"island_id": isl.ID}, http.StatusOK)
	var inv struct{ Token string `json:"token"` }
	_ = json.Unmarshal(invRaw, &inv)
	apiCall(t, dB.ControlAddr(), dB.Token(), http.MethodPost, "/v1/islands/join", map[string]string{"token": inv.Token}, http.StatusOK)

	// Connect, start transfer, then immediately cancel/close link? Better: connect, send file, verify completion,
	// AND test that a disconnected peer's queued/active transfer automatically resumes upon reconnect.
	apiCall(t, dB.ControlAddr(), dB.Token(), http.MethodPost, "/v1/peers/connect", map[string]string{"addr": dA.P2PAddr()}, http.StatusOK)

	srcFile := filepath.Join(t.TempDir(), "resumable.bin")
	raw := make([]byte, 800*1024)
	_, _ = rand.Read(raw)
	_ = os.WriteFile(srcFile, raw, 0o644)

	trRaw := apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/transfers/send", map[string]string{
		"to":        dB.NodeID(),
		"file_path": srcFile,
	}, http.StatusOK)
	var tr storage.TransferRecord
	_ = json.Unmarshal(trRaw, &tr)

	// Simulate an abrupt connection drop by reconnecting from Node B mid-flight (which replaces the old link on both sides),
	// triggering resumePendingTransfers on Node A!
	apiCall(t, dB.ControlAddr(), dB.Token(), http.MethodPost, "/v1/peers/connect", map[string]string{"addr": dA.P2PAddr()}, http.StatusOK)

	deadline := time.Now().Add(8 * time.Second)
	var recB *storage.TransferRecord
	for time.Now().Before(deadline) {
		rawB := apiCall(t, dB.ControlAddr(), dB.Token(), http.MethodGet, "/v1/transfers", nil, http.StatusOK)
		var listB []storage.TransferRecord
		_ = json.Unmarshal(rawB, &listB)
		for i := range listB {
			if listB[i].ID == tr.ID && listB[i].Status == "completed" {
				recB = &listB[i]
				break
			}
		}
		if recB != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if recB == nil {
		t.Fatalf("resumed transfer did not complete on Node B")
	}
	got, err := os.ReadFile(recB.SavePath)
	if err != nil || !bytes.Equal(raw, got) {
		t.Fatalf("resumed file content mismatch (err=%v)", err)
	}
}
