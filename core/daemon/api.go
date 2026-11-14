package daemon

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"github.com/islandweave/islandweave/core/beacon"
	"github.com/islandweave/islandweave/core/identity"
	"github.com/islandweave/islandweave/core/islands"
	"github.com/islandweave/islandweave/core/storage"
)

func (d *Daemon) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/status", d.handleStatus)
	mux.HandleFunc("/v1/islands", d.handleIslands)
	mux.HandleFunc("/v1/islands/invite", d.handleIslandInvite)
	mux.HandleFunc("/v1/islands/join", d.handleIslandJoin)
	mux.HandleFunc("/v1/islands/leave", d.handleIslandLeave)
	mux.HandleFunc("/v1/peers", d.handlePeers)
	mux.HandleFunc("/v1/peers/connect", d.handlePeerConnect)
	mux.HandleFunc("/v1/peers/trust", d.handlePeerTrust)
	mux.HandleFunc("/v1/peers/revoke", d.handlePeerRevoke)
	mux.HandleFunc("/v1/discover", d.handleDiscover)
	mux.HandleFunc("/v1/beacon", d.handleBeacon)
	mux.HandleFunc("/v1/messages", d.handleMessages)
	mux.HandleFunc("/v1/messages/send", d.handleMessageSend)
	mux.HandleFunc("/v1/services", d.handleServices)
	mux.HandleFunc("/v1/services/host", d.handleServiceHost)
	mux.HandleFunc("/v1/services/fetch", d.handleServiceFetch)
	mux.HandleFunc("/v1/transfers", d.handleTransfers)
	mux.HandleFunc("/v1/transfers/send", d.handleTransferSend)
	mux.HandleFunc("/v1/transfers/cancel", d.handleTransferCancel)
	mux.HandleFunc("/v1/peers/import-key", d.handlePeerImportKey)
	mux.HandleFunc("/v1/courier/status", d.handleCourierStatus)
	mux.HandleFunc("/v1/courier/queue", d.handleCourierQueue)
	mux.HandleFunc("/v1/courier/send", d.handleCourierSend)
	mux.HandleFunc("/v1/courier/export", d.handleCourierExport)
	mux.HandleFunc("/v1/courier/import", d.handleCourierImport)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func (d *Daemon) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	isl, _ := d.store.ListIslands()
	peers, _ := d.store.ListPeers()
	d.mu.RLock()
	activeLinks := len(d.links)
	d.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"node_id":        d.id.ID,
		"node_name":      d.id.Name,
		"public_key":     base64.StdEncoding.EncodeToString(d.id.PublicKey),
		"enc_public_key": base64.StdEncoding.EncodeToString(d.id.EncPublicKey),
		"p2p_addr":       d.P2PAddr(),
		"control_addr": d.ControlAddr(),
		"gateway_addr": d.GatewayAddr(),
		"islands":      len(isl),
		"known_peers":  len(peers),
		"active_links": activeLinks,
		"beacon":       d.beacon.Status(),
	})
}

func (d *Daemon) handleIslands(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := d.store.ListIslands()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, list)
	case http.MethodPost:
		var req struct {
			Name       string             `json:"name"`
			Visibility islands.Visibility `json:"visibility"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		isl, err := islands.CreateIsland(req.Name, req.Visibility)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := d.store.SaveIsland(isl); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, isl)
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Daemon) handleIslandInvite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		IslandID       string   `json:"island_id"`
		TTLSeconds     int64    `json:"ttl_seconds"`
		BootstrapAddrs []string `json:"bootstrap_addrs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	isl, err := d.store.GetIsland(req.IslandID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "island not found")
		return
	}
	addrs := req.BootstrapAddrs
	if len(addrs) == 0 {
		addrs = []string{d.P2PAddr()}
	}
	ttl := time.Duration(req.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	tok, err := islands.CreateInvite(isl, d.id.ID, addrs, ttl, 0)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	encoded, err := islands.EncodeInvite(tok)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":  encoded,
		"invite": tok,
	})
}

