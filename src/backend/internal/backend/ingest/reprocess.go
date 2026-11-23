package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"scriptorium/internal/backend/classification"
	"scriptorium/internal/backend/dao"
	"scriptorium/internal/backend/formats"

	"github.com/google/uuid"
)

// MaxConcurrentReextractions bounds active staging and extraction requests per service.
const MaxConcurrentReextractions = 2

var (
	// ErrReprocessCapacity indicates that all reprocessing slots are occupied.
	ErrReprocessCapacity = errors.New("text extraction capacity is busy; try again shortly")
	// ErrReprocessBusy indicates that this process is already re-extracting the item.
	ErrReprocessBusy = errors.New("text extraction is already running for this document")
	// ErrOriginalUnavailable indicates that the stored original could not be read.
	ErrOriginalUnavailable = errors.New("the stored original is unavailable; previous indexed text was retained")
	// ErrOriginalChanged indicates a digest mismatch, not permission to replace the original.
	ErrOriginalChanged = errors.New("the original failed its SHA-256 integrity check; previous indexed text was retained")
	// ErrExtractionUnsupported indicates that no text adapter handles this item.
	ErrExtractionUnsupported = errors.New("text extraction is unsupported for this format; previous indexed text was retained")
	// ErrReprocessFailed indicates that extraction did not produce a usable result.
	ErrReprocessFailed = errors.New("text extraction failed; previous indexed text and metadata were retained")
)

// ReprocessRepository supports conditional updates to existing catalogue items.
type ReprocessRepository interface {
	ReadRaw(uuid.UUID) ([]byte, error)
	ReplaceExtraction(context.Context, uuid.UUID, []byte, dao.ExtractionUpdate) (dao.MetaData, error)
}

// OriginalReader provides read-only access to storage-root-confined originals.
type OriginalReader interface {
	GetFile(string) (io.ReadCloser, error)
}

// Reprocessor extracts text from existing originals without changing their bytes
// or ingestion identity. Active requests for the same item fail fast.
type Reprocessor struct {
	repository ReprocessRepository
	storage    OriginalReader
	adapters   *formats.AdapterRegistry
	active     sync.Map
	slots      chan struct{}
}

// NewReprocessor creates a reprocessor using the same optional adapters as ingestion.
func NewReprocessor(repository ReprocessRepository, storage OriginalReader, adapters *formats.AdapterRegistry) (*Reprocessor, error) {
	if repository == nil || storage == nil {
		return nil, errors.New("catalogue and original storage are required")
	}
	if adapters == nil {
		adapters = formats.NewDefaultAdapterRegistry()
	}
	return &Reprocessor{repository: repository, storage: storage, adapters: adapters, slots: make(chan struct{}, MaxConcurrentReextractions)}, nil
}

// Reextract stages and verifies an existing original before extracting text.
// Unavailable tools, unsupported formats, failures, and cancellation do not replace
// previously indexed text. A successful empty PDF extraction intentionally clears
// old text. A one-minute outer deadline also bounds queueing and source staging.
func (r *Reprocessor) Reextract(ctx context.Context, id uuid.UUID) (dao.MetaData, error) {
	if err := ctx.Err(); err != nil {
		return dao.MetaData{}, err
	}
	if _, busy := r.active.LoadOrStore(id, struct{}{}); busy {
		return dao.MetaData{}, ErrReprocessBusy
	}
	defer r.active.Delete(id)
	select {
	case r.slots <- struct{}{}:
		defer func() { <-r.slots }()
	default:
		return dao.MetaData{}, ErrReprocessCapacity
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	raw, err := r.repository.ReadRaw(id)
	if err != nil {
		return dao.MetaData{}, err
	}
	var metadata dao.MetaData
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return dao.MetaData{}, ErrReprocessFailed
	}
	original, err := r.storage.GetFile(metadata.Path)
	if err != nil {
		return dao.MetaData{}, ErrOriginalUnavailable
	}
	defer original.Close()
	staged, err := os.CreateTemp("", "scriptorium-reextract-*")
	if err != nil {
		return dao.MetaData{}, ErrReprocessFailed
	}
	defer os.Remove(staged.Name())
	defer staged.Close()
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(staged, hash), io.LimitReader(contextReader{ctx: ctx, reader: original}, DefaultMaxFileSize+1))
	if ctx.Err() != nil {
		return dao.MetaData{}, ctx.Err()
	}
	if err != nil {
		return dao.MetaData{}, ErrOriginalUnavailable
	}
	if written == 0 || written > DefaultMaxFileSize {
		return dao.MetaData{}, ErrReprocessFailed
	}
	if metadata.SHA256 != "" && !strings.EqualFold(metadata.SHA256, hex.EncodeToString(hash.Sum(nil))) {
		return dao.MetaData{}, ErrOriginalChanged
	}
	if _, err := staged.Seek(0, io.SeekStart); err != nil {
		return dao.MetaData{}, ErrReprocessFailed
	}
	filename := metadata.OriginalFilename
	if filename == "" {
		filename = "document"
		if metadata.FileType != "" {
			filename += "." + strings.TrimPrefix(metadata.FileType, ".")
		}
	}
	identified, err := formats.Identify(filename, staged)
	if err != nil {
		return dao.MetaData{}, ErrReprocessFailed
	}
	if _, err := staged.Seek(0, io.SeekStart); err != nil {
		return dao.MetaData{}, ErrReprocessFailed
	}
	extracted, status, err := extractDocument(ctx, r.adapters, filename, identified.MediaType, staged)
	if ctx.Err() != nil {
		return dao.MetaData{}, ctx.Err()
	}
	if status == "unavailable" {
		return dao.MetaData{}, formats.ErrExtractorUnavailable
	}
	if status == "unsupported" {
		return dao.MetaData{}, ErrExtractionUnsupported
	}
	if err != nil {
		return dao.MetaData{}, ErrReprocessFailed
	}
	update := dao.ExtractionUpdate{Text: extracted.Text, Status: status}
	if metadata.DeweyDecimal == "" && metadata.ClassificationStatus != "confirmed" {
		suggestions := classification.Suggest(metadata.Title+" "+metadata.Author+" "+extracted.Text, 1)
		if len(suggestions) > 0 {
			update.Dewey, update.DeweyName = suggestions[0].Code, suggestions[0].Name
			update.Confidence, update.Evidence = suggestions[0].Confidence, suggestions[0].Evidence
		}
	}
	return r.repository.ReplaceExtraction(ctx, id, raw, update)
}
