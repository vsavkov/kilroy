package engine

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/danshapiro/kilroy/internal/cxdb"
	"github.com/zeebo/blake3"
)

// CXDBSink appends normalized Attractor events to a CXDB context and stores
// large artifacts in CXDB's blob CAS.
//
// v1 implementation notes:
// - Prefers binary protocol for mutating operations; falls back to HTTP compat routes.
// - Serializes appends to maintain a linear head within a context.
type CXDBSink struct {
	Client *cxdb.Client
	Binary *cxdb.BinaryClient

	RunID      string
	ContextID  string
	HeadTurnID string
	BundleID   string

	mu sync.Mutex
}

func NewCXDBSink(client *cxdb.Client, binary *cxdb.BinaryClient, runID, contextID, headTurnID, bundleID string) *CXDBSink {
	return &CXDBSink{
		Client:     client,
		Binary:     binary,
		RunID:      runID,
		ContextID:  contextID,
		HeadTurnID: headTurnID,
		BundleID:   bundleID,
	}
}

func (s *CXDBSink) append(ctx context.Context, req cxdb.AppendTurnRequest) (turnID string, contentHash string, err error) {
	if s == nil || (s.Client == nil && s.Binary == nil) {
		return "", "", fmt.Errorf("cxdb sink is nil")
	}
	if req.Data == nil {
		req.Data = map[string]any{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(req.ParentTurnID) == "" {
		req.ParentTurnID = s.HeadTurnID
	}

	var binErr error
	if s.Binary != nil {
		ctxID, err := strconv.ParseUint(strings.TrimSpace(s.ContextID), 10, 64)
		if err != nil {
			binErr = fmt.Errorf("cxdb binary append: invalid context_id %q: %w", s.ContextID, err)
		} else {
			parent := uint64(0)
			if p := strings.TrimSpace(req.ParentTurnID); p != "" {
				parent, err = strconv.ParseUint(p, 10, 64)
				if err != nil {
					binErr = fmt.Errorf("cxdb binary append: invalid parent_turn_id %q: %w", p, err)
				}
			}
			if binErr == nil {
				payload, err := cxdb.EncodeTurnPayload(req.TypeID, req.TypeVersion, req.Data)
				if err != nil {
					binErr = err
				} else {
					ack, err := s.Binary.AppendTurn(ctx, ctxID, parent, req.TypeID, uint32(req.TypeVersion), payload)
					if err == nil {
						s.HeadTurnID = strconv.FormatUint(ack.NewTurnID, 10)
						return s.HeadTurnID, hex.EncodeToString(ack.ContentHash[:]), nil
					}
					binErr = err
				}
			}
		}
	}

	if s.Client != nil {
		resp, err := s.Client.AppendTurn(ctx, s.ContextID, req)
		if err == nil {
			s.HeadTurnID = resp.TurnID
			return resp.TurnID, resp.ContentHash, nil
		}
		if binErr != nil {
			return "", "", fmt.Errorf("cxdb append failed (binary=%v, http=%v)", binErr, err)
		}
		return "", "", err
	}

	if binErr != nil {
		return "", "", binErr
	}
	return "", "", fmt.Errorf("cxdb append failed: no append transport available")
}

func (s *CXDBSink) Append(ctx context.Context, typeID string, typeVersion int, data map[string]any) (turnID string, contentHash string, err error) {
	return s.append(ctx, cxdb.AppendTurnRequest{
		TypeID:      typeID,
		TypeVersion: typeVersion,
		Data:        data,
	})
}

func (s *CXDBSink) ForkFromHead(ctx context.Context) (*CXDBSink, error) {
	if s == nil || (s.Client == nil && s.Binary == nil) {
		return nil, fmt.Errorf("cxdb sink is nil")
	}
	base := s.HeadTurnID
	if strings.TrimSpace(base) == "" {
		base = "0"
	}

	var binErr error
	if s.Binary != nil {
		if _, err := strconv.ParseUint(strings.TrimSpace(s.ContextID), 10, 64); err != nil {
			binErr = fmt.Errorf("cxdb binary fork: invalid context_id %q: %w", s.ContextID, err)
		} else {
			baseID, err := strconv.ParseUint(strings.TrimSpace(base), 10, 64)
			if err != nil {
				binErr = fmt.Errorf("cxdb binary fork: invalid base_turn_id %q: %w", base, err)
			} else {
				ci, err := s.Binary.ForkContext(ctx, baseID)
				if err == nil {
					return NewCXDBSink(
						s.Client,
						s.Binary,
						s.RunID,
						strconv.FormatUint(ci.ContextID, 10),
						strconv.FormatUint(ci.HeadTurnID, 10),
						s.BundleID,
					), nil
				}
				binErr = err
			}
		}
	}

	if s.Client != nil {
		ci, err := s.Client.ForkContext(ctx, base)
		if err == nil {
			return NewCXDBSink(s.Client, s.Binary, s.RunID, ci.ContextID, ci.HeadTurnID, s.BundleID), nil
		}
		if binErr != nil {
			return nil, fmt.Errorf("cxdb fork failed (binary=%v, http=%v)", binErr, err)
		}
		return nil, err
	}
	if binErr != nil {
		return nil, binErr
	}
	return nil, fmt.Errorf("cxdb fork failed: no fork transport available")
}

func (s *CXDBSink) PutArtifactFile(ctx context.Context, nodeID, logicalName, path string) (artifactTurnID string, err error) {
	if s == nil || s.Client == nil || s.Binary == nil {
		return "", fmt.Errorf("cxdb sink is nil")
	}

	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return "", err
	}
	rawLen := fi.Size()
	// CXDB PUT_BLOB payload is length-prefixed with a u32 and includes 36 bytes of overhead: hash(32)+raw_len(4).
	const putBlobOverhead = int64(32 + 4)
	maxBlobLen := int64(^uint32(0)) - putBlobOverhead
	if rawLen < 0 || rawLen > maxBlobLen {
		_ = f.Close()
		return "", fmt.Errorf("cxdb artifact too large for binary protocol (u32 frame len): %s size=%d", path, rawLen)
	}

	h := blake3.New()
	n, err := io.Copy(h, f)
	_ = f.Close()
	if err != nil {
		return "", err
	}
	if n != rawLen {
		// Be strict: PUT_BLOB must read exactly rawLen bytes.
		return "", fmt.Errorf("cxdb artifact read: size mismatch: stat=%d read=%d path=%s", rawLen, n, path)
	}
	sumBytes := h.Sum(nil)
	if len(sumBytes) != 32 {
		return "", fmt.Errorf("cxdb artifact hash: unexpected digest len=%d", len(sumBytes))
	}
	var sum [32]byte
	copy(sum[:], sumBytes)

	// Store raw bytes in CXDB's blob CAS (deduped; fetchable via HTTP GET /v1/blobs/:content_hash).
	f2, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f2.Close() }()
	if _, err := s.Binary.PutBlob(ctx, sum, uint32(rawLen), f2); err != nil {
		return "", err
	}
	blobHashHex := hex.EncodeToString(sum[:])

	mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if mimeType == "" {
		// best-effort fallbacks
		switch strings.ToLower(filepath.Ext(path)) {
		case ".md":
			mimeType = "text/markdown"
		case ".json":
			mimeType = "application/json"
		case ".ndjson":
			mimeType = "application/x-ndjson"
		case ".tgz", ".tar.gz":
			mimeType = "application/gzip"
		default:
			mimeType = "application/octet-stream"
		}
	}

	idemKey := fmt.Sprintf("kilroy:artifact:%s:%s:%s:%s", s.RunID, nodeID, logicalName, blobHashHex)
	turnID, _, err := s.append(ctx, cxdb.AppendTurnRequest{
		TypeID:      "com.kilroy.attractor.Artifact",
		TypeVersion: 1,
		Data: map[string]any{
			"run_id":       s.RunID,
			"node_id":      nodeID,
			"name":         logicalName,
			"mime":         mimeType,
			"content_hash": blobHashHex,
			"bytes_len":    uint64(rawLen),
			"local_path":   path,
		},
		IdempotencyKey: idemKey,
	})
	if err != nil {
		return "", err
	}
	return turnID, nil
}

func nowMS() uint64 { return uint64(time.Now().UTC().UnixNano() / int64(time.Millisecond)) }
