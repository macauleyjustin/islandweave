package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/islandweave/islandweave/core/objects"
	"github.com/islandweave/islandweave/core/services"
	"github.com/islandweave/islandweave/core/storage"
	"github.com/islandweave/islandweave/core/transport"
	"github.com/islandweave/islandweave/hosting"
	"github.com/islandweave/islandweave/protocols/iwtp"
)

func (d *Daemon) getOrDialLink(ctx context.Context, peerID string) (*transport.Link, error) {
	p, err := d.store.GetPeer(peerID)
	if err == nil && p.Revoked {
		return nil, fmt.Errorf("peer %s is revoked", peerID)
	}
	d.mu.RLock()
	link := d.links[peerID]
	d.mu.RUnlock()
	if link != nil {
		return link, nil
	}
	var dialAddr string
	if p != nil && len(p.Addrs) > 0 {
		dialAddr = p.Addrs[0]
	} else if d.beacon != nil {
		for _, dp := range d.beacon.Discovered() {
			if dp.Announcement.NodeID == peerID {
				dialAddr = dp.DialAddr
				break
			}
		}
	}
	if dialAddr == "" {
		return nil, fmt.Errorf("peer %s is not connected and has no known address", peerID)
	}
	return d.ConnectPeer(ctx, dialAddr)
}

// SendFile ingests a local file into IWP-OBJECT and initiates an IWTP transfer to peerID.
func (d *Daemon) SendFile(ctx context.Context, peerID, filePath string) (*storage.TransferRecord, error) {
	chunks, size, fileHash, err := d.objStore.IngestFile(filePath, objects.DefaultChunkSize)
	if err != nil {
		return nil, fmt.Errorf("ingest file: %w", err)
	}
	link, err := d.getOrDialLink(ctx, peerID)
	if err != nil {
		return nil, err
	}

	tr := &storage.TransferRecord{
		ID:          uuid.NewString(),
		PeerID:      peerID,
		Direction:   "outbound",
		FileName:    filepath.Base(filePath),
		FileSize:    size,
		FileHash:    fileHash,
		Chunks:      chunks,
		ReceivedMap: make([]bool, len(chunks)),
		Status:      "active",
		SavePath:    filePath,
		UpdatedAt:   time.Now().Unix(),
	}
	if err := d.store.SaveTransfer(tr); err != nil {
		return nil, err
	}
	if err := d.sendOffer(link, tr); err != nil {
		return nil, err
	}
	return tr, nil
}

func (d *Daemon) sendOffer(link *transport.Link, tr *storage.TransferRecord) error {
	env, err := iwtp.MakeEnvelope(d.id.ID, iwtp.TypeOffer, iwtp.OfferPayload{
		TransferID: tr.ID,
		FileName:   tr.FileName,
		FileSize:   tr.FileSize,
		FileHash:   tr.FileHash,
		Chunks:     tr.Chunks,
		ChunkSize:  objects.DefaultChunkSize,
	})
	if err != nil {
		return err
	}
	return link.SendEnvelope(env)
}

func (d *Daemon) resumePendingTransfers(link *transport.Link) {
	list, err := d.store.ListTransfers(100)
	if err != nil {
		return
	}
	for _, tr := range list {
		if tr.PeerID == link.PeerID() && tr.Direction == "outbound" && (tr.Status == "active" || tr.Status == "pending") {
			_ = d.sendOffer(link, tr)
		}
	}
}

// CancelTransfer marks a transfer as canceled and notifies the peer if connected.
func (d *Daemon) CancelTransfer(transferID string) error {
	tr, err := d.store.GetTransfer(transferID)
	if err != nil {
		return err
	}
	tr.Status = "canceled"
	tr.UpdatedAt = time.Now().Unix()
	if err := d.store.SaveTransfer(tr); err != nil {
		return err
	}
	d.mu.RLock()
	link := d.links[tr.PeerID]
	d.mu.RUnlock()
	if link != nil {
		env, _ := iwtp.MakeEnvelope(d.id.ID, iwtp.TypeCancel, iwtp.CancelPayload{
			TransferID: transferID,
			Reason:     "canceled by user",
		})
		_ = link.SendEnvelope(env)
	}
	return nil
}

