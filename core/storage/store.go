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
	_ "modernc.org/sqlite"
)

// PeerRecord stores known peer metadata and authorization state.
type PeerRecord struct {
	NodeID    string   `json:"node_id"`
	Name      string   `json:"name"`
	PublicKey []byte   `json:"public_key"`
	Addrs     []string `json:"addrs"`
	Islands   []string `json:"islands"`
	Trusted   bool     `json:"trusted"`
	Revoked   bool     `json:"revoked"`
	LastSeen  int64    `json:"last_seen"`
	Source    string   `json:"source"` // "beacon", "manual", "invite", "inbound"
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
		`INSERT INTO peers (node_id, name, public_key, addrs, islands, trusted, revoked, last_seen, source)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(node_id) DO UPDATE SET
		   name = CASE WHEN excluded.name != '' THEN excluded.name ELSE peers.name END,
		   addrs = CASE WHEN excluded.addrs != '[]' AND excluded.addrs != 'null' THEN excluded.addrs ELSE peers.addrs END,
		   islands = CASE WHEN excluded.islands != '[]' AND excluded.islands != 'null' THEN excluded.islands ELSE peers.islands END,
		   last_seen = excluded.last_seen,
		   source = excluded.source`,
		p.NodeID, p.Name, p.PublicKey, string(addrsJSON), string(islandsJSON), trustedInt, revokedInt, p.LastSeen, p.Source,
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
	row := s.db.QueryRow(`SELECT node_id, name, public_key, addrs, islands, trusted, revoked, last_seen, source FROM peers WHERE node_id = ?`, nodeID)
	return scanPeer(row)
}

// ListPeers returns all known peers.
func (s *Store) ListPeers() ([]*PeerRecord, error) {
	rows, err := s.db.Query(`SELECT node_id, name, public_key, addrs, islands, trusted, revoked, last_seen, source FROM peers ORDER BY last_seen DESC`)
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
	if err := s.Scan(&p.NodeID, &p.Name, &p.PublicKey, &addrsRaw, &islandsRaw, &trustedInt, &revokedInt, &p.LastSeen, &p.Source); err != nil {
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
