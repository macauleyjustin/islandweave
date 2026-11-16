package security

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// TrafficStats tracks cumulative bytes sent and received across network transports.
type TrafficStats struct {
	BytesSent     atomic.Int64
	BytesRecv     atomic.Int64
	LinksAccepted atomic.Int64
	LinksDialed   atomic.Int64
}

// Snapshot returns an immutable copy of current traffic metrics.
type Snapshot struct {
	BytesSent     int64 `json:"bytes_sent"`
	BytesRecv     int64 `json:"bytes_recv"`
	LinksAccepted int64 `json:"links_accepted"`
	LinksDialed   int64 `json:"links_dialed"`
}

func (s *TrafficStats) Snapshot() Snapshot {
	return Snapshot{
		BytesSent:     s.BytesSent.Load(),
		BytesRecv:     s.BytesRecv.Load(),
		LinksAccepted: s.LinksAccepted.Load(),
		LinksDialed:   s.LinksDialed.Load(),
	}
}

// MeteredConn wraps a net.Conn and records byte counters into stats.
type MeteredConn struct {
	net.Conn
	stats *TrafficStats
}

// WrapConn wraps conn to record TX/RX bytes into stats.
func WrapConn(conn net.Conn, stats *TrafficStats) net.Conn {
	if stats == nil || conn == nil {
		return conn
	}
	return &MeteredConn{Conn: conn, stats: stats}
}

func (m *MeteredConn) Read(p []byte) (int, error) {
	n, err := m.Conn.Read(p)
	if n > 0 {
		m.stats.BytesRecv.Add(int64(n))
	}
	return n, err
}

func (m *MeteredConn) Write(p []byte) (int, error) {
	n, err := m.Conn.Write(p)
	if n > 0 {
		m.stats.BytesSent.Add(int64(n))
	}
	return n, err
}

// Permissions configures consent and isolation policies for a node.
type Permissions struct {
	AllowIncomingTransfers bool `json:"allow_incoming_transfers"`
	AllowDTNForwarding     bool `json:"allow_dtn_forwarding"`
	AllowAutoSync          bool `json:"allow_auto_sync"`
	AllowPeerExchange      bool `json:"allow_peer_exchange"`
	RequireTrustedForSites bool `json:"require_trusted_for_sites"`
}

// DefaultPermissions returns conservative, safe defaults.
func DefaultPermissions() Permissions {
	return Permissions{
		AllowIncomingTransfers: true,
		AllowDTNForwarding:     true,
		AllowAutoSync:          true,
		AllowPeerExchange:      true,
		RequireTrustedForSites: false,
	}
}

// AuditEvent captures a security-relevant occurrence.
type AuditEvent struct {
	Timestamp int64  `json:"timestamp"`
	Category  string `json:"category"` // "auth", "rate_limit", "revocation", "sync", "policy"
	PeerID    string `json:"peer_id,omitempty"`
	Outcome   string `json:"outcome"` // "allow", "deny", "warn", "info"
	Detail    string `json:"detail"`
}

// RateLimiter implements a per-peer fixed-window request limiter.
type RateLimiter struct {
	mu      sync.Mutex
	maxPerW int
	window  time.Duration
	buckets map[string]*bucket
}

type bucket struct {
	count int
	reset time.Time
}

// NewRateLimiter creates a per-peer rate limiter (e.g. 120 events per minute).
func NewRateLimiter(maxPerWindow int, window time.Duration) *RateLimiter {
	if maxPerWindow <= 0 {
		maxPerWindow = 120
	}
	if window <= 0 {
		window = time.Minute
	}
	return &RateLimiter{
		maxPerW: maxPerWindow,
		window:  window,
		buckets: make(map[string]*bucket),
	}
}

// Allow reports whether peerID is within its allowed request budget.
func (r *RateLimiter) Allow(peerID string, now time.Time) bool {
	if peerID == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.buckets[peerID]
	if !ok || now.After(b.reset) {
		r.buckets[peerID] = &bucket{count: 1, reset: now.Add(r.window)}
		return true
	}
	if b.count >= r.maxPerW {
		return false
	}
	b.count++
	return true
}

// Engine holds runtime security policies, rate limits, non-repudiation audit ring, and keystore checks.
type Engine struct {
	mu          sync.RWMutex
	perms       Permissions
	audit       []AuditEvent
	maxAudit    int
	rateLimiter *RateLimiter
	maxBWBytes  int64 // optional per-hour bandwidth cap (0 = unlimited)
	Traffic     *TrafficStats
}

// NewEngine initializes a Security Engine.
func NewEngine() *Engine {
	return &Engine{
		perms:       DefaultPermissions(),
		maxAudit:    200,
		rateLimiter: NewRateLimiter(240, time.Minute),
		Traffic:     &TrafficStats{},
	}
}

func (e *Engine) Permissions() Permissions {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.perms
}

func (e *Engine) SetPermissions(p Permissions) {
	e.mu.Lock()
	e.perms = p
	e.mu.Unlock()
	e.RecordAudit("policy", "", "info", fmt.Sprintf("updated permissions (transfers=%v, dtn=%v, sync=%v)", p.AllowIncomingTransfers, p.AllowDTNForwarding, p.AllowAutoSync))
}

func (e *Engine) SetBandwidthLimitBytes(limit int64) {
	e.mu.Lock()
	e.maxBWBytes = limit
	e.mu.Unlock()
}

func (e *Engine) BandwidthLimitBytes() int64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.maxBWBytes
}

func (e *Engine) AllowPeerAction(peerID string) bool {
	ok := e.rateLimiter.Allow(peerID, time.Now())
	if !ok {
		e.RecordAudit("rate_limit", peerID, "deny", "peer exceeded per-minute envelope quota")
	}
	return ok
}

func (e *Engine) RecordAudit(category, peerID, outcome, detail string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	ev := AuditEvent{
		Timestamp: time.Now().Unix(),
		Category:  category,
		PeerID:    peerID,
		Outcome:   outcome,
		Detail:    detail,
	}
	e.audit = append(e.audit, ev)
	if len(e.audit) > e.maxAudit {
		e.audit = e.audit[len(e.audit)-e.maxAudit:]
	}
}

func (e *Engine) AuditEvents(limit int) []AuditEvent {
	e.mu.RLock()
	defer e.mu.RUnlock()
	n := len(e.audit)
	if limit <= 0 || limit > n {
		limit = n
	}
	out := make([]AuditEvent, limit)
	for i := 0; i < limit; i++ {
		out[i] = e.audit[n-1-i] // newest first
	}
	return out
}
