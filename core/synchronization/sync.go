package synchronization

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/islandweave/islandweave/core/services"
	"github.com/islandweave/islandweave/core/storage"
	"github.com/islandweave/islandweave/protocols/iwbn"
)

const (
	TypeSyncInventory = "sync.inv"
	TypeSyncRequest   = "sync.req"
	TypeSyncResponse  = "sync.resp"
)

// ServiceDigest summarizes a known hosted/cached ServiceRecord version.
type ServiceDigest struct {
	ServiceID   string `json:"service_id"`
	SiteVersion int    `json:"site_version"`
}

// BulletinDigest summarizes a known IWBN bulletin version.
type BulletinDigest struct {
	BulletinID      string `json:"bulletin_id"`
	BulletinVersion int    `json:"bulletin_version"`
}

// InventoryPayload advertises local services and bulletins for anti-entropy diffing (IWP-SYNC).
type InventoryPayload struct {
	Services  []ServiceDigest  `json:"services,omitempty"`
	Bulletins []BulletinDigest `json:"bulletins,omitempty"`
}

// RequestPayload asks a peer for specific missing or outdated records.
type RequestPayload struct {
	ServiceIDs  []string `json:"service_ids,omitempty"`
	BulletinIDs []string `json:"bulletin_ids,omitempty"`
}

// ResponsePayload carries verified ServiceRecords and Bulletins requested by a peer.
type ResponsePayload struct {
	Services  []services.ServiceRecord `json:"services,omitempty"`
	Bulletins []iwbn.Bulletin          `json:"bulletins,omitempty"`
}

// SyncEvent records a completed synchronization exchange for audit/UI history.
type SyncEvent struct {
	Timestamp       int64  `json:"timestamp"`
	PeerID          string `json:"peer_id"`
	ServicesSynced  int    `json:"services_synced"`
	BulletinsSynced int    `json:"bulletins_synced"`
	Status          string `json:"status"`
	Detail          string `json:"detail,omitempty"`
}

// Engine executes anti-entropy inventory diffing and maintains bounded sync history.
type Engine struct {
	mu      sync.RWMutex
	history []SyncEvent
	maxHist int
}

// NewEngine initializes an IWP-SYNC Engine.
func NewEngine() *Engine {
	return &Engine{maxHist: 100}
}

// BuildInventory constructs an InventoryPayload from local SQLite state.
func BuildInventory(store *storage.Store) ([]byte, error) {
	svcs, err := store.ListServices()
	if err != nil {
		return nil, err
	}
	buls, err := store.ListBulletins("", 200)
	if err != nil {
		return nil, err
	}
	var inv InventoryPayload
	for _, s := range svcs {
		inv.Services = append(inv.Services, ServiceDigest{
			ServiceID:   s.ServiceID,
			SiteVersion: s.SiteVersion,
		})
	}
	for _, b := range buls {
		inv.Bulletins = append(inv.Bulletins, BulletinDigest{
			BulletinID:      b.BulletinID,
			BulletinVersion: b.BulletinVersion,
		})
	}
	return json.Marshal(inv)
}

// DiffInventory identifies items offered by a remote peer that are missing locally or have newer versions.
func DiffInventory(store *storage.Store, remote InventoryPayload) (RequestPayload, error) {
	localSvcs, err := store.ListServices()
	if err != nil {
		return RequestPayload{}, err
	}
	localBuls, err := store.ListBulletins("", 500)
	if err != nil {
		return RequestPayload{}, err
	}

	svcVer := make(map[string]int, len(localSvcs))
	for _, s := range localSvcs {
		svcVer[s.ServiceID] = s.SiteVersion
	}
	bulVer := make(map[string]int, len(localBuls))
	for _, b := range localBuls {
		bulVer[b.BulletinID] = b.BulletinVersion
	}

	var req RequestPayload
	for _, rs := range remote.Services {
		if lv, ok := svcVer[rs.ServiceID]; !ok || rs.SiteVersion > lv {
			req.ServiceIDs = append(req.ServiceIDs, rs.ServiceID)
		}
	}
	for _, rb := range remote.Bulletins {
		if lv, ok := bulVer[rb.BulletinID]; !ok || rb.BulletinVersion > lv {
			req.BulletinIDs = append(req.BulletinIDs, rb.BulletinID)
		}
	}
	return req, nil
}

// BuildResponse collects requested verified records from local SQLite state.
func BuildResponse(store *storage.Store, req RequestPayload) ([]byte, error) {
	var resp ResponsePayload
	if len(req.ServiceIDs) > 0 {
		wantSvc := make(map[string]bool, len(req.ServiceIDs))
		for _, id := range req.ServiceIDs {
			wantSvc[id] = true
		}
		svcs, err := store.ListServices()
		if err != nil {
			return nil, err
		}
		for _, s := range svcs {
			if wantSvc[s.ServiceID] {
				resp.Services = append(resp.Services, s.ServiceRecord)
			}
		}
	}
	if len(req.BulletinIDs) > 0 {
		wantBul := make(map[string]bool, len(req.BulletinIDs))
		for _, id := range req.BulletinIDs {
			wantBul[id] = true
		}
		buls, err := store.ListBulletins("", 500)
		if err != nil {
			return nil, err
		}
		for _, b := range buls {
			if wantBul[b.BulletinID] {
				resp.Bulletins = append(resp.Bulletins, *b)
			}
		}
	}
	return json.Marshal(resp)
}

// ApplyResponse verifies and persists synced ServiceRecords and Bulletins.
func (e *Engine) ApplyResponse(store *storage.Store, peerID string, resp ResponsePayload, now time.Time) (svcApplied, bulApplied int, err error) {
	for i := range resp.Services {
		rec := resp.Services[i]
		if verr := services.VerifyServiceRecord(&rec, now); verr != nil {
			continue
		}
		if ser := store.SaveService(&rec, false); ser == nil {
			svcApplied++
		}
	}
	for i := range resp.Bulletins {
		b := resp.Bulletins[i]
		if verr := iwbn.VerifyBulletin(&b, now); verr != nil {
			continue
		}
		if ser := store.SaveBulletin(&b); ser == nil {
			bulApplied++
		}
	}
	e.RecordEvent(SyncEvent{
		Timestamp:       now.Unix(),
		PeerID:          peerID,
		ServicesSynced:  svcApplied,
		BulletinsSynced: bulApplied,
		Status:          "synced",
		Detail:          fmt.Sprintf("applied %d services and %d bulletins via IWP-SYNC", svcApplied, bulApplied),
	})
	return svcApplied, bulApplied, nil
}

func (e *Engine) RecordEvent(ev SyncEvent) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if ev.Timestamp == 0 {
		ev.Timestamp = time.Now().Unix()
	}
	e.history = append(e.history, ev)
	if len(e.history) > e.maxHist {
		e.history = e.history[len(e.history)-e.maxHist:]
	}
}

func (e *Engine) History(limit int) []SyncEvent {
	e.mu.RLock()
	defer e.mu.RUnlock()
	n := len(e.history)
	if limit <= 0 || limit > n {
		limit = n
	}
	out := make([]SyncEvent, limit)
	for i := 0; i < limit; i++ {
		out[i] = e.history[n-1-i]
	}
	return out
}
