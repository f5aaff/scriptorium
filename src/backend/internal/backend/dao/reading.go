package dao

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"time"
)

// ErrReadingConflict prevents a stale window from replacing a newer saved place.
var ErrReadingConflict = errors.New("reading position changed in another window")

// ReadingPosition identifies a place in a reader, independent of catalogue edits.
// Page is one-based for paged readers. Offset is a within-page scroll fraction,
// or elapsed seconds for audio/video. Locator is reserved for structural anchors.
type ReadingPosition struct {
	Kind    string  `json:"kind"`
	Page    int     `json:"page"`
	Offset  float64 `json:"offset"`
	Scale   float64 `json:"scale"`
	Locator string  `json:"locator,omitempty"`
}

// Validate rejects invalid and unbounded reader coordinates before persistence.
func (p ReadingPosition) Validate() error {
	if math.IsNaN(p.Offset) || math.IsInf(p.Offset, 0) || p.Offset < 0 || len(p.Locator) > 4096 {
		return errors.New("invalid reading position")
	}
	switch p.Kind {
	case "pdf", "text", "epub":
		if p.Page < 1 || p.Page > 10000000 || p.Offset > 1 || math.IsNaN(p.Scale) || math.IsInf(p.Scale, 0) || p.Scale < .25 || p.Scale > 10 {
			return errors.New("invalid paged reading position")
		}
	case "audio", "video":
		if p.Offset > 1e9 || p.Page != 0 || p.Scale != 1 {
			return errors.New("invalid playback position")
		}
	default:
		return errors.New("unknown reader kind")
	}
	return nil
}

// ReadingState includes an optimistic version and an ingestion-identity revision.
// Position is nil when no place exists or the original has been replaced.
type ReadingState struct {
	SourceRevision string           `json:"source_revision"`
	Version        int64            `json:"version"`
	Position       *ReadingPosition `json:"position"`
	UpdatedAt      string           `json:"updated_at,omitempty"`
}

func readingRevision(raw string) (string, error) {
	var metadata MetaData
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return "", err
	}
	if metadata.SHA256 != "" {
		return "sha256:" + metadata.SHA256, nil
	}
	// Legacy originals have no digest. Do not couple saved places to editable title,
	// classification, extraction status or updated_at, or fabricate an input hash.
	identity, err := json.Marshal([]any{metadata.Path, metadata.Size, metadata.OriginalFilename, metadata.FileType})
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(identity)
	return "legacy:" + hex.EncodeToString(hash[:]), nil
}

// ReadReadingState loads only this account's place, including the current source
// identity. Deleted documents never return orphaned positions.
func (s *SQLiteDAO) ReadReadingState(ctx context.Context, account, document string) (ReadingState, error) {
	var raw string
	var position, revision, updated sql.NullString
	var version sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT d.metadata_json, p.position_json, p.source_revision, p.version, p.updated_at
 FROM documents d LEFT JOIN reading_positions p ON p.document_id=d.id AND p.account_id=? WHERE d.id=?`, account, document).Scan(&raw, &position, &revision, &version, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return ReadingState{}, ErrDocumentNotFound
	}
	if err != nil {
		return ReadingState{}, err
	}
	source, err := readingRevision(raw)
	if err != nil {
		return ReadingState{}, err
	}
	state := ReadingState{SourceRevision: source, Version: version.Int64}
	if position.Valid && revision.String == source {
		var value ReadingPosition
		if err := json.Unmarshal([]byte(position.String), &value); err != nil {
			return ReadingState{}, err
		}
		state.Position, state.UpdatedAt = &value, updated.String
	}
	return state, nil
}

// SaveReadingState atomically checks the source and optimistic version before
// saving a personal place. It never writes document metadata or extraction data.
func (s *SQLiteDAO) SaveReadingState(ctx context.Context, account, document, source string, expected int64, position ReadingPosition) (ReadingState, error) {
	if account == "" || expected < 0 {
		return ReadingState{}, errors.New("invalid reading state identity or version")
	}
	if err := position.Validate(); err != nil {
		return ReadingState{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ReadingState{}, err
	}
	defer tx.Rollback()
	var raw string
	err = tx.QueryRowContext(ctx, `SELECT metadata_json FROM documents WHERE id=?`, document).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return ReadingState{}, ErrDocumentNotFound
	}
	if err != nil {
		return ReadingState{}, err
	}
	current, err := readingRevision(raw)
	if err != nil {
		return ReadingState{}, err
	}
	if current != source {
		return ReadingState{}, ErrDocumentChanged
	}
	var version int64
	err = tx.QueryRowContext(ctx, `SELECT version FROM reading_positions WHERE account_id=? AND document_id=?`, account, document).Scan(&version)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ReadingState{}, err
	}
	if expected != version {
		return ReadingState{}, ErrReadingConflict
	}
	encoded, err := json.Marshal(position)
	if err != nil {
		return ReadingState{}, err
	}
	updated := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx, `INSERT INTO reading_positions(account_id, document_id, source_revision, position_json, version, updated_at)
 VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(account_id, document_id) DO UPDATE SET
 source_revision=excluded.source_revision, position_json=excluded.position_json, version=excluded.version, updated_at=excluded.updated_at`, account, document, source, string(encoded), version+1, updated)
	if err != nil {
		return ReadingState{}, err
	}
	if err := tx.Commit(); err != nil {
		return ReadingState{}, err
	}
	return ReadingState{SourceRevision: source, Version: version + 1, Position: &position, UpdatedAt: updated}, nil
}
