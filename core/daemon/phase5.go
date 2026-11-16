package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/islandweave/islandweave/core/beacon"
	"github.com/islandweave/islandweave/core/discovery"
	"github.com/islandweave/islandweave/core/routing"
	"github.com/islandweave/islandweave/core/storage"
	"github.com/islandweave/islandweave/core/synchronization"
	"github.com/islandweave/islandweave/core/transport"
	"github.com/islandweave/islandweave/protocols/dtn"
	"github.com/islandweave/islandweave/protocols/iwst"
)

// ComputeRoutes builds the daemon's current multi-transport routing table via IWP-ROUTE.
func (d *Daemon) ComputeRoutes() []routing.RouteEntry {
	peers, _ := d.store.ListPeers()
	dtnList, _ := d.store.ListDTNBundles(500)
	var beacons []beacon.DiscoveredPeer
	if d.beacon != nil {
		beacons = d.beacon.Discovered()
	}

	d.mu.RLock()
	active := make(map[string]bool, len(d.links))
	for id := range d.links {
		active[id] = true
	}
	torCfg := d.cfg.Tor
	d.mu.RUnlock()

	return routing.ComputeRoutes(peers, active, beacons, dtnList, torCfg)
}

func (d *Daemon) startPeerSyncAndDiscovery(link *transport.Link) {
	perms := d.sec.Permissions()
	if perms.AllowAutoSync {
		_ = d.sendSyncInventory(link)
	}
	if perms.AllowPeerExchange {
		peers, err := d.store.ListPeers()
		if err == nil && len(peers) > 0 {
			if payload, err := d.disc.BuildExchangePayload(peers); err == nil {
				_ = link.SendEnvelope(&transport.Envelope{
					Version:   transport.ProtocolVersion,
					Type:      discovery.TypePeerExchange,
					ID:        uuid.NewString(),
					SenderID:  d.id.ID,
					Timestamp: time.Now().Unix(),
					Payload:   payload,
				})
			}
		}
	}
}

func (d *Daemon) sendSyncInventory(link *transport.Link) error {
	invBytes, err := synchronization.BuildInventory(d.store)
	if err != nil {
		return err
	}
	return link.SendEnvelope(&transport.Envelope{
		Version:   transport.ProtocolVersion,
		Type:      synchronization.TypeSyncInventory,
		ID:        uuid.NewString(),
		SenderID:  d.id.ID,
		Timestamp: time.Now().Unix(),
		Payload:   invBytes,
	})
}

// SyncNow triggers immediate IWP-SYNC inventory exchange with peerID (or all active links if peerID == "").
func (d *Daemon) SyncNow(peerID string) (int, error) {
	d.mu.RLock()
	var targets []*transport.Link
	if peerID != "" {
		if l, ok := d.links[peerID]; ok {
			targets = append(targets, l)
		}
	} else {
		for _, l := range d.links {
			targets = append(targets, l)
		}
	}
	d.mu.RUnlock()

	if len(targets) == 0 {
		return 0, fmt.Errorf("no matching active peer links to synchronize")
	}
	for _, l := range targets {
		_ = d.sendSyncInventory(l)
		go d.syncDTNBundles(l)
		go d.syncBulletins(l)
	}
	_ = d.store.RecordActivity("sync", fmt.Sprintf("Triggered IWP-SYNC across %d active link(s)", len(targets)), peerID, "ok")
	return len(targets), nil
}

