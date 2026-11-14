// Package ingest coordinates safe file import, format identification, metadata
// extraction, duplicate detection, classification and catalogue persistence.
package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"scriptorium/internal/backend/classification"
	"scriptorium/internal/backend/dao"
	"scriptorium/internal/backend/formats"

	"github.com/google/uuid"
)

const DefaultMaxFileSize int64 = 100 * 1024 * 1024

var ErrTooLarge = errors.New("uploaded file exceeds the configured size limit")

type Catalog interface {
	Create(dao.Document) error
	SearchByKeyValue(key, value string) ([]dao.MetaData, error)
}

type Storage interface {
	SaveFile(string, io.Reader) error
	DeleteFile(string) error
}

type Service struct {
	catalog  Catalog
	storage  Storage
	adapters *formats.AdapterRegistry
	maxBytes int64
}

type Request struct {
	Filename string
	Metadata map[string]any
}

type Result struct {
	Metadata  dao.MetaData `json:"metadata"`
	Kind      formats.Kind `json:"kind"`
	Duplicate bool         `json:"duplicate"`
}

func New(catalog Catalog, storage Storage, adapters *formats.AdapterRegistry, maxBytes int64) (*Service, error) {
	if catalog == nil || storage == nil {
		return nil, errors.New("catalogue and storage are required")
	}
	if adapters == nil {
		adapters = formats.NewDefaultAdapterRegistry()
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxFileSize
	}
	return &Service{catalog: catalog, storage: storage, adapters: adapters, maxBytes: maxBytes}, nil
}

