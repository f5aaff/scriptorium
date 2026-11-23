package dao

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// ErrDocumentNotFound indicates that a catalogue record no longer exists.
var ErrDocumentNotFound = errors.New("document not found")

// ErrDocumentChanged indicates that metadata changed while extraction was running.
var ErrDocumentChanged = errors.New("document changed during extraction; retry with the current record")

// ExtractionUpdate contains only derived fields that reprocessing may replace.
// Editable metadata and ingestion identity are deliberately not part of this type.
type ExtractionUpdate struct {
	Text       string
	Status     string
	Dewey      string
	DeweyName  string
	Confidence float64
	Evidence   []string
}

// ReplaceExtraction atomically replaces successful extraction fields and their FTS
// index entry. expected must be the exact ReadRaw snapshot used for extraction;
// concurrent edits or deletion cause an error, never an upsert or lost update.
// Confirmed classifications and all user-edited metadata remain unchanged.
func (s *SQLiteDAO) ReplaceExtraction(ctx context.Context, id uuid.UUID, expected []byte, update ExtractionUpdate) (MetaData, error) {
	if err := s.ready(); err != nil {
		return MetaData{}, err
	}
	if update.Status != "complete" && update.Status != "truncated" && update.Status != "no_text" {
		return MetaData{}, errors.New("only successful extraction outcomes may replace indexed text")
	}
	if len(update.Text) > 1<<20 || !utf8.ValidString(update.Text) {
		return MetaData{}, errors.New("extracted text must be bounded UTF-8")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MetaData{}, err
	}
	defer tx.Rollback()
	var raw string
	if err := tx.QueryRowContext(ctx, `SELECT metadata_json FROM documents WHERE id=?`, id.String()).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return MetaData{}, ErrDocumentNotFound
		}
		return MetaData{}, fmt.Errorf("read extraction snapshot: %w", err)
	}
	if raw != string(expected) {
		return MetaData{}, ErrDocumentChanged
	}
	var metadata MetaData
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return MetaData{}, fmt.Errorf("decode extraction snapshot: %w", err)
	}
	metadata.ExtractedText = update.Text
	metadata.ExtractionStatus = update.Status
	metadata.LastUpdated = time.Now().UTC().Format(time.RFC3339Nano)
	if metadata.DeweyDecimal == "" && metadata.ClassificationStatus != "confirmed" {
		metadata.SuggestedDeweyDecimal = update.Dewey
		metadata.SuggestedDeweyName = update.DeweyName
		metadata.ClassificationConfidence = update.Confidence
		metadata.ClassificationEvidence = update.Evidence
		metadata.ClassificationStatus = "unclassified"
		if update.Dewey != "" {
			metadata.ClassificationStatus = "suggested"
		}
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return MetaData{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `UPDATE documents SET metadata_json=?,extracted_text=?,search_text=?,updated_at=? WHERE id=?`, string(encoded), update.Text, metadataSearchText(metadata), now, id.String()); err != nil {
		return MetaData{}, fmt.Errorf("persist extraction: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM documents_fts WHERE id=?`, id.String()); err != nil {
		return MetaData{}, fmt.Errorf("remove previous extraction index: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO documents_fts(id,search_text) VALUES(?,?)`, id.String(), metadataSearchText(metadata)); err != nil {
		return MetaData{}, fmt.Errorf("index replacement extraction: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return MetaData{}, err
	}
	return metadata, nil
}

// preserveExtractionFields keeps stale metadata editors from reverting a newer
// extraction. SQLiteDAO.Update calls this inside its write transaction.
func preserveExtractionFields(metadata *MetaData, current MetaData) {
	metadata.ExtractionStatus = current.ExtractionStatus
	metadata.SuggestedDeweyDecimal = current.SuggestedDeweyDecimal
	metadata.SuggestedDeweyName = current.SuggestedDeweyName
	metadata.ClassificationConfidence = current.ClassificationConfidence
	metadata.ClassificationEvidence = current.ClassificationEvidence
	if metadata.DeweyDecimal != "" {
		metadata.ClassificationStatus = "confirmed"
	} else if metadata.SuggestedDeweyDecimal != "" {
		metadata.ClassificationStatus = "suggested"
	} else {
		metadata.ClassificationStatus = "unclassified"
	}
}