// HostSite publishes a local static website directory as a signed IWP-SERVICE.
func (d *Daemon) HostSite(dir, name, islandID string) (*services.ServiceRecord, *services.SiteManifest, error) {
	siteVersion := 1
	if existing, err := d.store.GetServiceByNameOrID(name); err == nil && existing.IsLocal {
		siteVersion = existing.SiteVersion + 1
	}
	rec, manifest, err := hosting.PublishStaticSite(d.objStore, d.id, dir, name, islandID, siteVersion, 0)
	if err != nil {
		return nil, nil, err
	}
	if err := d.store.SaveService(rec, true); err != nil {
		return nil, nil, err
	}
	return rec, manifest, nil
}

// FetchPeerServices queries a connected peer for its hosted/known services.
func (d *Daemon) FetchPeerServices(ctx context.Context, link *transport.Link) ([]services.ServiceRecord, error) {
	ch := make(chan []services.ServiceRecord, 1)
	d.waitMu.Lock()
	d.svcWaiters[link.PeerID()] = ch
	d.waitMu.Unlock()
	defer func() {
		d.waitMu.Lock()
		delete(d.svcWaiters, link.PeerID())
		d.waitMu.Unlock()
	}()

	env, err := iwtp.MakeEnvelope(d.id.ID, iwtp.TypeSvcList, map[string]any{})
	if err != nil {
		return nil, err
	}
	if err := link.SendEnvelope(env); err != nil {
		return nil, err
	}
	select {
	case list := <-ch:
		return list, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(5 * time.Second):
		return nil, errors.New("timed out waiting for service list")
	}
}

// FetchObjectFromPeer requests a single verified IWP-OBJECT chunk from a connected peer.
func (d *Daemon) FetchObjectFromPeer(ctx context.Context, link *transport.Link, hash string) ([]byte, error) {
	if data, err := d.objStore.GetChunk(hash); err == nil {
		return data, nil
	}
	ch := make(chan iwtp.ObjDataPayload, 1)
	d.waitMu.Lock()
	d.objWaiters[hash] = ch
	d.waitMu.Unlock()
	defer func() {
		d.waitMu.Lock()
		delete(d.objWaiters, hash)
		d.waitMu.Unlock()
	}()

	env, err := iwtp.MakeEnvelope(d.id.ID, iwtp.TypeObjGet, iwtp.ObjGetPayload{Hash: hash})
	if err != nil {
		return nil, err
	}
	if err := link.SendEnvelope(env); err != nil {
		return nil, err
	}
	select {
	case resp := <-ch:
		if resp.Error != "" {
			return nil, errors.New(resp.Error)
		}
		raw, err := iwtp.DecodeData(resp.DataB64)
		if err != nil {
			return nil, err
		}
		if err := d.objStore.PutVerifiedChunk(hash, raw); err != nil {
			return nil, err
		}
		return raw, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("timed out fetching object %s", hash)
	}
}

