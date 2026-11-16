package security

import (
	"testing"
	"time"
)

func TestRateLimiterAndAuditEngine(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)
	now := time.Now()
	if !rl.Allow("iw:node:a", now) || !rl.Allow("iw:node:a", now) {
		t.Fatal("first two calls should be allowed")
	}
	if rl.Allow("iw:node:a", now) {
		t.Fatal("third call within window should be rejected")
	}
	if !rl.Allow("iw:node:a", now.Add(2*time.Minute)) {
		t.Fatal("call after window reset should succeed")
	}

	eng := NewEngine()
	perms := DefaultPermissions()
	perms.AllowAutoSync = false
	eng.SetPermissions(perms)
	if eng.Permissions().AllowAutoSync {
		t.Fatal("expected AllowAutoSync=false")
	}
	events := eng.AuditEvents(10)
	if len(events) == 0 || events[0].Category != "policy" {
		t.Fatalf("expected recorded policy audit event, got %+v", events)
	}
}
