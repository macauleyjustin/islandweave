package identity

import (
	"testing"
)

func TestIdentitySaveLoadAndSign(t *testing.T) {
	dir := t.TempDir()
	id, created, err := LoadOrCreateNodeIdentity(dir, "test-node")
	if err != nil {
		t.Fatalf("LoadOrCreateNodeIdentity: %v", err)
	}
	if !created {
		t.Fatalf("expected created=true")
	}

	id2, created2, err := LoadOrCreateNodeIdentity(dir, "other")
	if err != nil {
		t.Fatalf("reload identity: %v", err)
	}
	if created2 {
		t.Fatalf("expected created=false on reload")
	}
	if id.ID != id2.ID {
		t.Fatalf("ID mismatch: %s vs %s", id.ID, id2.ID)
	}

	msg := []byte("hello islandweave")
	sig := SignDomain(id.PrivateKey, "TEST-DOMAIN", msg)
	if !VerifyDomain(id.PublicKey, "TEST-DOMAIN", msg, sig) {
		t.Fatalf("expected valid signature")
	}
	if VerifyDomain(id.PublicKey, "WRONG-DOMAIN", msg, sig) {
		t.Fatalf("expected signature verification to fail on wrong domain")
	}
	if VerifyDomain(id.PublicKey, "TEST-DOMAIN", []byte("tampered"), sig) {
		t.Fatalf("expected signature verification to fail on tampered data")
	}
}