// FetchSite discovers and caches a remote site's ServiceRecord, SiteManifest, and all file chunks.
func (d *Daemon) FetchSite(ctx context.Context, nameOrID string) error {
	d.mu.RLock()
	links := make([]*transport.Link, 0, len(d.links))
	for _, l := range d.links {
		links = append(links, l)
	}
	d.mu.RUnlock()

	for _, link := range links {
		svcs, err := d.FetchPeerServices(ctx, link)
		if err != nil {
			continue
		}
		for i := range svcs {
			rec := &svcs[i]
			if rec.Name != nameOrID && rec.ServiceID != nameOrID {
				continue
			}
			if err := services.VerifyServiceRecord(rec, time.Now()); err != nil {
				continue
			}
			if _, err := d.FetchObjectFromPeer(ctx, link, rec.ManifestHash); err != nil {
				continue
			}
			manifest, err := hosting.LoadSiteManifest(d.objStore, rec.ManifestHash)
			if err != nil {
				continue
			}
			allOK := true
			for _, f := range manifest.Files {
				for _, ch := range f.Chunks {
					if _, err := d.FetchObjectFromPeer(ctx, link, ch); err != nil {
						allOK = false
						break
					}
				}
				if !allOK {
					break
				}
			}
			if allOK {
				_ = d.store.SaveService(rec, false)
				return nil
			}
		}
	}
	return fmt.Errorf("service %q not found on connected peers", nameOrID)
}

func (d *Daemon) handlePhase2Envelope(link *transport.Link, env *transport.Envelope) {
	switch env.Type {
	case iwtp.TypeOffer:
		var offer iwtp.OfferPayload
		if err := json.Unmarshal(env.Payload, &offer); err != nil {
			return
		}
		tr, err := d.store.GetTransfer(offer.TransferID)
		if err != nil {
			downloadsDir := filepath.Join(d.cfg.DataDir, "downloads")
			savePath, ok := iwtp.ResolveDownloadPath(downloadsDir, offer.FileName, iwtp.ConflictRename)
			if !ok {
				return
			}
			tr = &storage.TransferRecord{
				ID:          offer.TransferID,
				PeerID:      link.PeerID(),
				Direction:   "inbound",
				FileName:    offer.FileName,
				FileSize:    offer.FileSize,
				FileHash:    offer.FileHash,
				Chunks:      offer.Chunks,
				ReceivedMap: make([]bool, len(offer.Chunks)),
				Status:      "active",
				SavePath:    savePath,
			}
		}
		if tr.Status == "canceled" {
			return
		}
		d.advanceInboundTransfer(link, tr)

	case iwtp.TypeChunkReq:
		var req iwtp.ChunkReqPayload
		if err := json.Unmarshal(env.Payload, &req); err != nil {
			return
		}
		tr, err := d.store.GetTransfer(req.TransferID)
		if err == nil && tr.Status == "canceled" {
			return
		}
		data, err := d.objStore.GetChunk(req.ChunkHash)
		if err != nil {
			return
		}
		respEnv, err := iwtp.MakeEnvelope(d.id.ID, iwtp.TypeChunkData, iwtp.ChunkDataPayload{
			TransferID: req.TransferID,
			ChunkIndex: req.ChunkIndex,
			ChunkHash:  req.ChunkHash,
			DataB64:    iwtp.EncodeData(data),
		})
		if err == nil {
			_ = link.SendEnvelope(respEnv)
		}

	case iwtp.TypeChunkData:
		var cd iwtp.ChunkDataPayload
		if err := json.Unmarshal(env.Payload, &cd); err != nil {
			return
		}
		tr, err := d.store.GetTransfer(cd.TransferID)
		if err != nil || tr.Status == "canceled" {
			return
		}
		raw, err := iwtp.DecodeData(cd.DataB64)
		if err != nil {
			return
		}
		if err := d.objStore.PutVerifiedChunk(cd.ChunkHash, raw); err != nil {
			tr.Status = "failed"
			tr.ErrorMsg = err.Error()
			_ = d.store.SaveTransfer(tr)
			return
		}
		if cd.ChunkIndex >= 0 && cd.ChunkIndex < len(tr.ReceivedMap) {
			tr.ReceivedMap[cd.ChunkIndex] = true
		}
		d.advanceInboundTransfer(link, tr)

	case iwtp.TypeComplete:
		var comp iwtp.CompletePayload
		if err := json.Unmarshal(env.Payload, &comp); err == nil {
			if tr, err := d.store.GetTransfer(comp.TransferID); err == nil {
				tr.Status = "completed"
				tr.UpdatedAt = time.Now().Unix()
				_ = d.store.SaveTransfer(tr)
			}
		}

	case iwtp.TypeCancel:
		var canc iwtp.CancelPayload
		if err := json.Unmarshal(env.Payload, &canc); err == nil {
			if tr, err := d.store.GetTransfer(canc.TransferID); err == nil {
				tr.Status = "canceled"
				tr.ErrorMsg = canc.Reason
				tr.UpdatedAt = time.Now().Unix()
				_ = d.store.SaveTransfer(tr)
			}
		}

	case iwtp.TypeObjGet:
		var req iwtp.ObjGetPayload
		if err := json.Unmarshal(env.Payload, &req); err != nil {
			return
		}
		data, err := d.objStore.GetChunk(req.Hash)
		payload := iwtp.ObjDataPayload{Hash: req.Hash}
		if err != nil {
			payload.Error = err.Error()
		} else {
			payload.DataB64 = iwtp.EncodeData(data)
		}
		respEnv, err := iwtp.MakeEnvelope(d.id.ID, iwtp.TypeObjData, payload)
		if err == nil {
			_ = link.SendEnvelope(respEnv)
		}

	case iwtp.TypeObjData:
		var resp iwtp.ObjDataPayload
		if err := json.Unmarshal(env.Payload, &resp); err != nil {
			return
		}
		d.waitMu.Lock()
		ch := d.objWaiters[resp.Hash]
		d.waitMu.Unlock()
		if ch != nil {
			select {
			case ch <- resp:
			default:
			}
		}

	case iwtp.TypeSvcList:
		entries, err := d.store.ListServices()
		if err != nil {
			return
		}
		recs := make([]services.ServiceRecord, 0, len(entries))
		for _, e := range entries {
			recs = append(recs, e.ServiceRecord)
		}
		respEnv, err := iwtp.MakeEnvelope(d.id.ID, iwtp.TypeSvcListResp, iwtp.SvcListRespPayload{Services: recs})
		if err == nil {
			_ = link.SendEnvelope(respEnv)
		}

	case iwtp.TypeSvcListResp:
		var resp iwtp.SvcListRespPayload
		if err := json.Unmarshal(env.Payload, &resp); err != nil {
			return
		}
		d.waitMu.Lock()
		ch := d.svcWaiters[link.PeerID()]
		d.waitMu.Unlock()
		if ch != nil {
			select {
			case ch <- resp.Services:
			default:
			}
		}
	}
}

