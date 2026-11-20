package dao

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrJobQueueFull = errors.New("ingestion queue is full")

const MaxActiveIngestionJobs = 10

// ImportFileResult is a durable, public outcome for one file in a batch.
// It contains no staging paths or private ingestion metadata.
type ImportFileResult struct {
	Filename   string `json:"filename"`
	Status     string `json:"status"` // imported, duplicate, or failed
	DocumentID string `json:"document_id,omitempty"`
	Error      string `json:"error,omitempty"`
}

// IngestionJob is durable queue state. Payload is private application data and
// must never be returned directly by HTTP handlers.
type IngestionJob struct {
	ID         string             `json:"id"`
	Status     string             `json:"status"`
	Payload    json.RawMessage    `json:"-"`
	Total      int                `json:"total"`
	Processed  int                `json:"processed"`
	Failed     int                `json:"failed"`
	Error      string             `json:"error,omitempty"`
	CreatedAt  string             `json:"created_at"`
	UpdatedAt  string             `json:"updated_at"`
	StartedAt  string             `json:"started_at,omitempty"`
	FinishedAt string             `json:"finished_at,omitempty"`
	Results    []ImportFileResult `json:"results,omitempty"`
}

func (s *SQLiteDAO) CreateIngestionJob(job IngestionJob) error {
	if err := s.ready(); err != nil {
		return err
	}
	if job.ID == "" || job.Total < 1 || len(job.Payload) == 0 {
		return errors.New("job ID, payload and positive total are required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin ingestion job transaction: %w", err)
	}
	defer tx.Rollback()
	var active int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM ingestion_jobs WHERE status IN ('queued','running')`).Scan(&active); err != nil {
		return fmt.Errorf("count active ingestion jobs: %w", err)
	}
	if active >= MaxActiveIngestionJobs {
		return ErrJobQueueFull
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(`INSERT INTO ingestion_jobs(id,status,payload_json,total,created_at,updated_at) VALUES(?, 'queued', ?, ?, ?, ?)`, job.ID, string(job.Payload), job.Total, now, now); err != nil {
		return fmt.Errorf("create ingestion job: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit ingestion job: %w", err)
	}
	return nil
}

func (s *SQLiteDAO) ActiveIngestionJobCount() (int, error) {
	if err := s.ready(); err != nil {
		return 0, err
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM ingestion_jobs WHERE status IN ('queued','running')`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count active ingestion jobs: %w", err)
	}
	return count, nil
}

func (s *SQLiteDAO) GetIngestionJob(id string) (IngestionJob, error) {
	if err := s.ready(); err != nil {
		return IngestionJob{}, err
	}
	return scanIngestionJob(s.db.QueryRow(`SELECT id,status,payload_json,total,processed,failed,error,created_at,updated_at,started_at,finished_at,results_json FROM ingestion_jobs WHERE id=?`, id))
}

// ListIngestionJobs returns recent job summaries without potentially large file results.
// GetIngestionJob returns the full results for an individual job.
func (s *SQLiteDAO) ListIngestionJobs(limit int) ([]IngestionJob, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		limit = 25
	}
	rows, err := s.db.Query(`SELECT id,status,payload_json,total,processed,failed,error,created_at,updated_at,started_at,finished_at,'[]' FROM ingestion_jobs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list ingestion jobs: %w", err)
	}
	defer rows.Close()
	jobs := make([]IngestionJob, 0)
	for rows.Next() {
		job, err := scanIngestionJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *SQLiteDAO) ClaimIngestionJob() (IngestionJob, bool, error) {
	if err := s.ready(); err != nil {
		return IngestionJob{}, false, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return IngestionJob{}, false, err
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRow(`SELECT id FROM ingestion_jobs WHERE status='queued' ORDER BY created_at LIMIT 1`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return IngestionJob{}, false, nil
	}
	if err != nil {
		return IngestionJob{}, false, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(`UPDATE ingestion_jobs SET status='running', started_at=?, updated_at=? WHERE id=? AND status='queued'`, now, now, id); err != nil {
		return IngestionJob{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return IngestionJob{}, false, err
	}
	job, err := s.GetIngestionJob(id)
	return job, err == nil, err
}

// UpdateIngestionJob atomically saves progress and the corresponding file outcomes.
func (s *SQLiteDAO) UpdateIngestionJob(job IngestionJob) error {
	if err := s.ready(); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	results, err := json.Marshal(job.Results)
	if err != nil {
		return fmt.Errorf("encode ingestion results: %w", err)
	}
	result, err := s.db.Exec(`UPDATE ingestion_jobs SET status=?,processed=?,failed=?,error=?,updated_at=?,finished_at=?,results_json=? WHERE id=?`, job.Status, job.Processed, job.Failed, job.Error, now, job.FinishedAt, string(results), job.ID)
	if err != nil {
		return fmt.Errorf("update ingestion job: %w", err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// RecoverIngestionJobs requeues interrupted work while retaining committed outcomes.
// Uncommitted imports can be replayed safely because content hashes prevent duplicates.
func (s *SQLiteDAO) RecoverIngestionJobs() error {
	if err := s.ready(); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.Exec(`UPDATE ingestion_jobs SET status='queued', error='recovered after an interrupted server run', updated_at=? WHERE status='running'`, now)
	return err
}

type rowScanner interface{ Scan(...any) error }

func scanIngestionJob(row rowScanner) (IngestionJob, error) {
	var job IngestionJob
	var payload, results string
	err := row.Scan(&job.ID, &job.Status, &payload, &job.Total, &job.Processed, &job.Failed, &job.Error, &job.CreatedAt, &job.UpdatedAt, &job.StartedAt, &job.FinishedAt, &results)
	if errors.Is(err, sql.ErrNoRows) {
		return IngestionJob{}, fmt.Errorf("ingestion job not found")
	}
	if err != nil {
		return IngestionJob{}, fmt.Errorf("read ingestion job: %w", err)
	}
	if err := json.Unmarshal([]byte(results), &job.Results); err != nil {
		return IngestionJob{}, fmt.Errorf("decode ingestion results: %w", err)
	}
	job.Payload = json.RawMessage(payload)
	return job, nil
}
