package objects

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
)

func TestObjectStoreIngestVerifyAndAssemble(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	raw := make([]byte, 600*1024) // ~600 KiB -> 3 chunks at 256 KiB
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	srcPath := filepath.Join(dir, "sample.bin")
	if err := os.WriteFile(srcPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	chunks, size, fileHash, err := store.IngestFile(srcPath, DefaultChunkSize)
	if err != nil {
		t.Fatalf("IngestFile: %v", err)
	}
	if size != int64(len(raw)) || len(chunks) != 3 {
		t.Fatalf("unexpected chunks=%d size=%d", len(chunks), size)
	}

	// Corrupt chunk must be rejected
	if err := store.PutVerifiedChunk(chunks[0], []byte("corrupt-data")); err == nil {
		t.Fatalf("expected PutVerifiedChunk to reject corrupt data")
	}

	outPath := filepath.Join(dir, "assembled.bin")
	if err := store.AssembleFile(chunks, fileHash, outPath); err != nil {
		t.Fatalf("AssembleFile: %v", err)
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, got) {
		t.Fatalf("assembled file bytes do not match original")
	}
}
