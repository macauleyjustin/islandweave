package dtn

import (
	"bytes"
	"testing"
	"time"

	"github.com/islandweave/islandweave/core/identity"
)

func TestDTNBundleEncryptVerifyAndDecrypt(t *testing.T) {
	alice, err := identity.GenerateNodeIdentity("alice")
	if err != nil {
		t.Fatal(err)
	}
	courier, err := identity.GenerateNodeIdentity("courier")
	if err != nil {
		t.Fatal(err)
	}
	carol, err := identity.GenerateNodeIdentity("carol")
	if err != nil {
		t.Fatal(err)
	}

	secret := []byte("meet at harbor dock at 0900")
	b, err := CreateBundle(alice, carol.ID, carol.EncPublicKey, "iw:island:1", "msg", 1, 4, time.Hour, secret)
	if err != nil {
		t.Fatalf("CreateBundle: %v", err)
	}

	// Courier increments HopCount and verifies signature without decrypting
	b.HopCount++
	if err := VerifyBundle(b, time.Now()); err != nil {
		t.Fatalf("VerifyBundle after hop: %v", err)
	}

	// Courier must NOT be able to decrypt
	if _, err := DecryptBundle(b, courier); err == nil {
		t.Fatalf("courier should not be able to decrypt Carol's bundle")
	}

	// Carol decrypts successfully
	got, err := DecryptBundle(b, carol)
	if err != nil {
		t.Fatalf("DecryptBundle: %v", err)
	}
	if !bytes.Equal(got, secret) {
		t.Fatalf("plaintext mismatch: got %q", got)
	}

	// Expired bundle fails verification
	if err := VerifyBundle(b, time.Now().Add(2*time.Hour)); err == nil {
		t.Fatalf("expected expired bundle to fail verification")
	}

	// Exceeding MaxHops fails verification
	b.HopCount = b.MaxHops + 1
	if err := VerifyBundle(b, time.Now()); err == nil {
		t.Fatalf("expected hop-exceeded bundle to fail verification")
	}
}
