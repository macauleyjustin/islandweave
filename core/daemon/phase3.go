package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/islandweave/islandweave/core/storage"
	"github.com/islandweave/islandweave/core/transport"
	"github.com/islandweave/islandweave/protocols/dtn"
	"github.com/islandweave/islandweave/protocols/iwbn"
	"github.com/islandweave/islandweave/protocols/iwmp"
	"github.com/islandweave/islandweave/protocols/iwtp"
)

type dtnMsgInner struct {
	Body string `json:"body"`
}

// QueueDTNMessage encrypts a message end-to-end for destID and places it in the persistent DTN outbound queue.
func (d *Daemon) QueueDTNMessage(destID, islandID, body string, priority, maxHops int, ttl time.Duration) (*storage.DTNBundleRecord, error) {
	peer, err := d.store.GetPeer(destID)
	if err != nil || len(peer.EncPublicKey) != 32 {
		return nil, fmt.Errorf("recipient %s has no known X25519 encryption key (connect once or exchange identity first)", destID)
	}
	if peer.Revoked {
		return nil, fmt.Errorf("recipient %s is revoked", destID)
	}
	inner, _ := json.Marshal(dtnMsgInner{Body: body})
	b, err := dtn.CreateBundle(d.id, destID, peer.EncPublicKey, islandID, "msg", priority, maxHops, ttl, inner)
	if err != nil {
		return nil, err
	}
	rec := bundleToRecord(b, "queued", d.id.ID)
	if err := d.store.SaveDTNBundle(rec); err != nil {
		return nil, err
	}
	_, _ = d.store.MarkDTNSeen(b.BundleID, time.Now().Unix())

	// Also record in messages table as pending DTN delivery
	_ = d.store.SaveMessage(&storage.MessageRecord{
		ID:          b.BundleID,
		IslandID:    islandID,
		SenderID:    d.id.ID,
		RecipientID: destID,
		Body:        body,
		Direction:   "outbound",
		Status:      "queued-dtn",
		Timestamp:   b.CreatedAt,
	})

	// If any peers are currently connected, immediately forward
	d.mu.RLock()
	for _, l := range d.links {
		go d.syncDTNBundles(l)
	}
	d.mu.RUnlock()

	return rec, nil
}

func (d *Daemon) syncDTNBundles(link *transport.Link) {
	list, err := d.store.ListForwardableDTNBundles(time.Now().Unix())
	if err != nil {
		return
	}
	for _, r := range list {
		// Only send to final destination or to peers when forwarding is enabled
		if r.DestinationID != link.PeerID() && !d.courierForwardingEnabled() {
			continue
		}
		b := recordToBundle(r)
		env, err := iwtp.MakeEnvelope(d.id.ID, dtn.TypeBundle, b)
		if err != nil {
			continue
		}
		_ = link.SendEnvelope(env)
	}
}

func (d *Daemon) handleDTNEnvelope(link *transport.Link, env *transport.Envelope) bool {
	switch env.Type {
	case dtn.TypeBundle:
		var b dtn.Bundle
		if err := json.Unmarshal(env.Payload, &b); err != nil {
			return true
		}
		_, _ = d.processIncomingBundle(&b, link)
		return true

	case dtn.TypeCustodyAck:
		var ack dtn.CustodyAckPayload
		if err := json.Unmarshal(env.Payload, &ack); err == nil && ack.BundleID != "" {
			_ = d.store.UpdateDTNState(ack.BundleID, "carrying", ack.CustodyHolder)
		}
		return true
	}
	return false
}

