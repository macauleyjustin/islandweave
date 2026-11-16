package daemon

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/islandweave/islandweave/core/security"
	"github.com/islandweave/islandweave/transports/relay"
)

func (d *Daemon) handleRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, d.ComputeRoutes())
}

func (d *Daemon) handleDiscoverMerged(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var beacons = d.beacon.Discovered()
	writeJSON(w, http.StatusOK, d.disc.MergedList(beacons))
}

func (d *Daemon) handleSyncNow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		PeerID string `json:"peer_id"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	n, err := d.SyncNow(req.PeerID)
	if err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "sync_triggered",
		"synced_links": n,
	})
}

func (d *Daemon) handleSyncHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, d.syncEng.History(50))
}

func (d *Daemon) handleShareCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Dir      string `json:"dir"`
		Name     string `json:"name"`
		IslandID string `json:"island_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Dir == "" {
		writeErr(w, http.StatusBadRequest, "dir is required")
		return
	}
	rec, manifest, err := d.ShareDirectory(req.Dir, req.Name, req.IslandID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"service":  rec,
		"manifest": manifest,
		"url":      "http://" + d.GatewayAddr() + "/site/" + rec.Name + "/",
	})
}

func (d *Daemon) handleGatewayList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	svcs, err := d.store.ListServices()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	gwBase := "http://" + d.GatewayAddr()
	entries := make([]map[string]any, 0, len(svcs))
	for _, s := range svcs {
		entries = append(entries, map[string]any{
			"service_id":   s.ServiceID,
			"name":         s.Name,
			"type":         s.Type,
			"site_version": s.SiteVersion,
			"is_local":     s.IsLocal,
			"gateway_url":  gwBase + "/site/" + s.Name + "/",
			"island_uri":   "island://" + s.Name + "/",
		})
	}
	d.mu.RLock()
	relayOn := d.cfg.RelayEnabled
	d.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"gateway_addr": d.GatewayAddr(),
		"relay":        relay.BuildInfo(relayOn),
		"endpoints":    entries,
	})
}

func (d *Daemon) handleSecurityStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	keyPath := filepath.Join(d.cfg.DataDir, "keys", "node.json")
	keyMode := "unknown"
	if st, err := os.Stat(keyPath); err == nil {
		keyMode = st.Mode().Perm().String()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"node_id":      d.id.ID,
		"key_storage":  "local-0600-file",
		"key_file":     keyPath,
		"key_mode":     keyMode,
		"permissions":  d.sec.Permissions(),
		"audit_count":  len(d.sec.AuditEvents(200)),
		"control_auth": "bearer-256bit",
	})
}

func (d *Daemon) handleSecurityAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, d.sec.AuditEvents(100))
}

func (d *Daemon) handleSecurityPermissions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, d.sec.Permissions())
	case http.MethodPost:
		var req security.Permissions
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		d.sec.SetPermissions(req)
		writeJSON(w, http.StatusOK, d.sec.Permissions())
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Daemon) handleBandwidth(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, d.TrafficSnapshot())
	case http.MethodPost:
		var req struct {
			BandwidthLimitHour int64 `json:"bandwidth_limit_hour"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		d.sec.SetBandwidthLimitBytes(req.BandwidthLimitHour)
		writeJSON(w, http.StatusOK, d.TrafficSnapshot())
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Daemon) handleActivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	list, err := d.store.ListActivity(100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (d *Daemon) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var dbBytes int64
	if st, err := os.Stat(filepath.Join(d.cfg.DataDir, "islandweave.db")); err == nil {
		dbBytes = st.Size()
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	d.mu.RLock()
	activeLinks := len(d.links)
	relayOn := d.cfg.RelayEnabled
	d.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"os":           runtime.GOOS,
		"arch":         runtime.GOARCH,
		"go_version":   runtime.Version(),
		"goroutines":   runtime.NumGoroutine(),
		"alloc_bytes":  m.Alloc,
		"uptime_sec":   int64(time.Since(d.startedAt).Seconds()),
		"db_bytes":     dbBytes,
		"data_dir":     d.cfg.DataDir,
		"active_links": activeLinks,
		"relay":        relay.BuildInfo(relayOn),
		"traffic":      d.sec.Traffic.Snapshot(),
		"bootstraps":   d.disc.BootstrapAddrs(),
	})
}

func (d *Daemon) handleNavBookmarks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := d.store.ListBookmarks()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, list)
	case http.MethodPost:
		var req struct {
			URI   string `json:"uri"`
			Title string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URI == "" {
			writeErr(w, http.StatusBadRequest, "uri is required")
			return
		}
		if err := d.store.SaveBookmark(req.URI, req.Title); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, req)
	case http.MethodDelete:
		uri := r.URL.Query().Get("uri")
		if uri == "" {
			writeErr(w, http.StatusBadRequest, "uri query param is required")
			return
		}
		_ = d.store.DeleteBookmark(uri)
		writeJSON(w, http.StatusOK, map[string]string{"deleted": uri})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Daemon) handleNavHistory(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := d.store.ListHistory(50)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, list)
	case http.MethodPost:
		var req struct {
			URI   string `json:"uri"`
			Title string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URI == "" {
			writeErr(w, http.StatusBadRequest, "uri is required")
			return
		}
		_ = d.store.AddHistory(req.URI, req.Title)
		writeJSON(w, http.StatusCreated, req)
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Daemon) handleSatelliteSimulate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		SatelliteName string `json:"satellite_name"`
		MaxBytes      int64  `json:"max_bytes"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	res, err := d.RecordSimulatedSatellitePass(req.SatelliteName, req.MaxBytes)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}
