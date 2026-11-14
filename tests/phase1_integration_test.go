package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/islandweave/islandweave/core/beacon"
	"github.com/islandweave/islandweave/core/daemon"
	"github.com/islandweave/islandweave/core/storage"
)

func apiCall(t *testing.T, baseURL, token, method, path string, body any, wantCode int) []byte {
	t.Helper()
	var r io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, "http://"+baseURL+path, r)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("%s %s failed: %v", method, path, err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantCode {
		t.Fatalf("%s %s: got status %d, want %d; body=%s", method, path, resp.StatusCode, wantCode, string(out))
	}
	return out
}

func TestPhase1TwoNodesDiscoverConnectMessageAndRevoke(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use a dedicated test multicast port so we don't collide with any running daemon.
	mcast := "239.255.73.87:47570"

	dA, err := daemon.New(daemon.Config{
		DataDir:       t.TempDir(),
		NodeName:      "node-alpha",
		ListenAddr:    "127.0.0.1:0",
		ControlAddr:   "127.0.0.1:0",
		GatewayAddr:   "127.0.0.1:0",
		MulticastAddr: mcast,
	})
	if err != nil {
		t.Fatalf("new daemon A: %v", err)
	}
	if err := dA.Start(ctx); err != nil {
		t.Fatalf("start daemon A: %v", err)
	}
	defer dA.Stop()

	dB, err := daemon.New(daemon.Config{
		DataDir:       t.TempDir(),
		NodeName:      "node-beta",
		ListenAddr:    "127.0.0.1:0",
		ControlAddr:   "127.0.0.1:0",
		GatewayAddr:   "127.0.0.1:0",
		MulticastAddr: mcast,
	})
	if err != nil {
		t.Fatalf("new daemon B: %v", err)
	}
	if err := dB.Start(ctx); err != nil {
		t.Fatalf("start daemon B: %v", err)
	}
	defer dB.Stop()

	// 1. Control API must reject unauthenticated requests.
	apiCall(t, dA.ControlAddr(), "", http.MethodGet, "/v1/status", nil, http.StatusUnauthorized)

	// 2. Node A creates an Island and generates an invite token.
	islRaw := apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/islands", map[string]string{
		"name":       "Harbor Net",
		"visibility": "invite-only",
	}, http.StatusCreated)
	var islResp struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(islRaw, &islResp)
	if islResp.ID == "" {
		t.Fatalf("expected island ID, got %s", string(islRaw))
	}

	invRaw := apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/islands/invite", map[string]any{
		"island_id": islResp.ID,
	}, http.StatusOK)
	var invResp struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(invRaw, &invResp)

	// Node B joins the Island using the signed invite token.
	apiCall(t, dB.ControlAddr(), dB.Token(), http.MethodPost, "/v1/islands/join", map[string]string{
		"token": invResp.Token,
	}, http.StatusOK)

	// 3. Node A enables Beacon Mode; Node B discovers Node A via UDP multicast.
	apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/beacon", map[string]any{
		"mode":         "public",
		"island_id":    islResp.ID,
		"interval_sec": 1,
	}, http.StatusOK)

	var discovered []beacon.DiscoveredPeer
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		discRaw := apiCall(t, dB.ControlAddr(), dB.Token(), http.MethodGet, "/v1/discover", nil, http.StatusOK)
		_ = json.Unmarshal(discRaw, &discovered)
		if len(discovered) > 0 {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if len(discovered) == 0 {
		t.Fatalf("Node B did not discover Node A via Beacon multicast")
	}
	if discovered[0].Announcement.NodeID != dA.NodeID() {
		t.Fatalf("discovered unexpected node: %s", discovered[0].Announcement.NodeID)
	}

	// 4. Node B connects to Node A (using P2P addr) and sends an encrypted message.
	apiCall(t, dB.ControlAddr(), dB.Token(), http.MethodPost, "/v1/peers/connect", map[string]string{
		"addr": dA.P2PAddr(),
	}, http.StatusOK)

	apiCall(t, dB.ControlAddr(), dB.Token(), http.MethodPost, "/v1/messages/send", map[string]string{
		"to":   dA.NodeID(),
		"body": "Hello from Node B without internet!",
	}, http.StatusOK)

	// Verify Node A received the message and Node B received the chat.ack.
	var msgsA, msgsB []storage.MessageRecord
	msgDeadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(msgDeadline) {
		rawA := apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodGet, "/v1/messages", nil, http.StatusOK)
		rawB := apiCall(t, dB.ControlAddr(), dB.Token(), http.MethodGet, "/v1/messages", nil, http.StatusOK)
		_ = json.Unmarshal(rawA, &msgsA)
		_ = json.Unmarshal(rawB, &msgsB)
		if len(msgsA) > 0 && len(msgsB) > 0 && msgsB[0].Status == "acked" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if len(msgsA) == 0 || msgsA[0].Body != "Hello from Node B without internet!" {
		t.Fatalf("Node A did not receive expected message: %+v", msgsA)
	}
	if len(msgsB) == 0 || msgsB[0].Status != "acked" {
		t.Fatalf("Node B message was not acked: %+v", msgsB)
	}

	// 5. Node A revokes Node B; Node B can no longer connect.
	apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/peers/revoke", map[string]string{
		"node_id": dB.NodeID(),
	}, http.StatusOK)

	time.Sleep(100 * time.Millisecond)
	apiCall(t, dB.ControlAddr(), dB.Token(), http.MethodPost, "/v1/peers/connect", map[string]string{
		"addr": dA.P2PAddr(),
	}, http.StatusBadGateway)
}
