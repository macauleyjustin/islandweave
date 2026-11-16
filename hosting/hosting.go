package hosting

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/islandweave/islandweave/core/identity"
	"github.com/islandweave/islandweave/core/objects"
	"github.com/islandweave/islandweave/core/services"
)

// PublishStaticSite ingests all files in dir into objStore and returns a signed ServiceRecord and SiteManifest.
func PublishStaticSite(objStore *objects.Store, id *identity.NodeIdentity, dir, name, islandID string, siteVersion int, ttl time.Duration) (*services.ServiceRecord, *services.SiteManifest, error) {
	return PublishContent(objStore, id, dir, name, "website", islandID, siteVersion, ttl)
}

// PublishSharedDir ingests all files in dir as a browsable Shared File Library ("share") service.
func PublishSharedDir(objStore *objects.Store, id *identity.NodeIdentity, dir, name, islandID string, siteVersion int, ttl time.Duration) (*services.ServiceRecord, *services.SiteManifest, error) {
	return PublishContent(objStore, id, dir, name, "share", islandID, siteVersion, ttl)
}

// PublishContent ingests all files in dir into objStore and signs a ServiceRecord of svcType ("website" or "share").
func PublishContent(objStore *objects.Store, id *identity.NodeIdentity, dir, name, svcType, islandID string, siteVersion int, ttl time.Duration) (*services.ServiceRecord, *services.SiteManifest, error) {
	if svcType == "" {
		svcType = "website"
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("stat site dir: %w", err)
	}
	if !info.IsDir() {
		return nil, nil, errors.New("site path must be a directory")
	}
	if name == "" {
		name = filepath.Base(filepath.Clean(dir))
	}
	if siteVersion <= 0 {
		siteVersion = 1
	}

	svcID := services.DeriveServiceID(id.ID, name)
	var files []services.SiteFileEntry

	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil // skip symlinks for safety
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		chunks, size, fileHash, err := objStore.IngestFile(p, objects.DefaultChunkSize)
		if err != nil {
			return fmt.Errorf("ingest %s: %w", rel, err)
		}
		ct := mime.TypeByExtension(filepath.Ext(p))
		if ct == "" {
			ct = "application/octet-stream"
		}
		files = append(files, services.SiteFileEntry{
			Path:        rel,
			Size:        size,
			ContentType: ct,
			FileHash:    fileHash,
			Chunks:      chunks,
		})
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if len(files) == 0 {
		return nil, nil, errors.New("site directory contains no files")
	}

	manifest := &services.SiteManifest{
		Version:     1,
		ServiceID:   svcID,
		Name:        name,
		SiteVersion: siteVersion,
		PublisherID: id.ID,
		Files:       files,
		CreatedAt:   time.Now().Unix(),
	}
	mBytes, err := json.Marshal(manifest)
	if err != nil {
		return nil, nil, err
	}
	manifestHash, err := objStore.PutChunk(mBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("store site manifest: %w", err)
	}

	rec, err := services.SignServiceRecord(id, name, svcType, islandID, manifestHash, siteVersion, ttl)
	if err != nil {
		return nil, nil, err
	}
	return rec, manifest, nil
}

// LoadSiteManifest loads and parses a SiteManifest from objStore.
func LoadSiteManifest(objStore *objects.Store, manifestHash string) (*services.SiteManifest, error) {
	raw, err := objStore.GetChunk(manifestHash)
	if err != nil {
		return nil, err
	}
	var m services.SiteManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("unmarshal site manifest: %w", err)
	}
	return &m, nil
}

// ReadSiteFile reads and verifies a file from a SiteManifest using objStore.
func ReadSiteFile(objStore *objects.Store, manifest *services.SiteManifest, relPath string) (*services.SiteFileEntry, []byte, error) {
	clean := strings.TrimPrefix(filepath.ToSlash(filepath.Clean("/"+relPath)), "/")
	if clean == "" || clean == "." {
		clean = "index.html"
	}
	for i := range manifest.Files {
		f := &manifest.Files[i]
		if f.Path == clean {
			var buf bytes.Buffer
			for _, ch := range f.Chunks {
				data, err := objStore.GetChunk(ch)
				if err != nil {
					return nil, nil, err
				}
				buf.Write(data)
			}
			return f, buf.Bytes(), nil
		}
	}
	return nil, nil, os.ErrNotExist
}
