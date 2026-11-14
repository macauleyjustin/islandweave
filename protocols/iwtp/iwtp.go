package iwtp

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/islandweave/islandweave/core/objects"
	"github.com/islandweave/islandweave/core/services"
	"github.com/islandweave/islandweave/core/storage"
	"github.com/islandweave/islandweave/core/transport"
)

const (
	TypeOffer     = "iwtp.offer"
	TypeChunkReq  = "iwtp.chunk_req"
	TypeChunkData = "iwtp.chunk_data"
	TypeComplete  = "iwtp.complete"
	TypeCancel    = "iwtp.cancel"

	TypeObjGet      = "obj.get"
	TypeObjData     = "obj.data"
	TypeSvcList     = "svc.list"
	TypeSvcListResp = "svc.list_resp"
)

type OfferPayload struct {
	TransferID string   `json:"transfer_id"`
	FileName   string   `json:"file_name"`
	FileSize   int64    `json:"file_size"`
	FileHash   string   `json:"file_hash"`
	Chunks     []string `json:"chunks"`
	ChunkSize  int      `json:"chunk_size"`
}

type ChunkReqPayload struct {
	TransferID string `json:"transfer_id"`
	ChunkIndex int    `json:"chunk_index"`
	ChunkHash  string `json:"chunk_hash"`
}

type ChunkDataPayload struct {
	TransferID string `json:"transfer_id"`
	ChunkIndex int    `json:"chunk_index"`
	ChunkHash  string `json:"chunk_hash"`
	DataB64    string `json:"data_b64"`
}

type CompletePayload struct {
	TransferID string `json:"transfer_id"`
}

type CancelPayload struct {
	TransferID string `json:"transfer_id"`
	Reason     string `json:"reason"`
}

type ObjGetPayload struct {
	Hash string `json:"hash"`
}

type ObjDataPayload struct {
	Hash    string `json:"hash"`
	DataB64 string `json:"data_b64,omitempty"`
	Error   string `json:"error,omitempty"`
}

type SvcListRespPayload struct {
	Services []services.ServiceRecord `json:"services"`
}

// ConflictPolicy controls what happens when a downloaded filename already exists.
type ConflictPolicy string

const (
	ConflictRename    ConflictPolicy = "rename"
	ConflictOverwrite ConflictPolicy = "overwrite"
	ConflictSkip      ConflictPolicy = "skip"
)

// ResolveDownloadPath sanitizes the filename (preventing path traversal) and applies ConflictPolicy.
func ResolveDownloadPath(downloadsDir, rawName string, policy ConflictPolicy) (string, bool) {
	base := filepath.Base(rawName)
	if base == "" || base == "." || base == ".." || base == string(filepath.Separator) {
		base = "download.bin"
	}
	target := filepath.Join(downloadsDir, base)
	if policy == ConflictOverwrite {
		return target, true
	}
	// Check existence via simple stat loop for rename
	if _, err := filepath.Abs(target); err == nil {
		if fileExists(target) {
			if policy == ConflictSkip {
				return target, false
			}
			ext := filepath.Ext(base)
			stem := strings.TrimSuffix(base, ext)
			for i := 1; i < 10000; i++ {
				cand := filepath.Join(downloadsDir, fmt.Sprintf("%s-%d%s", stem, i, ext))
				if !fileExists(cand) {
					return cand, true
				}
			}
		}
	}
	return target, true
}

// NextMissingChunk returns the index of the first unreceived chunk, or -1 if all are received.
func NextMissingChunk(tr *storage.TransferRecord, objStore *objects.Store) int {
	for i, h := range tr.Chunks {
		if i < len(tr.ReceivedMap) && tr.ReceivedMap[i] && objStore.HasChunk(h) {
			continue
		}
		if objStore.HasChunk(h) {
			if i < len(tr.ReceivedMap) {
				tr.ReceivedMap[i] = true
			}
			continue
		}
		return i
	}
	return -1
}

// MakeEnvelope builds a signed/attributed Envelope for an IWTP or service payload.
func MakeEnvelope(senderID, msgType string, payload any) (*transport.Envelope, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &transport.Envelope{
		Version:   transport.ProtocolVersion,
		Type:      msgType,
		ID:        uuid.NewString(),
		SenderID:  senderID,
		Timestamp: time.Now().Unix(),
		Payload:   raw,
	}, nil
}

// EncodeData encodes raw chunk bytes for JSON envelope transport.
func EncodeData(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

// DecodeData decodes base64 chunk bytes.
func DecodeData(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