func (d *Daemon) handlePhase5Envelope(link *transport.Link, env *transport.Envelope) bool {
	switch env.Type {
	case discovery.TypePeerExchange:
		if !d.sec.Permissions().AllowPeerExchange {
			return true
		}
		var p discovery.ExchangePayload
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			return true
		}
		accepted := d.disc.RecordHints(p.Peers)
		for _, h := range accepted {
			if _, err := d.store.GetPeer(h.NodeID); err != nil {
				_ = d.store.UpsertPeer(&storage.PeerRecord{
					NodeID:   h.NodeID,
					Name:     h.NodeName,
					Addrs:    h.Addrs,
					Islands:  h.Islands,
					LastSeen: h.LastSeen,
					Source:   "peer-exchange",
				})
			}
		}
		return true

	case synchronization.TypeSyncInventory:
		var inv synchronization.InventoryPayload
		if err := json.Unmarshal(env.Payload, &inv); err != nil {
			return true
		}
		req, err := synchronization.DiffInventory(d.store, inv)
		if err != nil || (len(req.ServiceIDs) == 0 && len(req.BulletinIDs) == 0) {
			return true
		}
		rawReq, _ := json.Marshal(req)
		_ = link.SendEnvelope(&transport.Envelope{
			Version:   transport.ProtocolVersion,
			Type:      synchronization.TypeSyncRequest,
			ID:        uuid.NewString(),
			SenderID:  d.id.ID,
			Timestamp: time.Now().Unix(),
			Payload:   rawReq,
		})
		return true

	case synchronization.TypeSyncRequest:
		var req synchronization.RequestPayload
		if err := json.Unmarshal(env.Payload, &req); err != nil {
			return true
		}
		rawResp, err := synchronization.BuildResponse(d.store, req)
		if err != nil {
			return true
		}
		_ = link.SendEnvelope(&transport.Envelope{
			Version:   transport.ProtocolVersion,
			Type:      synchronization.TypeSyncResponse,
			ID:        uuid.NewString(),
			SenderID:  d.id.ID,
			Timestamp: time.Now().Unix(),
			Payload:   rawResp,
		})
		return true

	case synchronization.TypeSyncResponse:
		var resp synchronization.ResponsePayload
		if err := json.Unmarshal(env.Payload, &resp); err != nil {
			return true
		}
		svcN, bulN, _ := d.syncEng.ApplyResponse(d.store, link.PeerID(), resp, time.Now())
		// Backfill missing chunks/manifests for newly synced remote services in background
		for _, s := range resp.Services {
			svcName := s.ServiceID
			go func(id string) {
				ctx, cancel := context.WithTimeout(d.runCtx, 15*time.Second)
				defer cancel()
				_ = d.FetchSite(ctx, id)
			}(svcName)
		}
		if svcN > 0 || bulN > 0 {
			_ = d.store.RecordActivity("sync", fmt.Sprintf("Synced %d service(s) and %d bulletin(s)", svcN, bulN), link.PeerID(), "ok")
		}
		return true
	}
	return false
}

// RecordSimulatedSatellitePass executes an IWST pass-budget check over queued local DTN bundles and records the PassLog.
func (d *Daemon) RecordSimulatedSatellitePass(satName string, maxBytes int64) (*iwst.PassLog, error) {
	if satName == "" {
		satName = "IW-SIM-LEO-1"
	}
	if maxBytes <= 0 {
		maxBytes = 576000
	}
	bundles, err := d.ExportForwardableBundles()
	if err != nil {
		return nil, err
	}
	var sentBytes int64
	var sentBundles int
	interrupted := false
	for _, b := range bundles {
		raw, _ := json.Marshal(b)
		n := int64(len(raw))
		if sentBytes+n <= maxBytes {
			sentBytes += n
			sentBundles++
		} else {
			sentBytes = maxBytes
			interrupted = true
			break
		}
	}
	logEntry := &iwst.PassLog{
		PassID:           "pass-" + uuid.NewString()[:8],
		SatelliteName:    satName,
		BytesTransferred: sentBytes,
		BundlesDelivered: sentBundles,
		Interrupted:      interrupted,
		CompletedAt:      time.Now().Unix(),
	}
	if err := d.store.SavePassLog(logEntry); err != nil {
		return nil, err
	}
	_ = d.store.RecordActivity("satellite", fmt.Sprintf("IWST pass %s: %d bytes (%d bundles queued/accounted)", logEntry.PassID, sentBytes, sentBundles), "", "ok")
	return logEntry, nil
}

// TrafficSnapshot returns the current P2P byte & link counters.
func (d *Daemon) TrafficSnapshot() map[string]any {
	snap := d.sec.Traffic.Snapshot()
	return map[string]any{
		"bytes_sent":           snap.BytesSent,
		"bytes_recv":           snap.BytesRecv,
		"links_accepted":       snap.LinksAccepted,
		"links_dialed":         snap.LinksDialed,
		"bandwidth_limit_hour": d.sec.BandwidthLimitBytes(),
		"dtn_quota_bytes":      d.courierQuota(),
	}
}

func (d *Daemon) courierQuota() int64 {
	if d.cfg.MaxCourierBytes > 0 {
		return d.cfg.MaxCourierBytes
	}
	return dtn.DefaultQuotaBytes
}
