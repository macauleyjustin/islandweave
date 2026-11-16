package synchronization

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/islandweave/islandweave/core/identity"
	"github.com/islandweave/islandweave/core/storage"
	"github.com/islandweave/islandweave/protocols/iwbn"
)

func TestInventoryDiffAndApplyResponse(t *testing.T) {
	storeA, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer storeA.Close()

	storeB, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer storeB.Close()

	idA, _, err := identity.LoadOrCreateNodeIdentity(t.TempDir(), "node-a")
	if err != nil {
		t.Fatal(err)
	}
	bul, err := iwbn.CreateBulletin(idA, "local-news", "news", "Sync Title", "summary", "Sync Body", "", false, 1, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := storeA.SaveBulletin(bul); err != nil {
		t.Fatal(err)
	}

	invRaw, err := BuildInventory(storeA)
	if err != nil {
		t.Fatal(err)
	}
	var inv InventoryPayload
	if err := json.Unmarshal(invRaw, &inv); err != nil {
		t.Fatal(err)
	}

	req, err := DiffInventory(storeB, inv)
	if err != nil {
		t.Fatal(err)
	}
	if len(req.BulletinIDs) != 1 || req.BulletinIDs[0] != bul.BulletinID {
		t.Fatalf("expected missing bulletin %s in diff request, got %+v", bul.BulletinID, req)
	}

	respRaw, err := BuildResponse(storeA, req)
	if err != nil {
		t.Fatal(err)
	}
	var resp ResponsePayload
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		t.Fatal(err)
	}

	eng := NewEngine()
	_, bN, err := eng.ApplyResponse(storeB, idA.ID, resp, time.Now())
	if err != nil || bN != 1 {
		t.Fatalf("ApplyResponse failed: bN=%d err=%v", bN, err)
	}
	got, err := storeB.ListBulletins("local-news", 10)
	if err != nil || len(got) != 1 || got[0].Title != "Sync Title" {
		t.Fatalf("expected synced bulletin on storeB, got %+v err=%v", got, err)
	}
}