func (d *Daemon) handleIslandJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	tok, err := islands.DecodeInvite(req.Token)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	isl, err := islands.VerifyInvite(tok, time.Now())
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := d.store.SaveIsland(isl); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"island":          isl,
		"bootstrap_addrs": tok.BootstrapAddrs,
	})
}

func (d *Daemon) handleIslandLeave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		IslandID string `json:"island_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := d.store.DeleteIsland(req.IslandID); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "left", "island_id": req.IslandID})
}

func (d *Daemon) handlePeers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	peers, err := d.store.ListPeers()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.mu.RLock()
	type peerView struct {
		*storage.PeerRecord
		Connected bool `json:"connected"`
	}
	out := make([]peerView, 0, len(peers))
	for _, p := range peers {
		_, conn := d.links[p.NodeID]
		out = append(out, peerView{PeerRecord: p, Connected: conn})
	}
	d.mu.RUnlock()
	writeJSON(w, http.StatusOK, out)
}

func (d *Daemon) handlePeerConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Addr string `json:"addr"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Addr == "" {
		writeErr(w, http.StatusBadRequest, "addr is required")
		return
	}
	link, err := d.ConnectPeer(r.Context(), req.Addr)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"peer_id":        link.PeerID(),
		"shared_islands": link.SharedIslands(),
		"capabilities":   link.Capabilities(),
	})
}

func (d *Daemon) handlePeerTrust(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		NodeID string `json:"node_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.NodeID == "" {
		writeErr(w, http.StatusBadRequest, "node_id is required")
		return
	}
	if err := d.store.SetPeerTrust(req.NodeID, true, false); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "trusted", "node_id": req.NodeID})
}

func (d *Daemon) handlePeerRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		NodeID string `json:"node_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.NodeID == "" {
		writeErr(w, http.StatusBadRequest, "node_id is required")
		return
	}
	if err := d.store.SetPeerTrust(req.NodeID, false, true); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	d.mu.Lock()
	if link, ok := d.links[req.NodeID]; ok {
		_ = link.Close()
		delete(d.links, req.NodeID)
	}
	d.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked", "node_id": req.NodeID})
}

func (d *Daemon) handleDiscover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, d.beacon.Discovered())
}

func (d *Daemon) handleBeacon(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, d.beacon.Status())
	case http.MethodPost:
		var req struct {
			Mode        beacon.Mode `json:"mode"`
			IslandID    string      `json:"island_id"`
			IntervalSec int         `json:"interval_sec"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		var islandName string
		if req.IslandID != "" {
			if isl, err := d.store.GetIsland(req.IslandID); err == nil {
				islandName = isl.Name
			}
		}
		d.beacon.UpdateConfig(func(c *beacon.Config) {
			if req.Mode != "" {
				c.Mode = req.Mode
			}
			if req.IslandID != "" {
				c.IslandID = req.IslandID
				c.IslandName = islandName
			}
			if req.IntervalSec > 0 {
				c.Interval = time.Duration(req.IntervalSec) * time.Second
			}
		})
		writeJSON(w, http.StatusOK, d.beacon.Status())
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Daemon) handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	msgs, err := d.store.ListMessages(100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, msgs)
}

func (d *Daemon) handleMessageSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		To       string `json:"to"`
		IslandID string `json:"island_id"`
		Body     string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.To == "" || req.Body == "" {
		writeErr(w, http.StatusBadRequest, "to and body are required")
		return
	}
	rec, err := d.SendMessage(r.Context(), req.To, req.IslandID, req.Body)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func (d *Daemon) handleServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	list, err := d.store.ListServices()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (d *Daemon) handleServiceHost(w http.ResponseWriter, r *http.Request) {
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
	rec, manifest, err := d.HostSite(req.Dir, req.Name, req.IslandID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"service":     rec,
		"files_count": len(manifest.Files),
		"gateway_url": "http://" + d.GatewayAddr() + "/site/" + rec.Name + "/",
	})
}

func (d *Daemon) handleServiceFetch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		NameOrID string `json:"name_or_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.NameOrID == "" {
		writeErr(w, http.StatusBadRequest, "name_or_id is required")
		return
	}
	if err := d.FetchSite(r.Context(), req.NameOrID); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":      "cached",
		"gateway_url": "http://" + d.GatewayAddr() + "/site/" + req.NameOrID + "/",
	})
}