// processIncomingBundle verifies, deduplicates, and either delivers (if local node is destination)
// or stores for courier forwarding (if quota & hop limits allow).
func (d *Daemon) processIncomingBundle(b *dtn.Bundle, link *transport.Link) (outcome string, err error) {
	now := time.Now()
	if err := dtn.VerifyBundle(b, now); err != nil {
		return "rejected", err
	}
	newlySeen, err := d.store.MarkDTNSeen(b.BundleID, now.Unix())
	if err != nil {
		return "error", err
	}
	if !newlySeen {
		return "duplicate", nil
	}

	// Destination is this node: decrypt and deliver
	if b.DestinationID == d.id.ID {
		plain, err := dtn.DecryptBundle(b, d.id)
		if err != nil {
			return "decrypt-failed", err
		}
		if b.Kind == "ack" {
			var ack dtn.DeliveryAckPayload
			if err := json.Unmarshal(plain, &ack); err == nil && ack.OriginalBundleID != "" {
				_ = d.store.UpdateDTNState(ack.OriginalBundleID, "acked", d.id.ID)
				_ = d.store.UpdateMessageStatus(ack.OriginalBundleID, "acked")
				_ = d.store.UpdateMailDelivery(ack.OriginalBundleID, "acked")
			}
			_ = d.store.SaveDTNBundle(bundleToRecord(b, "delivered", d.id.ID))
			return "acked", nil
		}

		if b.Kind == "mail" {
			var mp iwmp.MailPayload
			if err := json.Unmarshal(plain, &mp); err == nil {
				mailID := mp.MailID
				if mailID == "" {
					mailID = b.BundleID
				}
				_ = d.store.SaveMail(&iwmp.MailRecord{
					ID:          mailID,
					ThreadID:    mp.ThreadID,
					IslandID:    b.IslandID,
					FromID:      b.SourceID,
					ToID:        d.id.ID,
					Subject:     mp.Subject,
					Body:        mp.Body,
					Attachments: mp.Attachments,
					Folder:      "inbox",
					Status:      "delivered",
					Timestamp:   b.CreatedAt,
				})
			}
		} else {
			var inner dtnMsgInner
			bodyText := string(plain)
			if err := json.Unmarshal(plain, &inner); err == nil && inner.Body != "" {
				bodyText = inner.Body
			}
			_ = d.store.SaveMessage(&storage.MessageRecord{
				ID:          b.BundleID,
				IslandID:    b.IslandID,
				SenderID:    b.SourceID,
				RecipientID: d.id.ID,
				Body:        bodyText,
				Direction:   "inbound",
				Status:      "received",
				Timestamp:   b.CreatedAt,
			})
		}
		_ = d.store.SaveDTNBundle(bundleToRecord(b, "delivered", d.id.ID))

		// Queue end-to-end delivery ack back to SourceID if we know their X25519 key
		if srcPeer, err := d.store.GetPeer(b.SourceID); err == nil && len(srcPeer.EncPublicKey) == 32 {
			ackInner, _ := json.Marshal(dtn.DeliveryAckPayload{
				OriginalBundleID: b.BundleID,
				RecipientID:      d.id.ID,
				DeliveredAt:      now.Unix(),
			})
			if ackBundle, err := dtn.CreateBundle(d.id, b.SourceID, srcPeer.EncPublicKey, b.IslandID, "ack", 1, dtn.DefaultMaxHops, dtn.DefaultBundleTTL, ackInner); err == nil {
				_ = d.store.SaveDTNBundle(bundleToRecord(ackBundle, "queued", d.id.ID))
				_, _ = d.store.MarkDTNSeen(ackBundle.BundleID, now.Unix())
			}
		}

		if link != nil {
			cAck, _ := iwtp.MakeEnvelope(d.id.ID, dtn.TypeCustodyAck, dtn.CustodyAckPayload{
				BundleID:      b.BundleID,
				CustodyHolder: d.id.ID,
				Timestamp:     now.Unix(),
			})
			_ = link.SendEnvelope(cAck)
		}
		return "delivered", nil
	}

	// Intermediate courier node: store without decrypting!
	if !d.courierForwardingEnabled() {
		return "forwarding-disabled", errors.New("courier forwarding is disabled")
	}
	if b.HopCount+1 >= b.MaxHops {
		return "hop-limit", errors.New("bundle reached max hops")
	}
	used, err := d.store.CourierStorageUsed()
	if err != nil {
		return "error", err
	}
	quota := d.maxCourierBytes()
	if used+int64(len(b.Ciphertext)) > quota {
		return "quota-exceeded", fmt.Errorf("courier storage quota exceeded (%d / %d bytes)", used, quota)
	}

	b.HopCount++
	if err := d.store.SaveDTNBundle(bundleToRecord(b, "carrying", d.id.ID)); err != nil {
		return "error", err
	}
	// If the destination is currently connected to this relay/courier, forward immediately!
	d.mu.RLock()
	destLink := d.links[b.DestinationID]
	d.mu.RUnlock()
	if destLink != nil {
		if fwdEnv, err := iwtp.MakeEnvelope(d.id.ID, dtn.TypeBundle, b); err == nil {
			_ = destLink.SendEnvelope(fwdEnv)
		}
	}
	if link != nil {
		cAck, _ := iwtp.MakeEnvelope(d.id.ID, dtn.TypeCustodyAck, dtn.CustodyAckPayload{
			BundleID:      b.BundleID,
			CustodyHolder: d.id.ID,
			Timestamp:     now.Unix(),
		})
		_ = link.SendEnvelope(cAck)
	}
	return "carrying", nil
}

