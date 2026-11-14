package daemon

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/islandweave/islandweave/core/transport"
	"github.com/islandweave/islandweave/protocols/dtn"
	"github.com/islandweave/islandweave/protocols/iwbn"
	"github.com/islandweave/islandweave/protocols/iwmp"
	"github.com/islandweave/islandweave/protocols/iwtp"
)

// SendMail composes, encrypts, and queues an IWMP mail message (or saves a draft).
func (d *Daemon) SendMail(toID, threadID, islandID, subject, body string, attachments []iwmp.Attachment, draft bool) (*iwmp.MailRecord, error) {
	now := time.Now().Unix()
	if draft {
		rec := &iwmp.MailRecord{
			ID:          uuid.NewString(),
			ThreadID:    threadID,
			IslandID:    islandID,
			FromID:      d.id.ID,
			ToID:        toID,
			Subject:     subject,
			Body:        body,
			Attachments: attachments,
			Folder:      "drafts",
			Status:      "draft",
			Timestamp:   now,
		}
		if err := d.store.SaveMail(rec); err != nil {
			return nil, err
		}
		return rec, nil
	}

	peer, err := d.store.GetPeer(toID)
	if err != nil || len(peer.EncPublicKey) != 32 {
		return nil, fmt.Errorf("recipient %s has no known X25519 encryption key", toID)
	}
	if peer.Revoked {
		return nil, fmt.Errorf("recipient %s is revoked", toID)
	}

	mp := iwmp.MailPayload{
		ThreadID:    threadID,
		Subject:     subject,
		Body:        body,
		Attachments: attachments,
		SentAt:      now,
	}
	plain, _ := json.Marshal(mp)
	b, err := dtn.CreateBundle(d.id, toID, peer.EncPublicKey, islandID, "mail", 1, dtn.DefaultMaxHops, dtn.DefaultBundleTTL, plain)
	if err != nil {
		return nil, err
	}
	rec := &iwmp.MailRecord{
		ID:          b.BundleID,
		ThreadID:    threadID,
		IslandID:    islandID,
		FromID:      d.id.ID,
		ToID:        toID,
		Subject:     subject,
		Body:        body,
		Attachments: attachments,
		Folder:      "outbox",
		Status:      "queued",
		Timestamp:   now,
	}
	if err := d.store.SaveDTNBundle(bundleToRecord(b, "queued", d.id.ID)); err != nil {
		return nil, err
	}
	_, _ = d.store.MarkDTNSeen(b.BundleID, now)
	if err := d.store.SaveMail(rec); err != nil {
		return nil, err
	}

	d.mu.RLock()
	for _, l := range d.links {
		go d.syncDTNBundles(l)
	}
	d.mu.RUnlock()

	return rec, nil
}

// PublishBulletin creates, signs, stores, and replicates an IWBN bulletin.
func (d *Daemon) PublishBulletin(channel, category, title, summary, body, islandID string, officialEmergency bool, ttl time.Duration) (*iwbn.Bulletin, error) {
	b, err := iwbn.CreateBulletin(d.id, channel, category, title, summary, body, islandID, officialEmergency, 1, ttl)
	if err != nil {
		return nil, err
	}
	if err := d.store.SaveBulletin(b); err != nil {
		return nil, err
	}

	d.mu.RLock()
	for _, l := range d.links {
		env, err := iwtp.MakeEnvelope(d.id.ID, iwbn.TypeBulletin, b)
		if err == nil {
			_ = l.SendEnvelope(env)
		}
	}
	d.mu.RUnlock()

	return b, nil
}

func (d *Daemon) syncBulletins(link *transport.Link) {
	list, err := d.store.ListBulletins("", 100)
	if err != nil {
		return
	}
	now := time.Now().Unix()
	for _, b := range list {
		if b.ExpiresAt > 0 && b.ExpiresAt <= now {
			continue
		}
		env, err := iwtp.MakeEnvelope(d.id.ID, iwbn.TypeBulletin, b)
		if err == nil {
			_ = link.SendEnvelope(env)
		}
	}
}

func (d *Daemon) handlePhase4Envelope(_ *transport.Link, env *transport.Envelope) bool {
	if env.Type != iwbn.TypeBulletin {
		return false
	}
	var b iwbn.Bulletin
	if err := json.Unmarshal(env.Payload, &b); err != nil {
		return true
	}
	if err := iwbn.VerifyBulletin(&b, time.Now()); err != nil {
		return true
	}
	_ = d.store.SaveBulletin(&b)
	return true
}
