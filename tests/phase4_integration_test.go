package tests

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/islandweave/islandweave/core/daemon"
	"github.com/islandweave/islandweave/protocols/iwbn"
	"github.com/islandweave/islandweave/protocols/iwmp"
)

func TestPhase4MailAndBulletinThroughCourier(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dirC := t.TempDir()
	dCInit, err := daemon.New(daemon.Config{
		DataDir:       dirC,
		NodeName:      "mail-carol",
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
	_ = dCInit.Stop()

	// Start Alice (sender) and PostOffice (courier)
	dA, err := daemon.New(daemon.Config{
		DataDir:       t.TempDir(),
		NodeName:      "mail-alice",
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

	dPO, err := daemon.New(daemon.Config{
		DataDir:       t.TempDir(),
		NodeName:      "post-office",
		ListenAddr:    "127.0.0.1:0",
		ControlAddr:   "127.0.0.1:0",
		GatewayAddr:   "127.0.0.1:0",
		DisableBeacon: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := dPO.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer dPO.Stop()

	islRaw := apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/islands", map[string]string{"name": "MailIsland"}, http.StatusCreated)
	var isl struct{ ID string `json:"id"` }
	_ = json.Unmarshal(islRaw, &isl)
	invRaw := apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/islands/invite", map[string]any{"island_id": isl.ID}, http.StatusOK)
	var inv struct{ Token string `json:"token"` }
	_ = json.Unmarshal(invRaw, &inv)

	apiCall(t, dPO.ControlAddr(), dPO.Token(), http.MethodPost, "/v1/islands/join", map[string]string{"token": inv.Token}, http.StatusOK)

	apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/peers/import-key", map[string]string{
		"name":           "mail-carol",
		"public_key":     statusC.PublicKey,
		"enc_public_key": statusC.EncPublicKey,
	}, http.StatusOK)

	// 1. Alice saves a draft offline, then sends mail with an attachment
	apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/mail/send", map[string]any{
		"to":      statusC.NodeID,
		"subject": "Draft Subject",
		"body":    "Work in progress",
		"draft":   true,
	}, http.StatusOK)

	draftsRaw := apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodGet, "/v1/mail?folder=drafts", nil, http.StatusOK)
	var drafts []iwmp.MailRecord
	_ = json.Unmarshal(draftsRaw, &drafts)
	if len(drafts) != 1 {
		t.Fatalf("expected 1 draft, got %d", len(drafts))
	}

	attData := base64.StdEncoding.EncodeToString([]byte("harbor-schedule-v1"))
	apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/mail/send", map[string]any{
		"to":        statusC.NodeID,
		"island_id": isl.ID,
		"subject":   "Ferry Schedule",
		"body":      "Attached is the updated ferry schedule.",
		"attachments": []iwmp.Attachment{{
			Name:        "schedule.txt",
			ContentType: "text/plain",
			Size:        18,
			DataB64:     attData,
		}},
	}, http.StatusOK)

	// 2. Alice publishes a community bulletin (and claims OfficialEmergency=true, which Carol must NOT treat as verified!)
	apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/bulletins/publish", map[string]any{
		"channel":            "community-news",
		"category":           "notice",
		"title":              "Winter Dock Maintenance",
		"summary":            "Dock 2 closed Tuesday",
		"body":               "Dock 2 will be closed for repairs on Tuesday.",
		"island_id":          isl.ID,
		"official_emergency": true,
	}, http.StatusCreated)

	// 3. Alice connects to Post Office B, hands off encrypted mail + bulletin, then Alice disconnects!
	apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/peers/connect", map[string]string{"addr": dPO.P2PAddr()}, http.StatusOK)
	time.Sleep(300 * time.Millisecond)

	// Export physical courier pack from Post Office B as well, and stop Alice
	packPath := filepath.Join(t.TempDir(), "post.iwcourier")
	apiCall(t, dPO.ControlAddr(), dPO.Token(), http.MethodPost, "/v1/courier/export", map[string]string{"file_path": packPath}, http.StatusOK)
	_ = dA.Stop()

	// 4. Carol starts up offline and imports the courier pack (or connects to Post Office B)
	dC, err := daemon.New(daemon.Config{
		DataDir:       dirC,
		NodeName:      "mail-carol",
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

	apiCall(t, dC.ControlAddr(), dC.Token(), http.MethodPost, "/v1/courier/import", map[string]string{"file_path": packPath}, http.StatusOK)

	inboxRaw := apiCall(t, dC.ControlAddr(), dC.Token(), http.MethodGet, "/v1/mail?folder=inbox", nil, http.StatusOK)
	var inbox []iwmp.MailRecord
	_ = json.Unmarshal(inboxRaw, &inbox)
	if len(inbox) != 1 || inbox[0].Subject != "Ferry Schedule" || len(inbox[0].Attachments) != 1 {
		t.Fatalf("Carol did not receive expected mail with attachment: %+v", inbox)
	}

	bulRaw := apiCall(t, dC.ControlAddr(), dC.Token(), http.MethodGet, "/v1/bulletins?channel=community-news", nil, http.StatusOK)
	var bulletins []iwbn.Bulletin
	_ = json.Unmarshal(bulRaw, &bulletins)
	if len(bulletins) != 1 || bulletins[0].Title != "Winter Dock Maintenance" {
		t.Fatalf("Carol did not receive expected bulletin: %+v", bulletins)
	}

	// Verify unauthenticated official emergency claim is rejected
	if iwbn.IsVerifiedEmergency(&bulletins[0], map[string]bool{}) {
		t.Fatalf("ordinary publisher must not be verified as official emergency authority")
	}
}