func (d *Daemon) handleTransfers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	list, err := d.store.ListTransfers(100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (d *Daemon) handleTransferSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		To       string `json:"to"`
		FilePath string `json:"file_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.To == "" || req.FilePath == "" {
		writeErr(w, http.StatusBadRequest, "to and file_path are required")
		return
	}
	tr, err := d.SendFile(r.Context(), req.To, req.FilePath)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tr)
}

func (d *Daemon) handleTransferCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		TransferID string `json:"transfer_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TransferID == "" {
		writeErr(w, http.StatusBadRequest, "transfer_id is required")
		return
	}
	if err := d.CancelTransfer(req.TransferID); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "canceled", "transfer_id": req.TransferID})
}

func (d *Daemon) handlePeerImportKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Name         string `json:"name"`
		PublicKey    string `json:"public_key"`
		EncPublicKey string `json:"enc_public_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PublicKey == "" || req.EncPublicKey == "" {
		writeErr(w, http.StatusBadRequest, "public_key and enc_public_key are required")
		return
	}
	pub, err := base64.StdEncoding.DecodeString(req.PublicKey)
	if err != nil || len(pub) != 32 {
		writeErr(w, http.StatusBadRequest, "invalid public_key")
		return
	}
	encPub, err := base64.StdEncoding.DecodeString(req.EncPublicKey)
	if err != nil || len(encPub) != 32 {
		writeErr(w, http.StatusBadRequest, "invalid enc_public_key")
		return
	}
	nodeID := identity.DeriveNodeID(pub)
	if err := d.store.UpsertPeer(&storage.PeerRecord{
		NodeID:       nodeID,
		Name:         req.Name,
		PublicKey:    pub,
		EncPublicKey: encPub,
		LastSeen:     time.Now().Unix(),
		Source:       "manual-key",
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"node_id": nodeID, "status": "imported"})
}

func (d *Daemon) handleCourierStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	used, _ := d.store.CourierStorageUsed()
	fwd, _ := d.store.ListForwardableDTNBundles(time.Now().Unix())
	writeJSON(w, http.StatusOK, map[string]any{
		"forwarding_enabled": d.courierForwardingEnabled(),
		"storage_used_bytes": used,
		"storage_quota":      d.maxCourierBytes(),
		"forwardable_count":  len(fwd),
	})
}

func (d *Daemon) handleCourierQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	list, err := d.store.ListDTNBundles(100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (d *Daemon) handleCourierSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		To         string `json:"to"`
		IslandID   string `json:"island_id"`
		Body       string `json:"body"`
		Priority   int    `json:"priority"`
		MaxHops    int    `json:"max_hops"`
		TTLSeconds int64  `json:"ttl_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.To == "" || req.Body == "" {
		writeErr(w, http.StatusBadRequest, "to and body are required")
		return
	}
	ttl := time.Duration(req.TTLSeconds) * time.Second
	rec, err := d.QueueDTNMessage(req.To, req.IslandID, req.Body, req.Priority, req.MaxHops, ttl)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func (d *Daemon) handleCourierExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		FilePath string `json:"file_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FilePath == "" {
		writeErr(w, http.StatusBadRequest, "file_path is required")
		return
	}
	n, err := d.ExportCourierPack(req.FilePath)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"exported_bundles": n, "file_path": req.FilePath})
}

func (d *Daemon) handleCourierImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		FilePath string `json:"file_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FilePath == "" {
		writeErr(w, http.StatusBadRequest, "file_path is required")
		return
	}
	delivered, carrying, skipped, err := d.ImportCourierPack(req.FilePath)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"delivered": delivered,
		"carrying":  carrying,
		"skipped":   skipped,
	})
}
