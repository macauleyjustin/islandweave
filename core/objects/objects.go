package objects

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const DefaultChunkSize = 256 * 1024 // 256 KiB

// Store provides content-addressed SHA-256 chunk storage under <dataDir>/objects.
type Store struct {
	root string
}

// NewStore initializes an IWP-OBJECT store at <dataDir>/objects.
func NewStore(dataDir string) (*Store, error) {
	root := filepath.Join(dataDir, "objects")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create objects dir: %w", err)
	}
	return &Store{root: root}, nil
}

// PutChunk verifies or computes the SHA-256 hash of data and stores it on disk.
func (s *Store) PutChunk(data []byte) (string, error) {
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	p := s.chunkPath(hash)
	if _, err := os.Stat(p); err == nil {
		return hash, nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return "", err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return hash, nil
}

// PutVerifiedChunk verifies that data hashes to expectedHash before storing it.
func (s *Store) PutVerifiedChunk(expectedHash string, data []byte) error {
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if got != expectedHash {
		return fmt.Errorf("chunk hash mismatch: got %s, want %s", got, expectedHash)
	}
	_, err := s.PutChunk(data)
	return err
}

// GetChunk reads and verifies a chunk by its SHA-256 hex hash.
func (s *Store) GetChunk(hash string) ([]byte, error) {
	if len(hash) != 64 {
		return nil, errors.New("invalid sha256 hash length")
	}
	data, err := os.ReadFile(s.chunkPath(hash))
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != hash {
		return nil, fmt.Errorf("stored chunk %s failed integrity check", hash)
	}
	return data, nil
}

// HasChunk checks whether a chunk exists in the local store.
func (s *Store) HasChunk(hash string) bool {
	if len(hash) != 64 {
		return false
	}
	_, err := os.Stat(s.chunkPath(hash))
	return err == nil
}

// IngestFile splits a file into fixed-size chunks, stores them, and returns the chunk hashes and total size.
func (s *Store) IngestFile(path string, chunkSize int) (chunks []string, totalSize int64, fileHash string, err error) {
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, "", err
	}
	defer f.Close()

	wholeHasher := sha256.New()
	buf := make([]byte, chunkSize)
	for {
		n, readErr := io.ReadFull(f, buf)
		if n > 0 {
			chunk := buf[:n]
			wholeHasher.Write(chunk)
			h, putErr := s.PutChunk(chunk)
			if putErr != nil {
				return nil, 0, "", putErr
			}
			chunks = append(chunks, h)
			totalSize += int64(n)
		}
		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			break
		}
		if readErr != nil {
			return nil, 0, "", readErr
		}
	}
	if len(chunks) == 0 {
		// Handle 0-byte empty file cleanly.
		h, putErr := s.PutChunk([]byte{})
		if putErr != nil {
			return nil, 0, "", putErr
		}
		chunks = append(chunks, h)
	}
	fileHash = hex.EncodeToString(wholeHasher.Sum(nil))
	return chunks, totalSize, fileHash, nil
}

// AssembleFile writes ordered chunks to outPath and verifies the final file SHA-256.
func (s *Store) AssembleFile(chunks []string, expectedFileHash, outPath string) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	tmp := outPath + ".iwtp.tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	wholeHasher := sha256.New()
	for _, ch := range chunks {
		data, err := s.GetChunk(ch)
		if err != nil {
			f.Close()
			_ = os.Remove(tmp)
			return fmt.Errorf("missing/corrupt chunk %s: %w", ch, err)
		}
		if _, err := f.Write(data); err != nil {
			f.Close()
			_ = os.Remove(tmp)
			return err
		}
		wholeHasher.Write(data)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	gotHash := hex.EncodeToString(wholeHasher.Sum(nil))
	if expectedFileHash != "" && gotHash != expectedFileHash {
		_ = os.Remove(tmp)
		return fmt.Errorf("assembled file hash mismatch: got %s, want %s", gotHash, expectedFileHash)
	}
	return os.Rename(tmp, outPath)
}

func (s *Store) chunkPath(hash string) string {
	return filepath.Join(s.root, hash[:2], hash)
}