func (d *Daemon) advanceInboundTransfer(link *transport.Link, tr *storage.TransferRecord) {
	nextIdx := iwtp.NextMissingChunk(tr, d.objStore)
	if nextIdx < 0 {
		if err := d.objStore.AssembleFile(tr.Chunks, tr.FileHash, tr.SavePath); err != nil {
			tr.Status = "failed"
			tr.ErrorMsg = err.Error()
			_ = d.store.SaveTransfer(tr)
			return
		}
		tr.Status = "completed"
		tr.UpdatedAt = time.Now().Unix()
		_ = d.store.SaveTransfer(tr)
		compEnv, _ := iwtp.MakeEnvelope(d.id.ID, iwtp.TypeComplete, iwtp.CompletePayload{TransferID: tr.ID})
		_ = link.SendEnvelope(compEnv)
		return
	}
	tr.Status = "active"
	tr.UpdatedAt = time.Now().Unix()
	_ = d.store.SaveTransfer(tr)
	reqEnv, _ := iwtp.MakeEnvelope(d.id.ID, iwtp.TypeChunkReq, iwtp.ChunkReqPayload{
		TransferID: tr.ID,
		ChunkIndex: nextIdx,
		ChunkHash:  tr.Chunks[nextIdx],
	})
	_ = link.SendEnvelope(reqEnv)
}
