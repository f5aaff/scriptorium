package dao

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	bolt "github.com/boltdb/bolt"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

// SQLiteConnectionParams configures the application's primary metadata store.
type SQLiteConnectionParams struct {
	Path string
	Mode fs.FileMode
}

func (p *SQLiteConnectionParams) getParams() any { return *p }

// SQLiteDAO stores catalogue metadata in SQLite and maintains an FTS5 index.
// Original file bytes remain in the storage layer, not in this database.
type SQLiteDAO struct {
	db *sql.DB
}

var _ DAO = (*SQLiteDAO)(nil)
var _ DAO = (*BoltDao)(nil)

func (s *SQLiteDAO) Connect(cp ConnectParams) error {
	if cp == nil {
		return errors.New("SQLite connection parameters are required")
	}
	params, ok := cp.getParams().(SQLiteConnectionParams)
	if !ok {
		return errors.New("connection parameters do not conform to SQLiteConnectionParams")
	}
	if strings.TrimSpace(params.Path) == "" {
		return errors.New("SQLite database path is required")
	}
	abs, err := filepath.Abs(params.Path)
	if err != nil {
		return fmt.Errorf("resolve database path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0700); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}

	dsn := (&url.URL{Scheme: "file", Path: abs}).String() + "?_busy_timeout=5000&_foreign_keys=on&_journal_mode=WAL&_synchronous=FULL"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return fmt.Errorf("open SQLite database: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite has one writer; serialising access avoids lock churn.
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return fmt.Errorf("connect to SQLite database: %w", err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		_ = db.Close()
		return fmt.Errorf("enable SQLite foreign keys: %w", err)
	}
	if err := migrateSQLiteSchema(db); err != nil {
		_ = db.Close()
		return err
	}
	if err := os.Chmod(abs, params.Mode.Perm()); err != nil && !errors.Is(err, os.ErrNotExist) {
		_ = db.Close()
		return fmt.Errorf("set database permissions: %w", err)
	}
	s.db = db
	return nil
}

func migrateSQLiteSchema(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS documents (
    id TEXT PRIMARY KEY NOT NULL,
    metadata_json TEXT NOT NULL,
    search_text TEXT NOT NULL DEFAULT '',
    extracted_text TEXT NOT NULL DEFAULT '',
    sha256 TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_documents_updated_at ON documents(updated_at DESC);
CREATE TABLE IF NOT EXISTS content_hashes (
    sha256 TEXT PRIMARY KEY NOT NULL,
    document_id TEXT NOT NULL UNIQUE REFERENCES documents(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_documents_dewey ON documents(json_extract(metadata_json, '$.DeweyDecimal'));
CREATE VIRTUAL TABLE IF NOT EXISTS documents_fts USING fts5(id UNINDEXED, search_text, tokenize='unicode61 remove_diacritics 2');
CREATE TABLE IF NOT EXISTS ingestion_jobs (
    id TEXT PRIMARY KEY NOT NULL,
    status TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    total INTEGER NOT NULL DEFAULT 0,
    processed INTEGER NOT NULL DEFAULT 0,
    failed INTEGER NOT NULL DEFAULT 0,
    error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    started_at TEXT NOT NULL DEFAULT '',
    finished_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_ingestion_jobs_created ON ingestion_jobs(created_at DESC);
INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (1, CURRENT_TIMESTAMP);
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("apply SQLite schema migration 1: %w", err)
	}
	// Keep additive upgrades safe for databases created by earlier development builds.
	for _, column := range []struct{ name, definition string }{
		{"extracted_text", "TEXT NOT NULL DEFAULT ''"},
		{"sha256", "TEXT NOT NULL DEFAULT ''"},
	} {
		if err := ensureSQLiteColumn(db, "documents", column.name, column.definition); err != nil {
			return err
		}
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_documents_sha256 ON documents(sha256); INSERT OR IGNORE INTO content_hashes(sha256, document_id) SELECT sha256, MIN(id) FROM documents WHERE sha256 <> '' GROUP BY sha256; INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (2, CURRENT_TIMESTAMP); INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (3, CURRENT_TIMESTAMP);`); err != nil {
		return fmt.Errorf("apply SQLite schema migration 2: %w", err)
	}
	if err := ensureSQLiteColumn(db, "ingestion_jobs", "results_json", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (4, CURRENT_TIMESTAMP)`); err != nil {
		return fmt.Errorf("record SQLite schema migration 4: %w", err)
	}
	return nil
}

func ensureSQLiteColumn(db *sql.DB, table, name, definition string) error {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return fmt.Errorf("inspect SQLite schema: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var column, dataType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &column, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return fmt.Errorf("read SQLite schema: %w", err)
		}
		if column == name {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close SQLite schema inspection: %w", err)
	}
	if _, err := db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + name + ` ` + definition); err != nil {
		return fmt.Errorf("add SQLite column %s: %w", name, err)
	}
	return nil
}

func (s *SQLiteDAO) Disconnect() error {
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	if err != nil {
		return fmt.Errorf("close SQLite database: %w", err)
	}
	return nil
}

func (s *SQLiteDAO) ready() error {
	if s == nil || s.db == nil {
		return errors.New("SQLite database is not connected")
	}
	return nil
}

func metadataSearchText(m MetaData) string {
	parts := []string{m.Title, m.Author, m.PublishDate, m.FileType, m.DocType, m.DeweyDecimal,
		m.SuggestedDeweyDecimal, m.SuggestedDeweyName, m.OriginalFilename, m.MediaType, m.MediaKind,
		m.ClassificationStatus, m.ExtractionStatus}
	parts = append(parts, m.Subjects...)
	parts = append(parts, m.ExtractedText)
	for k, v := range m.ExtraMetadata {
		parts = append(parts, k, v)
	}
	return strings.Join(parts, " ")
}

func (s *SQLiteDAO) putMetadata(m MetaData, insertOnly bool) error {
	return s.putMetadataWithDuplicateHashPolicy(m, insertOnly, false)
}

func (s *SQLiteDAO) putMetadataWithDuplicateHashPolicy(m MetaData, insertOnly, allowDuplicateHash bool) error {
	if err := s.ready(); err != nil {
		return err
	}
	if _, err := uuid.Parse(m.Uuid); err != nil {
		return fmt.Errorf("invalid document UUID %q: %w", m.Uuid, err)
	}
	oldHash := ""
	if !insertOnly {
		if err := s.db.QueryRow(`SELECT extracted_text, sha256 FROM documents WHERE id = ?`, m.Uuid).Scan(&m.ExtractedText, &oldHash); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read existing document fields: %w", err)
		}
	}
	encoded, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("encode document metadata: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin document transaction: %w", err)
	}
	defer tx.Rollback()
	query := `INSERT INTO documents(id, metadata_json, search_text, extracted_text, sha256, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?)`
	if !insertOnly {
		query += ` ON CONFLICT(id) DO UPDATE SET metadata_json=excluded.metadata_json, search_text=excluded.search_text, extracted_text=excluded.extracted_text, sha256=excluded.sha256, updated_at=excluded.updated_at`
	}
	if _, err := tx.Exec(query, m.Uuid, string(encoded), metadataSearchText(m), m.ExtractedText, m.SHA256, now, now); err != nil {
		return fmt.Errorf("persist document metadata: %w", err)
	}
	if insertOnly || oldHash != m.SHA256 {
		if !insertOnly {
			if _, err := tx.Exec(`DELETE FROM content_hashes WHERE document_id = ?`, m.Uuid); err != nil {
				return fmt.Errorf("replace document content hash: %w", err)
			}
			if oldHash != "" {
				if err := promoteContentHash(tx, oldHash); err != nil {
					return err
				}
			}
		}
		if m.SHA256 != "" {
			hashQuery := `INSERT INTO content_hashes(sha256, document_id) VALUES(?, ?)`
			if allowDuplicateHash {
				hashQuery = `INSERT OR IGNORE INTO content_hashes(sha256, document_id) VALUES(?, ?)`
			}
			if _, err := tx.Exec(hashQuery, m.SHA256, m.Uuid); err != nil {
				return fmt.Errorf("register document content hash: %w", err)
			}
		}
	}
	if _, err := tx.Exec(`DELETE FROM documents_fts WHERE id = ?`, m.Uuid); err != nil {
		return fmt.Errorf("replace full-text index entry: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO documents_fts(id, search_text) VALUES(?, ?)`, m.Uuid, metadataSearchText(m)); err != nil {
		return fmt.Errorf("index document metadata: %w", err)
	}
	return tx.Commit()
}

func promoteContentHash(tx *sql.Tx, hash string) error {
	if hash == "" {
		return nil
	}
	_, err := tx.Exec(`INSERT OR IGNORE INTO content_hashes(sha256, document_id)
		SELECT sha256, id FROM documents WHERE sha256 = ? ORDER BY id LIMIT 1`, hash)
	if err != nil {
		return fmt.Errorf("promote duplicate content hash: %w", err)
	}
	return nil
}

func (s *SQLiteDAO) Create(doc Document) error {
	if doc == nil {
		return errors.New("document is required")
	}
	return s.putMetadata(doc.GetMetaData(), true)
}

func (s *SQLiteDAO) Update(doc Document) error {
	if doc == nil {
		return errors.New("document is required")
	}
	return s.putMetadata(doc.GetMetaData(), false)
}

func (s *SQLiteDAO) ReadRaw(id uuid.UUID) ([]byte, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	var raw string
	if err := s.db.QueryRow(`SELECT metadata_json FROM documents WHERE id = ?`, id.String()).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("document not found: %s", id)
		}
		return nil, fmt.Errorf("read document metadata: %w", err)
	}
	return []byte(raw), nil
}

func (s *SQLiteDAO) Read(doc *Document, id uuid.UUID) (Document, error) {
	if doc == nil || *doc == nil {
		return nil, errors.New("document target is required")
	}
	if err := s.ready(); err != nil {
		return nil, err
	}
	var raw, extracted string
	if err := s.db.QueryRow(`SELECT metadata_json, extracted_text FROM documents WHERE id = ?`, id.String()).Scan(&raw, &extracted); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("document not found: %s", id)
		}
		return nil, fmt.Errorf("read document metadata: %w", err)
	}
	var m MetaData
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, fmt.Errorf("decode document metadata: %w", err)
	}
	m.ExtractedText = extracted
	if err := (*doc).SetMetaData(m); err != nil {
		return nil, fmt.Errorf("set document metadata: %w", err)
	}
	return *doc, nil
}