// Import stages the incoming stream to a private temporary file so the hash,
// content sniffing and metadata extraction all operate on the same bytes. The
// original is only committed to managed storage after duplicate detection.
func (s *Service) Import(ctx context.Context, request Request, source io.Reader) (Result, error) {
	if source == nil {
		return Result{}, errors.New("source file is required")
	}
	filename := filepath.Base(strings.TrimSpace(request.Filename))
	if filename == "." || filename == string(filepath.Separator) || filename == "" {
		return Result{}, errors.New("a valid filename is required")
	}

	staged, err := os.CreateTemp("", "scriptorium-ingest-*")
	if err != nil {
		return Result{}, fmt.Errorf("create import staging file: %w", err)
	}
	stagedPath := staged.Name()
	defer os.Remove(stagedPath)
	defer staged.Close()

	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(staged, hasher), io.LimitReader(contextReader{ctx: ctx, reader: source}, s.maxBytes+1))
	if err != nil {
		return Result{}, fmt.Errorf("stage uploaded file: %w", err)
	}
	if written == 0 {
		return Result{}, errors.New("empty files are not accepted")
	}
	if written > s.maxBytes {
		return Result{}, fmt.Errorf("%w (%d bytes)", ErrTooLarge, s.maxBytes)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if err := staged.Sync(); err != nil {
		return Result{}, fmt.Errorf("sync staged upload: %w", err)
	}
	hash := hex.EncodeToString(hasher.Sum(nil))

	duplicates, err := s.catalog.SearchByKeyValue("SHA256", hash)
	if err != nil {
		return Result{}, fmt.Errorf("check duplicate content: %w", err)
	}
	if len(duplicates) > 0 {
		return Result{Metadata: duplicates[0], Kind: formats.Kind(duplicates[0].MediaKind), Duplicate: true}, nil
	}

	if _, err := staged.Seek(0, io.SeekStart); err != nil {
		return Result{}, err
	}
	identification, err := formats.Identify(filename, staged)
	if err != nil {
		return Result{}, fmt.Errorf("identify file format: %w", err)
	}
	if _, err := staged.Seek(0, io.SeekStart); err != nil {
		return Result{}, err
	}
	extracted, err := s.adapters.Extract(ctx, filename, staged)
	extractionStatus := "unsupported"
	if s.adapters.Supports(filename) {
		extractionStatus = "complete"
	}
	if ctx.Err() != nil {
		return Result{}, ctx.Err()
	}
	if err != nil {
		extractionStatus = "failed"
		// Extraction is an enhancement, not a condition of acceptance. Keep the
		// original and record that extraction failed so it can be retried later.
		extracted = formats.ExtractedMetadata{}
	} else if extracted.Truncated {
		extractionStatus = "truncated"
	}

	id := uuid.New()
	storagePath := id.String()
	if _, err := staged.Seek(0, io.SeekStart); err != nil {
		return Result{}, err
	}
	if err := s.storage.SaveFile(storagePath, io.LimitReader(staged, s.maxBytes)); err != nil {
		return Result{}, fmt.Errorf("store original file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		if cleanupErr := s.storage.DeleteFile(storagePath); cleanupErr != nil {
			return Result{}, fmt.Errorf("cancelled import and failed to remove stored original: %w", cleanupErr)
		}
		return Result{}, err
	}

	docType := stringValue(request.Metadata["DocType"], "File")
	meta := dao.MetaData{
		Uuid: id.String(), Title: strings.TrimSuffix(filename, filepath.Ext(filename)),
		DocType: docType, FileType: identification.Extension, Path: storagePath,
		OriginalFilename: filename, MediaType: identification.MediaType, MediaKind: string(identification.Kind), Size: written,
		SHA256: hash, Subjects: extracted.Subjects, ExtractedText: extracted.Text,
		ClassificationStatus: "unclassified", ExtractionStatus: extractionStatus, LastUpdated: time.Now().UTC().Format(time.RFC3339),
	}
	meta.Title = stringValue(request.Metadata["Title"], stringValue(extracted.Title, meta.Title))
	meta.Author = stringValue(request.Metadata["Author"], extracted.Author)
	meta.PublishDate = stringValue(request.Metadata["PublishDate"], extracted.PublishedDate)
	if dewey := stringValue(request.Metadata["DeweyDecimal"], ""); dewey != "" {
		meta.DeweyDecimal, meta.ClassificationStatus = dewey, "confirmed"
	}
	if meta.DeweyDecimal == "" {
		suggestions := classification.Suggest(meta.Title+" "+meta.Author+" "+meta.ExtractedText, 1)
		if len(suggestions) > 0 {
			meta.SuggestedDeweyDecimal = suggestions[0].Code
			meta.SuggestedDeweyName = suggestions[0].Name
			meta.ClassificationConfidence = suggestions[0].Confidence
			meta.ClassificationEvidence = suggestions[0].Evidence
			meta.ClassificationStatus = "suggested"
		}
	}

	if err := s.catalog.Create(&dao.Notes{Title: meta.Title, Metadata: meta}); err != nil {
		// A concurrent import may have committed the same hash after our initial
		// duplicate check. The database's unique hash index is the final arbiter.
		if duplicates, lookupErr := s.catalog.SearchByKeyValue("SHA256", hash); lookupErr == nil && len(duplicates) > 0 {
			if cleanupErr := s.storage.DeleteFile(storagePath); cleanupErr != nil {
				return Result{}, fmt.Errorf("duplicate content found but temporary original cleanup failed: %w", cleanupErr)
			}
			return Result{Metadata: duplicates[0], Kind: formats.Kind(duplicates[0].MediaKind), Duplicate: true}, nil
		}
		if cleanupErr := s.storage.DeleteFile(storagePath); cleanupErr != nil {
			return Result{}, fmt.Errorf("catalogue import failed: %w (also failed to remove stored original: %v)", err, cleanupErr)
		}
		return Result{}, fmt.Errorf("catalogue import failed: %w", err)
	}
	return Result{Metadata: meta, Kind: identification.Kind}, nil
}

func stringValue(value any, fallback string) string {
	if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
		return strings.TrimSpace(text)
	}
	return fallback
}
