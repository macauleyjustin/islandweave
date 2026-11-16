package storage

import (
	"crypto/ed25519"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/islandweave/islandweave/core/islands"
	"github.com/islandweave/islandweave/core/services"
	"github.com/islandweave/islandweave/federation"
	"github.com/islandweave/islandweave/protocols/iwbn"
	"github.com/islandweave/islandweave/protocols/iwmp"
	"github.com/islandweave/islandweave/protocols/iwst"
	_ "modernc.org/sqlite"
)

// PeerRecord stores known peer metadata and authorization state.
type PeerRecord struct {
	NodeID       string   `json:"node_id"`
	Name         string   `json:"name"`
	PublicKey    []byte   `json:"public_key"`
	EncPublicKey []byte   `json:"enc_public_key,omitempty"`
	Addrs        []string `json:"addrs"`
	Islands      []string `json:"islands"`
	Trusted      bool     `json:"trusted"`
	Revoked      bool     `json:"revoked"`
	LastSeen     int64    `json:"last_seen"`
	Source       string   `json:"source"` // "beacon", "manual", "invite", "inbound"
}

// MessageRecord stores direct and Island chat messages.
type MessageRecord struct {
	ID          string `json:"id"`
	IslandID    string `json:"island_id"`
	SenderID    string `json:"sender_id"`
	RecipientID string `json:"recipient_id"`
	Body        string `json:"body"`
	Direction   string `json:"direction"` // "inbound" or "outbound"
	Status      string `json:"status"`    // "pending", "sent", "acked", "received"
	Timestamp   int64  `json:"timestamp"`
}

// Store wraps the SQLite database for islandd.
type Store struct {
	db *sql.DB
}