func (s *SQLiteDAO) Delete(id uuid.UUID) error {
	if err := s.ready(); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var oldHash string
	if err := tx.QueryRow(`SELECT sha256 FROM documents WHERE id = ?`, id.String()).Scan(&oldHash); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read document content hash: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM content_hashes WHERE document_id = ?`, id.String()); err != nil {
		return fmt.Errorf("remove document content hash: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM documents_fts WHERE id = ?`, id.String()); err != nil {
		return fmt.Errorf("remove full-text index entry: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM documents WHERE id = ?`, id.String()); err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	if oldHash != "" {
		if err := promoteContentHash(tx, oldHash); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteDAO) GetAll() ([]MetaData, error) {
	return s.queryMetadata(`SELECT metadata_json, '' FROM documents ORDER BY json_extract(metadata_json, '$.Title') COLLATE NOCASE, id`)
}

func (s *SQLiteDAO) SearchByKeyValue(key, value string) ([]MetaData, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if key == "" && value == "" {
		return s.GetAll()
	}
	if key == "SHA256" {
		return s.queryMetadata(`SELECT d.metadata_json, '' FROM content_hashes h JOIN documents d ON d.id = h.document_id WHERE h.sha256 = ?`, value)
	}
	all, err := s.GetAll()
	if err != nil {
		return nil, err
	}
	out := make([]MetaData, 0)
	for _, m := range all {
		if metaDataMatches(m, key, value) {
			out = append(out, m)
		}
	}
	return out, nil
}

func (s *SQLiteDAO) FuzzySearch(query string) ([]MetaData, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return s.GetAll()
	}
	// Quote each token so user punctuation cannot become FTS query syntax.
	tokens := strings.Fields(query)
	quoted := make([]string, 0, len(tokens))
	for _, token := range tokens {
		hasTerm := false
		for _, r := range token {
			if unicode.IsLetter(r) || unicode.IsNumber(r) {
				hasTerm = true
				break
			}
		}
		if hasTerm {
			quoted = append(quoted, `"`+strings.ReplaceAll(token, `"`, `""`)+`"`)
		}
	}
	if len(quoted) == 0 {
		return []MetaData{}, nil
	}
	return s.queryMetadata(`SELECT d.metadata_json, '' FROM documents_fts f JOIN documents d ON d.id=f.id WHERE documents_fts MATCH ? ORDER BY rank, d.updated_at DESC`, strings.Join(quoted, " AND "))
}