// ExportCourierPack exports all forwardable DTN bundles and active bulletins to a portable file for removable storage transport.
func (d *Daemon) ExportCourierPack(outPath string) (int, error) {
	now := time.Now().Unix()
	list, err := d.store.ListForwardableDTNBundles(now)
	if err != nil {
		return 0, err
	}
	bulletins, _ := d.store.ListBulletins("", 200)
	pack := dtn.CourierPack{
		Version:    1,
		ExportedBy: d.id.ID,
		ExportedAt: now,
		Bundles:    make([]dtn.Bundle, 0, len(list)),
	}
	for _, r := range list {
		pack.Bundles = append(pack.Bundles, *recordToBundle(r))
	}
	for _, b := range bulletins {
		if b.ExpiresAt == 0 || b.ExpiresAt > now {
			pack.Bulletins = append(pack.Bulletins, *b)
		}
	}
	raw, err := json.MarshalIndent(pack, "", "  ")
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(outPath, raw, 0o600); err != nil {
		return 0, err
	}
	return len(pack.Bundles), nil
}

// ImportCourierPack imports DTN bundles and signed bulletins from a physical courier file.
func (d *Daemon) ImportCourierPack(inPath string) (delivered, carrying, skipped int, err error) {
	raw, err := os.ReadFile(inPath)
	if err != nil {
		return 0, 0, 0, err
	}
	var pack dtn.CourierPack
	if err := json.Unmarshal(raw, &pack); err != nil {
		return 0, 0, 0, fmt.Errorf("invalid courier pack: %w", err)
	}
	now := time.Now()
	for i := range pack.Bulletins {
		b := &pack.Bulletins[i]
		if iwbn.VerifyBulletin(b, now) == nil {
			_ = d.store.SaveBulletin(b)
		}
	}
	for i := range pack.Bundles {
		outcome, procErr := d.processIncomingBundle(&pack.Bundles[i], nil)
		if procErr != nil || outcome == "duplicate" {
			skipped++
			continue
		}
		switch outcome {
		case "delivered", "acked":
			delivered++
		case "carrying":
			carrying++
		default:
			skipped++
		}
	}
	return delivered, carrying, skipped, nil
}

func (d *Daemon) courierForwardingEnabled() bool {
	return !d.cfg.DisableCourierForwarding
}

func (d *Daemon) maxCourierBytes() int64 {
	if d.cfg.MaxCourierBytes > 0 {
		return d.cfg.MaxCourierBytes
	}
	return dtn.DefaultQuotaBytes
}

func bundleToRecord(b *dtn.Bundle, state, custodyHolder string) *storage.DTNBundleRecord {
	return &storage.DTNBundleRecord{
		BundleID:      b.BundleID,
		SourceID:      b.SourceID,
		SourcePub:     b.SourcePub,
		DestinationID: b.DestinationID,
		IslandID:      b.IslandID,
		Kind:          b.Kind,
		Priority:      b.Priority,
		HopCount:      b.HopCount,
		MaxHops:       b.MaxHops,
		CreatedAt:     b.CreatedAt,
		ExpiresAt:     b.ExpiresAt,
		EphemeralPub:  b.EphemeralPub,
		Ciphertext:    b.Ciphertext,
		Signature:     b.Signature,
		State:         state,
		CustodyHolder: custodyHolder,
		SizeBytes:     len(b.Ciphertext),
	}
}

func recordToBundle(r *storage.DTNBundleRecord) *dtn.Bundle {
	return &dtn.Bundle{
		Version:       1,
		BundleID:      r.BundleID,
		SourceID:      r.SourceID,
		SourcePub:     r.SourcePub,
		DestinationID: r.DestinationID,
		IslandID:      r.IslandID,
		Kind:          r.Kind,
		Priority:      r.Priority,
		HopCount:      r.HopCount,
		MaxHops:       r.MaxHops,
		CreatedAt:     r.CreatedAt,
		ExpiresAt:     r.ExpiresAt,
		EphemeralPub:  r.EphemeralPub,
		Ciphertext:    r.Ciphertext,
		Signature:     r.Signature,
	}
}

// ExportForwardableBundles returns all non-expired forwardable DTN bundles ordered by priority.
func (d *Daemon) ExportForwardableBundles() ([]dtn.Bundle, error) {
	list, err := d.store.ListForwardableDTNBundles(time.Now().Unix())
	if err != nil {
		return nil, err
	}
	out := make([]dtn.Bundle, 0, len(list))
	for _, r := range list {
		out = append(out, *recordToBundle(r))
	}
	return out, nil
}

// IngestExternalBundle verifies, deduplicates, and delivers/stores a bundle arriving from an external transport (e.g. IWST satellite).
func (d *Daemon) IngestExternalBundle(b *dtn.Bundle) (string, error) {
	return d.processIncomingBundle(b, nil)
}