// Open opens or initializes the SQLite database in <dataDir>/islandweave.db.
func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	dbPath := filepath.Join(dataDir, "islandweave.db")
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS islands (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			public_key BLOB NOT NULL,
			private_key BLOB,
			role TEXT NOT NULL,
			visibility TEXT NOT NULL,
			created_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS peers (
			node_id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			public_key BLOB NOT NULL,
			enc_public_key BLOB,
			addrs TEXT NOT NULL,
			islands TEXT NOT NULL,
			trusted INTEGER NOT NULL DEFAULT 0,
			revoked INTEGER NOT NULL DEFAULT 0,
			last_seen INTEGER NOT NULL,
			source TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			island_id TEXT NOT NULL,
			sender_id TEXT NOT NULL,
			recipient_id TEXT NOT NULL,
			body TEXT NOT NULL,
			direction TEXT NOT NULL,
			status TEXT NOT NULL,
			timestamp INTEGER NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_messages_ts ON messages(timestamp DESC);`,
		`CREATE TABLE IF NOT EXISTS services (
			service_id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			island_id TEXT NOT NULL,
			publisher_id TEXT NOT NULL,
			publisher_pub TEXT NOT NULL,
			site_version INTEGER NOT NULL,
			manifest_hash TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL,
			signature TEXT NOT NULL,
			is_local INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS transfers (
			id TEXT PRIMARY KEY,
			peer_id TEXT NOT NULL,
			direction TEXT NOT NULL,
			file_name TEXT NOT NULL,
			file_size INTEGER NOT NULL,
			file_hash TEXT NOT NULL,
			chunks_json TEXT NOT NULL,
			received_json TEXT NOT NULL,
			status TEXT NOT NULL,
			save_path TEXT NOT NULL,
			error_msg TEXT NOT NULL,
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS dtn_bundles (
			bundle_id TEXT PRIMARY KEY,
			source_id TEXT NOT NULL,
			source_pub TEXT NOT NULL,
			destination_id TEXT NOT NULL,
			island_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			priority INTEGER NOT NULL DEFAULT 0,
			hop_count INTEGER NOT NULL DEFAULT 0,
			max_hops INTEGER NOT NULL DEFAULT 8,
			created_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL,
			ephemeral_pub TEXT NOT NULL,
			ciphertext TEXT NOT NULL,
			signature TEXT NOT NULL,
			state TEXT NOT NULL,
			custody_holder TEXT NOT NULL,
			size_bytes INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE INDEX IF NOT EXISTS idx_dtn_state_prio ON dtn_bundles(state, priority DESC, created_at ASC);`,
		`CREATE TABLE IF NOT EXISTS dtn_seen (
			bundle_id TEXT PRIMARY KEY,
			seen_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS mail (
			id TEXT PRIMARY KEY,
			thread_id TEXT NOT NULL,
			island_id TEXT NOT NULL,
			from_id TEXT NOT NULL,
			to_id TEXT NOT NULL,
			subject TEXT NOT NULL,
			body TEXT NOT NULL,
			attachments_json TEXT NOT NULL,
			folder TEXT NOT NULL,
			status TEXT NOT NULL,
			timestamp INTEGER NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_mail_folder_ts ON mail(folder, timestamp DESC);`,
		`CREATE TABLE IF NOT EXISTS bulletins (
			bulletin_id TEXT PRIMARY KEY,
			channel TEXT NOT NULL,
			category TEXT NOT NULL,
			official_emergency INTEGER NOT NULL DEFAULT 0,
			title TEXT NOT NULL,
			summary TEXT NOT NULL,
			body TEXT NOT NULL,
			island_id TEXT NOT NULL,
			publisher_id TEXT NOT NULL,
			publisher_pub TEXT NOT NULL,
			bulletin_version INTEGER NOT NULL,
			created_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL,
			signature TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS bulletin_subscriptions (
			channel TEXT PRIMARY KEY,
			subscribed_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS federations (
			agreement_id TEXT PRIMARY KEY,
			local_island_id TEXT NOT NULL,
			local_island_pub TEXT NOT NULL,
			remote_island_id TEXT NOT NULL,
			shared_services_json TEXT NOT NULL,
			allowed_identities_json TEXT NOT NULL,
			allow_bulletin_sync INTEGER NOT NULL DEFAULT 0,
			allow_dtn_forward INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL,
			revoked INTEGER NOT NULL DEFAULT 0,
			signature TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS navigator_bookmarks (
			uri TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			created_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS navigator_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			uri TEXT NOT NULL,
			title TEXT NOT NULL,
			visited_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS activity_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			category TEXT NOT NULL,
			summary TEXT NOT NULL,
			peer_id TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS satellite_logs (
			pass_id TEXT PRIMARY KEY,
			satellite_name TEXT NOT NULL,
			bytes_transferred INTEGER NOT NULL,
			bundles_delivered INTEGER NOT NULL,
			interrupted INTEGER NOT NULL DEFAULT 0,
			completed_at INTEGER NOT NULL
		);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate sqlite: %w", err)
		}
	}
	return nil
}

// SaveIsland inserts or updates an Island membership.
func (s *Store) SaveIsland(isl *islands.Island) error {
	_, err := s.db.Exec(
		`INSERT INTO islands (id, name, public_key, private_key, role, visibility, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   name = excluded.name,
		   visibility = excluded.visibility`,
		isl.ID, isl.Name, []byte(isl.PublicKey), []byte(isl.PrivateKey), isl.Role, string(isl.Visibility), isl.CreatedAt,
	)
	return err
}

// GetIsland fetches an Island by ID.
func (s *Store) GetIsland(id string) (*islands.Island, error) {
	row := s.db.QueryRow(`SELECT id, name, public_key, private_key, role, visibility, created_at FROM islands WHERE id = ?`, id)
	var isl islands.Island
	var pub, priv []byte
	var vis string
	if err := row.Scan(&isl.ID, &isl.Name, &pub, &priv, &isl.Role, &vis, &isl.CreatedAt); err != nil {
		return nil, err
	}
	isl.PublicKey = ed25519.PublicKey(pub)
	if len(priv) > 0 {
		isl.PrivateKey = ed25519.PrivateKey(priv)
	}
	isl.Visibility = islands.Visibility(vis)
	return &isl, nil
}

// ListIslands returns all joined/created Islands.
func (s *Store) ListIslands() ([]*islands.Island, error) {
	rows, err := s.db.Query(`SELECT id, name, public_key, private_key, role, visibility, created_at FROM islands ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*islands.Island
	for rows.Next() {
		var isl islands.Island
		var pub, priv []byte
		var vis string
		if err := rows.Scan(&isl.ID, &isl.Name, &pub, &priv, &isl.Role, &vis, &isl.CreatedAt); err != nil {
			return nil, err
		}
		isl.PublicKey = ed25519.PublicKey(pub)
		if len(priv) > 0 {
			isl.PrivateKey = ed25519.PrivateKey(priv)
		}
		isl.Visibility = islands.Visibility(vis)
		out = append(out, &isl)
	}
	return out, rows.Err()
}

// DeleteIsland removes an Island membership.
func (s *Store) DeleteIsland(id string) error {
	res, err := s.db.Exec(`DELETE FROM islands WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpsertPeer inserts or updates a peer record, preserving trusted/revoked flags if already present.
func (s *Store) UpsertPeer(p *PeerRecord) error {
	addrsJSON, _ := json.Marshal(p.Addrs)
	islandsJSON, _ := json.Marshal(p.Islands)
	trustedInt := 0
	if p.Trusted {
		trustedInt = 1
	}
	revokedInt := 0
	if p.Revoked {
		revokedInt = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO peers (node_id, name, public_key, enc_public_key, addrs, islands, trusted, revoked, last_seen, source)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(node_id) DO UPDATE SET
		   name = CASE WHEN excluded.name != '' THEN excluded.name ELSE peers.name END,
		   enc_public_key = CASE WHEN length(excluded.enc_public_key) = 32 THEN excluded.enc_public_key ELSE peers.enc_public_key END,
		   addrs = CASE WHEN excluded.addrs != '[]' AND excluded.addrs != 'null' THEN excluded.addrs ELSE peers.addrs END,
		   islands = CASE WHEN excluded.islands != '[]' AND excluded.islands != 'null' THEN excluded.islands ELSE peers.islands END,
		   last_seen = excluded.last_seen,
		   source = excluded.source`,
		p.NodeID, p.Name, p.PublicKey, p.EncPublicKey, string(addrsJSON), string(islandsJSON), trustedInt, revokedInt, p.LastSeen, p.Source,
	)
	return err
}

// SetPeerTrust updates a peer's trusted and revoked state.
func (s *Store) SetPeerTrust(nodeID string, trusted, revoked bool) error {
	tInt, rInt := 0, 0
	if trusted {
		tInt = 1
	}
	if revoked {
		rInt = 1
	}
	res, err := s.db.Exec(`UPDATE peers SET trusted = ?, revoked = ? WHERE node_id = ?`, tInt, rInt, nodeID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetPeer retrieves a peer by NodeID.
func (s *Store) GetPeer(nodeID string) (*PeerRecord, error) {
	row := s.db.QueryRow(`SELECT node_id, name, public_key, enc_public_key, addrs, islands, trusted, revoked, last_seen, source FROM peers WHERE node_id = ?`, nodeID)
	return scanPeer(row)
}

// ListPeers returns all known peers.
func (s *Store) ListPeers() ([]*PeerRecord, error) {
	rows, err := s.db.Query(`SELECT node_id, name, public_key, enc_public_key, addrs, islands, trusted, revoked, last_seen, source FROM peers ORDER BY last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*PeerRecord
	for rows.Next() {
		p, err := scanPeer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanPeer(s scanner) (*PeerRecord, error) {
	var p PeerRecord
	var addrsRaw, islandsRaw string
	var trustedInt, revokedInt int
	if err := s.Scan(&p.NodeID, &p.Name, &p.PublicKey, &p.EncPublicKey, &addrsRaw, &islandsRaw, &trustedInt, &revokedInt, &p.LastSeen, &p.Source); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(addrsRaw), &p.Addrs)
	_ = json.Unmarshal([]byte(islandsRaw), &p.Islands)
	p.Trusted = trustedInt != 0
	p.Revoked = revokedInt != 0
	return &p, nil
}

// SaveMessage stores a message record.
func (s *Store) SaveMessage(m *MessageRecord) error {
	if m.Timestamp == 0 {
		m.Timestamp = time.Now().Unix()
	}
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO messages (id, island_id, sender_id, recipient_id, body, direction, status, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.IslandID, m.SenderID, m.RecipientID, m.Body, m.Direction, m.Status, m.Timestamp,
	)
	return err
}

// UpdateMessageStatus updates the delivery status of a message.
func (s *Store) UpdateMessageStatus(id, status string) error {
	res, err := s.db.Exec(`UPDATE messages SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 && !errors.Is(err, sql.ErrNoRows) {
		return sql.ErrNoRows
	}
	return nil
}

// ListMessages returns recent messages (up to limit).
func (s *Store) ListMessages(limit int) ([]*MessageRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id, island_id, sender_id, recipient_id, body, direction, status, timestamp FROM messages ORDER BY timestamp DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*MessageRecord
	for rows.Next() {
		var m MessageRecord
		if err := rows.Scan(&m.ID, &m.IslandID, &m.SenderID, &m.RecipientID, &m.Body, &m.Direction, &m.Status, &m.Timestamp); err != nil {
			return nil, err
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

// ServiceEntry pairs a ServiceRecord with whether this node hosts it locally.
type ServiceEntry struct {
	services.ServiceRecord
	IsLocal bool `json:"is_local"`
}

// TransferRecord persists IWTP file transfer state for progress tracking and resumption.
type TransferRecord struct {
	ID            string   `json:"id"`
	PeerID        string   `json:"peer_id"`
	Direction     string   `json:"direction"` // "outbound" | "inbound"
	FileName      string   `json:"file_name"`
	FileSize      int64    `json:"file_size"`
	FileHash      string   `json:"file_hash"`
	Chunks        []string `json:"chunks"`
	ReceivedCount int      `json:"received_count"`
	ReceivedMap   []bool   `json:"received_map"`
	Status        string   `json:"status"` // "pending" | "active" | "completed" | "canceled" | "failed"
	SavePath      string   `json:"save_path"`
	ErrorMsg      string   `json:"error_msg,omitempty"`
	UpdatedAt     int64    `json:"updated_at"`
}

// SaveService inserts or updates a service record.
func (s *Store) SaveService(rec *services.ServiceRecord, isLocal bool) error {
	localInt := 0
	if isLocal {
		localInt = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO services (service_id, name, type, island_id, publisher_id, publisher_pub, site_version, manifest_hash, created_at, expires_at, signature, is_local)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(service_id) DO UPDATE SET
		   name = excluded.name,
		   site_version = excluded.site_version,
		   manifest_hash = excluded.manifest_hash,
		   created_at = excluded.created_at,
		   expires_at = excluded.expires_at,
		   signature = excluded.signature,
		   is_local = MAX(services.is_local, excluded.is_local)`,
		rec.ServiceID, rec.Name, rec.Type, rec.IslandID, rec.PublisherID, rec.PublisherPub,
		rec.SiteVersion, rec.ManifestHash, rec.CreatedAt, rec.ExpiresAt, rec.Signature, localInt,
	)
	return err
}

// GetServiceByNameOrID looks up a service by ServiceID or human-readable Name.
func (s *Store) GetServiceByNameOrID(nameOrID string) (*ServiceEntry, error) {
	row := s.db.QueryRow(
		`SELECT service_id, name, type, island_id, publisher_id, publisher_pub, site_version, manifest_hash, created_at, expires_at, signature, is_local
		 FROM services WHERE service_id = ? OR name = ? ORDER BY is_local DESC, site_version DESC LIMIT 1`,
		nameOrID, nameOrID,
	)
	var e ServiceEntry
	var localInt int
	if err := row.Scan(&e.ServiceID, &e.Name, &e.Type, &e.IslandID, &e.PublisherID, &e.PublisherPub,
		&e.SiteVersion, &e.ManifestHash, &e.CreatedAt, &e.ExpiresAt, &e.Signature, &localInt); err != nil {
		return nil, err
	}
	e.Version = 1
	e.IsLocal = localInt != 0
	return &e, nil
}

// ListServices returns all known services (local and discovered/cached).
func (s *Store) ListServices() ([]*ServiceEntry, error) {
	rows, err := s.db.Query(
		`SELECT service_id, name, type, island_id, publisher_id, publisher_pub, site_version, manifest_hash, created_at, expires_at, signature, is_local
		 FROM services ORDER BY is_local DESC, created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ServiceEntry
	for rows.Next() {
		var e ServiceEntry
		var localInt int
		if err := rows.Scan(&e.ServiceID, &e.Name, &e.Type, &e.IslandID, &e.PublisherID, &e.PublisherPub,
			&e.SiteVersion, &e.ManifestHash, &e.CreatedAt, &e.ExpiresAt, &e.Signature, &localInt); err != nil {
			return nil, err
		}
		e.Version = 1
		e.IsLocal = localInt != 0
		out = append(out, &e)
	}
	return out, rows.Err()
}

// SaveTransfer inserts or updates an IWTP transfer record.
func (s *Store) SaveTransfer(tr *TransferRecord) error {
	if tr.UpdatedAt == 0 {
		tr.UpdatedAt = time.Now().Unix()
	}
	chunksJSON, _ := json.Marshal(tr.Chunks)
	recJSON, _ := json.Marshal(tr.ReceivedMap)
	_, err := s.db.Exec(
		`INSERT INTO transfers (id, peer_id, direction, file_name, file_size, file_hash, chunks_json, received_json, status, save_path, error_msg, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   received_json = excluded.received_json,
		   status = excluded.status,
		   save_path = excluded.save_path,
		   error_msg = excluded.error_msg,
		   updated_at = excluded.updated_at`,
		tr.ID, tr.PeerID, tr.Direction, tr.FileName, tr.FileSize, tr.FileHash,
		string(chunksJSON), string(recJSON), tr.Status, tr.SavePath, tr.ErrorMsg, tr.UpdatedAt,
	)
	return err
}

// GetTransfer retrieves a transfer by ID.
func (s *Store) GetTransfer(id string) (*TransferRecord, error) {
	row := s.db.QueryRow(
		`SELECT id, peer_id, direction, file_name, file_size, file_hash, chunks_json, received_json, status, save_path, error_msg, updated_at
		 FROM transfers WHERE id = ?`, id,
	)
	return scanTransfer(row)
}

// ListTransfers returns recent transfers.
func (s *Store) ListTransfers(limit int) ([]*TransferRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT id, peer_id, direction, file_name, file_size, file_hash, chunks_json, received_json, status, save_path, error_msg, updated_at
		 FROM transfers ORDER BY updated_at DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*TransferRecord
	for rows.Next() {
		tr, err := scanTransfer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, tr)
	}
	return out, rows.Err()
}

func scanTransfer(s scanner) (*TransferRecord, error) {
	var tr TransferRecord
	var chunksRaw, recRaw string
	if err := s.Scan(&tr.ID, &tr.PeerID, &tr.Direction, &tr.FileName, &tr.FileSize, &tr.FileHash,
		&chunksRaw, &recRaw, &tr.Status, &tr.SavePath, &tr.ErrorMsg, &tr.UpdatedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(chunksRaw), &tr.Chunks)
	_ = json.Unmarshal([]byte(recRaw), &tr.ReceivedMap)
	for _, ok := range tr.ReceivedMap {
		if ok {
			tr.ReceivedCount++
		}
	}
	return &tr, nil
}

// DTNBundleRecord persists a DTN bundle and its local custody/delivery state.
type DTNBundleRecord struct {
	BundleID      string `json:"bundle_id"`
	SourceID      string `json:"source_id"`
	SourcePub     string `json:"source_pub"`
	DestinationID string `json:"destination_id"`
	IslandID      string `json:"island_id,omitempty"`
	Kind          string `json:"kind"`
	Priority      int    `json:"priority"`
	HopCount      int    `json:"hop_count"`
	MaxHops       int    `json:"max_hops"`
	CreatedAt     int64  `json:"created_at"`
	ExpiresAt     int64  `json:"expires_at"`
	EphemeralPub  string `json:"ephemeral_pub"`
	Ciphertext    string `json:"ciphertext"`
	Signature     string `json:"signature"`
	State         string `json:"state"` // "queued", "carrying", "delivered", "expired", "acked"
	CustodyHolder string `json:"custody_holder"`
	SizeBytes     int    `json:"size_bytes"`
}

// MarkDTNSeen records a bundle_id for duplicate suppression. Returns true if newly seen.
func (s *Store) MarkDTNSeen(bundleID string, now int64) (bool, error) {
	res, err := s.db.Exec(`INSERT OR IGNORE INTO dtn_seen (bundle_id, seen_at) VALUES (?, ?)`, bundleID, now)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// IsDTNSeen checks if bundle_id has already been seen.
func (s *Store) IsDTNSeen(bundleID string) bool {
	var id string
	err := s.db.QueryRow(`SELECT bundle_id FROM dtn_seen WHERE bundle_id = ?`, bundleID).Scan(&id)
	return err == nil
}

// CourierStorageUsed returns total ciphertext bytes currently held in queued/carrying state.
func (s *Store) CourierStorageUsed() (int64, error) {
	var total sql.NullInt64
	err := s.db.QueryRow(`SELECT SUM(size_bytes) FROM dtn_bundles WHERE state IN ('queued', 'carrying')`).Scan(&total)
	if err != nil {
		return 0, err
	}
	return total.Int64, nil
}

// SaveDTNBundle inserts or updates a DTN bundle record.
func (s *Store) SaveDTNBundle(r *DTNBundleRecord) error {
	_, err := s.db.Exec(
		`INSERT INTO dtn_bundles (bundle_id, source_id, source_pub, destination_id, island_id, kind, priority, hop_count, max_hops, created_at, expires_at, ephemeral_pub, ciphertext, signature, state, custody_holder, size_bytes)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(bundle_id) DO UPDATE SET
		   hop_count = excluded.hop_count,
		   state = excluded.state,
		   custody_holder = excluded.custody_holder`,
		r.BundleID, r.SourceID, r.SourcePub, r.DestinationID, r.IslandID, r.Kind,
		r.Priority, r.HopCount, r.MaxHops, r.CreatedAt, r.ExpiresAt,
		r.EphemeralPub, r.Ciphertext, r.Signature, r.State, r.CustodyHolder, r.SizeBytes,
	)
	return err
}

// UpdateDTNState updates the state and custody holder of a bundle.
func (s *Store) UpdateDTNState(bundleID, state, custodyHolder string) error {
	_, err := s.db.Exec(`UPDATE dtn_bundles SET state = ?, custody_holder = ? WHERE bundle_id = ?`, state, custodyHolder, bundleID)
	return err
}

// ListForwardableDTNBundles returns non-expired bundles in queued/carrying state ordered by priority.
func (s *Store) ListForwardableDTNBundles(nowUnix int64) ([]*DTNBundleRecord, error) {
	rows, err := s.db.Query(
		`SELECT bundle_id, source_id, source_pub, destination_id, island_id, kind, priority, hop_count, max_hops, created_at, expires_at, ephemeral_pub, ciphertext, signature, state, custody_holder, size_bytes
		 FROM dtn_bundles WHERE state IN ('queued', 'carrying') AND expires_at > ? AND hop_count < max_hops
		 ORDER BY priority DESC, created_at ASC`, nowUnix,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*DTNBundleRecord
	for rows.Next() {
		r, err := scanDTN(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListDTNBundles returns all stored DTN bundles (up to limit).
func (s *Store) ListDTNBundles(limit int) ([]*DTNBundleRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(
		`SELECT bundle_id, source_id, source_pub, destination_id, island_id, kind, priority, hop_count, max_hops, created_at, expires_at, ephemeral_pub, ciphertext, signature, state, custody_holder, size_bytes
		 FROM dtn_bundles ORDER BY created_at DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*DTNBundleRecord
	for rows.Next() {
		r, err := scanDTN(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func scanDTN(s scanner) (*DTNBundleRecord, error) {
	var r DTNBundleRecord
	err := s.Scan(&r.BundleID, &r.SourceID, &r.SourcePub, &r.DestinationID, &r.IslandID, &r.Kind,
		&r.Priority, &r.HopCount, &r.MaxHops, &r.CreatedAt, &r.ExpiresAt,
		&r.EphemeralPub, &r.Ciphertext, &r.Signature, &r.State, &r.CustodyHolder, &r.SizeBytes)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// SaveMail inserts or updates an IWMP mail record.
func (s *Store) SaveMail(m *iwmp.MailRecord) error {
	if m.Timestamp == 0 {
		m.Timestamp = time.Now().Unix()
	}
	attJSON, _ := json.Marshal(m.Attachments)
	_, err := s.db.Exec(
		`INSERT INTO mail (id, thread_id, island_id, from_id, to_id, subject, body, attachments_json, folder, status, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   folder = excluded.folder,
		   status = excluded.status,
		   subject = excluded.subject,
		   body = excluded.body,
		   attachments_json = excluded.attachments_json`,
		m.ID, m.ThreadID, m.IslandID, m.FromID, m.ToID, m.Subject, m.Body, string(attJSON), m.Folder, m.Status, m.Timestamp,
	)
	return err
}

// UpdateMailDelivery marks an outbound mail as delivered/acked and moves it from outbox to sent.
func (s *Store) UpdateMailDelivery(id, status string) error {
	_, err := s.db.Exec(`UPDATE mail SET status = ?, folder = 'sent' WHERE id = ? AND folder = 'outbox'`, status, id)
	return err
}

// ListMail returns mail in the given folder (or all folders if folder == ""), optionally filtered by search query.
func (s *Store) ListMail(folder, query string, limit int) ([]*iwmp.MailRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	q := `SELECT id, thread_id, island_id, from_id, to_id, subject, body, attachments_json, folder, status, timestamp FROM mail WHERE 1=1`
	var args []any
	if folder != "" {
		q += ` AND folder = ?`
		args = append(args, folder)
	}
	if query != "" {
		q += ` AND (subject LIKE ? OR body LIKE ? OR from_id LIKE ? OR to_id LIKE ?)`
		like := "%" + query + "%"
		args = append(args, like, like, like, like)
	}
	q += ` ORDER BY timestamp DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*iwmp.MailRecord
	for rows.Next() {
		var m iwmp.MailRecord
		var attRaw string
		if err := rows.Scan(&m.ID, &m.ThreadID, &m.IslandID, &m.FromID, &m.ToID, &m.Subject, &m.Body, &attRaw, &m.Folder, &m.Status, &m.Timestamp); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(attRaw), &m.Attachments)
		out = append(out, &m)
	}
	return out, rows.Err()
}

// SaveBulletin inserts or updates a verified IWBN bulletin.
func (s *Store) SaveBulletin(b *iwbn.Bulletin) error {
	emergInt := 0
	if b.OfficialEmergency {
		emergInt = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO bulletins (bulletin_id, channel, category, official_emergency, title, summary, body, island_id, publisher_id, publisher_pub, bulletin_version, created_at, expires_at, signature)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(bulletin_id) DO UPDATE SET
		   title = excluded.title,
		   summary = excluded.summary,
		   body = excluded.body,
		   bulletin_version = excluded.bulletin_version,
		   signature = excluded.signature`,
		b.BulletinID, b.Channel, b.Category, emergInt, b.Title, b.Summary, b.Body, b.IslandID,
		b.PublisherID, b.PublisherPub, b.BulletinVersion, b.CreatedAt, b.ExpiresAt, b.Signature,
	)
	return err
}

// ListBulletins returns bulletins, optionally filtered by channel.
func (s *Store) ListBulletins(channel string, limit int) ([]*iwbn.Bulletin, error) {
	if limit <= 0 {
		limit = 100
	}
	q := `SELECT bulletin_id, channel, category, official_emergency, title, summary, body, island_id, publisher_id, publisher_pub, bulletin_version, created_at, expires_at, signature FROM bulletins`
	var args []any
	if channel != "" {
		q += ` WHERE channel = ?`
		args = append(args, channel)
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*iwbn.Bulletin
	for rows.Next() {
		var b iwbn.Bulletin
		var emergInt int
		if err := rows.Scan(&b.BulletinID, &b.Channel, &b.Category, &emergInt, &b.Title, &b.Summary, &b.Body,
			&b.IslandID, &b.PublisherID, &b.PublisherPub, &b.BulletinVersion, &b.CreatedAt, &b.ExpiresAt, &b.Signature); err != nil {
			return nil, err
		}
		b.Version = 1
		b.OfficialEmergency = emergInt != 0
		out = append(out, &b)
	}
	return out, rows.Err()
}

// SubscribeChannel subscribes to an IWBN bulletin channel.
func (s *Store) SubscribeChannel(channel string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO bulletin_subscriptions (channel, subscribed_at) VALUES (?, ?)`, channel, time.Now().Unix())
	return err
}

// ListSubscriptions returns subscribed IWBN channels.
func (s *Store) ListSubscriptions() ([]string, error) {
	rows, err := s.db.Query(`SELECT channel FROM bulletin_subscriptions ORDER BY channel ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var ch string
		if err := rows.Scan(&ch); err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

// SaveFederation persists a signed Island federation agreement.
func (s *Store) SaveFederation(ag *federation.Agreement) error {
	svcJSON, _ := json.Marshal(ag.SharedServices)
	idJSON, _ := json.Marshal(ag.AllowedIdentities)
	bInt, dInt, rInt := 0, 0, 0
	if ag.AllowBulletinSync {
		bInt = 1
	}
	if ag.AllowDTNForward {
		dInt = 1
	}
	if ag.Revoked {
		rInt = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO federations (agreement_id, local_island_id, local_island_pub, remote_island_id, shared_services_json, allowed_identities_json, allow_bulletin_sync, allow_dtn_forward, created_at, expires_at, revoked, signature)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(agreement_id) DO UPDATE SET revoked = excluded.revoked`,
		ag.AgreementID, ag.LocalIslandID, ag.LocalIslandPub, ag.RemoteIslandID,
		string(svcJSON), string(idJSON), bInt, dInt, ag.CreatedAt, ag.ExpiresAt, rInt, ag.Signature,
	)
	return err
}

// RevokeFederation marks a federation agreement as revoked.
func (s *Store) RevokeFederation(agreementID string) error {
	res, err := s.db.Exec(`UPDATE federations SET revoked = 1 WHERE agreement_id = ?`, agreementID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ListFederations returns all stored federation agreements.
func (s *Store) ListFederations() ([]*federation.Agreement, error) {
	rows, err := s.db.Query(
		`SELECT agreement_id, local_island_id, local_island_pub, remote_island_id, shared_services_json, allowed_identities_json, allow_bulletin_sync, allow_dtn_forward, created_at, expires_at, revoked, signature
		 FROM federations ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*federation.Agreement
	for rows.Next() {
		var ag federation.Agreement
		var svcRaw, idRaw string
		var bInt, dInt, rInt int
		if err := rows.Scan(&ag.AgreementID, &ag.LocalIslandID, &ag.LocalIslandPub, &ag.RemoteIslandID,
			&svcRaw, &idRaw, &bInt, &dInt, &ag.CreatedAt, &ag.ExpiresAt, &rInt, &ag.Signature); err != nil {
			return nil, err
		}
		ag.Version = 1
		_ = json.Unmarshal([]byte(svcRaw), &ag.SharedServices)
		_ = json.Unmarshal([]byte(idRaw), &ag.AllowedIdentities)
		ag.AllowBulletinSync = bInt != 0
		ag.AllowDTNForward = dInt != 0
		ag.Revoked = rInt != 0
		out = append(out, &ag)
	}
	return out, rows.Err()
}

// BookmarkEntry represents a saved IslandWeave Navigator bookmark.
type BookmarkEntry struct {
	URI       string `json:"uri"`
	Title     string `json:"title"`
	CreatedAt int64  `json:"created_at"`
}

// HistoryEntry represents a Navigator browsing history entry.
type HistoryEntry struct {
	ID        int64  `json:"id"`
	URI       string `json:"uri"`
	Title     string `json:"title"`
	VisitedAt int64  `json:"visited_at"`
}

// ActivityEntry records user-visible daemon/network/sync events.
type ActivityEntry struct {
	ID        int64  `json:"id"`
	Category  string `json:"category"`
	Summary   string `json:"summary"`
	PeerID    string `json:"peer_id,omitempty"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"created_at"`
}

func (s *Store) SaveBookmark(uri, title string) error {
	if title == "" {
		title = uri
	}
	_, err := s.db.Exec(
		`INSERT INTO navigator_bookmarks (uri, title, created_at) VALUES (?, ?, ?)
		 ON CONFLICT(uri) DO UPDATE SET title = excluded.title`,
		uri, title, time.Now().Unix(),
	)
	return err
}

func (s *Store) DeleteBookmark(uri string) error {
	_, err := s.db.Exec(`DELETE FROM navigator_bookmarks WHERE uri = ?`, uri)
	return err
}

func (s *Store) ListBookmarks() ([]BookmarkEntry, error) {
	rows, err := s.db.Query(`SELECT uri, title, created_at FROM navigator_bookmarks ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BookmarkEntry
	for rows.Next() {
		var b BookmarkEntry
		if err := rows.Scan(&b.URI, &b.Title, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) AddHistory(uri, title string) error {
	if title == "" {
		title = uri
	}
	_, err := s.db.Exec(`INSERT INTO navigator_history (uri, title, visited_at) VALUES (?, ?, ?)`, uri, title, time.Now().Unix())
	return err
}

func (s *Store) ListHistory(limit int) ([]HistoryEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id, uri, title, visited_at FROM navigator_history ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HistoryEntry
	for rows.Next() {
		var h HistoryEntry
		if err := rows.Scan(&h.ID, &h.URI, &h.Title, &h.VisitedAt); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Store) RecordActivity(category, summary, peerID, status string) error {
	_, err := s.db.Exec(
		`INSERT INTO activity_log (category, summary, peer_id, status, created_at) VALUES (?, ?, ?, ?, ?)`,
		category, summary, peerID, status, time.Now().Unix(),
	)
	return err
}

func (s *Store) ListActivity(limit int) ([]ActivityEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT id, category, summary, peer_id, status, created_at FROM activity_log ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ActivityEntry
	for rows.Next() {
		var a ActivityEntry
		if err := rows.Scan(&a.ID, &a.Category, &a.Summary, &a.PeerID, &a.Status, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) SavePassLog(p *iwst.PassLog) error {
	intFlag := 0
	if p.Interrupted {
		intFlag = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO satellite_logs (pass_id, satellite_name, bytes_transferred, bundles_delivered, interrupted, completed_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(pass_id) DO UPDATE SET bytes_transferred=excluded.bytes_transferred, bundles_delivered=excluded.bundles_delivered, interrupted=excluded.interrupted, completed_at=excluded.completed_at`,
		p.PassID, p.SatelliteName, p.BytesTransferred, p.BundlesDelivered, intFlag, p.CompletedAt,
	)
	return err
}

func (s *Store) ListPassLogs(limit int) ([]iwst.PassLog, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT pass_id, satellite_name, bytes_transferred, bundles_delivered, interrupted, completed_at FROM satellite_logs ORDER BY completed_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []iwst.PassLog
	for rows.Next() {
		var p iwst.PassLog
		var intFlag int
		if err := rows.Scan(&p.PassID, &p.SatelliteName, &p.BytesTransferred, &p.BundlesDelivered, &intFlag, &p.CompletedAt); err != nil {
			return nil, err
		}
		p.Interrupted = intFlag != 0
		out = append(out, p)
	}
	return out, rows.Err()
}