func (s *SQLiteDAO) queryMetadata(query string, args ...any) ([]MetaData, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query document metadata: %w", err)
	}
	defer rows.Close()
	results := make([]MetaData, 0)
	for rows.Next() {
		var raw, extracted string
		if err := rows.Scan(&raw, &extracted); err != nil {
			return nil, fmt.Errorf("scan document metadata: %w", err)
		}
		var m MetaData
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			return nil, fmt.Errorf("decode document metadata: %w", err)
		}
		m.ExtractedText = extracted
		results = append(results, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate document metadata: %w", err)
	}
	return results, nil
}

// MigrateBoltToSQLite converts an existing BoltDB catalogue in place. The old
// file is retained as <path>.bolt.bak; migration never deletes the only copy.
func MigrateBoltToSQLite(path string, mode fs.FileMode) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect existing database: %w", err)
	}
	if info.Size() >= 16 {
		header := make([]byte, 16)
		f, openErr := os.Open(path)
		if openErr != nil {
			return false, openErr
		}
		_, readErr := f.Read(header)
		_ = f.Close()
		if readErr == nil && string(header) == "SQLite format 3\x00" {
			return false, nil
		}
	}

	legacy, err := bolt.Open(path, mode, &bolt.Options{ReadOnly: true, Timeout: time.Second})
	if err != nil {
		return false, fmt.Errorf("existing database is not a readable SQLite or BoltDB database: %w", err)
	}
	legacyClosed := false
	defer func() {
		if !legacyClosed {
			_ = legacy.Close()
		}
	}()
	metadata := make([]MetaData, 0)
	if err := legacy.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("documents"))
		if bucket == nil {
			return nil
		}
		return bucket.ForEach(func(_, value []byte) error {
			var m MetaData
			if err := json.Unmarshal(value, &m); err != nil {
				return fmt.Errorf("decode legacy document: %w", err)
			}
			metadata = append(metadata, m)
			return nil
		})
	}); err != nil {
		_ = legacy.Close()
		legacyClosed = true
		return false, fmt.Errorf("read legacy BoltDB catalogue: %w", err)
	}

	tempPath := path + ".sqlite-migration"
	_ = os.Remove(tempPath)
	_ = os.Remove(tempPath + "-wal")
	_ = os.Remove(tempPath + "-shm")
	destination := &SQLiteDAO{}
	if err := destination.Connect(&SQLiteConnectionParams{Path: tempPath, Mode: mode}); err != nil {
		return false, fmt.Errorf("create migrated SQLite database: %w", err)
	}
	for _, m := range metadata {
		if err := destination.putMetadataWithDuplicateHashPolicy(m, true, true); err != nil {
			_ = destination.Disconnect()
			_ = os.Remove(tempPath)
			return false, fmt.Errorf("migrate document %s: %w", m.Uuid, err)
		}
	}
	if err := destination.Disconnect(); err != nil {
		_ = os.Remove(tempPath)
		return false, err
	}
	if err := legacy.Close(); err != nil {
		_ = os.Remove(tempPath)
		legacyClosed = true
		return false, fmt.Errorf("close legacy database: %w", err)
	}
	legacyClosed = true

	backup := path + ".bolt.bak"
	if _, err := os.Stat(backup); err == nil {
		_ = os.Remove(tempPath)
		return false, fmt.Errorf("migration backup already exists at %s; refusing to overwrite it", backup)
	}
	if err := os.Rename(path, backup); err != nil {
		_ = os.Remove(tempPath)
		return false, fmt.Errorf("preserve legacy database: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Rename(backup, path)
		_ = os.Remove(tempPath)
		return false, fmt.Errorf("install migrated database (legacy backup restored): %w", err)
	}
	return true, nil
}
