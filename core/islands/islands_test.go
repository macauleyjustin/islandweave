package islands

import (
	"testing"
	"time"
)

func TestIslandCreateInviteAndVerify(t *testing.T) {
	isl, err := CreateIsland("Community Net", VisibilityInviteOnly)
	if err != nil {
		t.Fatalf("CreateIsland: %v", err)
	}
	tok, err := CreateInvite(isl, "iw:node:inviter", []string{"127.0.0.1:47472"}, time.Hour, 0)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	encoded, err := EncodeInvite(tok)
	if err != nil {
		t.Fatalf("EncodeInvite: %v", err)
	}
	decoded, err := DecodeInvite(encoded)
	if err != nil {
		t.Fatalf("DecodeInvite: %v", err)
	}
	joined, err := VerifyInvite(decoded, time.Now())
	if err != nil {
		t.Fatalf("VerifyInvite: %v", err)
	}
	if joined.ID != isl.ID || joined.Name != isl.Name || joined.Role != "member" {
		t.Fatalf("unexpected joined island: %+v", joined)
	}

	// Expired invite should fail
	if _, err := VerifyInvite(decoded, time.Now().Add(2*time.Hour)); err == nil {
		t.Fatalf("expected expired invite to fail")
	}

	// Tampered invite should fail
	decoded.IslandName = "Evil Net"
	if _, err := VerifyInvite(decoded, time.Now()); err == nil {
		t.Fatalf("expected tampered invite to fail")
	}
}
