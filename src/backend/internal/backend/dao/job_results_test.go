package dao

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestSQLiteImportResultsSurviveReopen(t *testing.T) {
	params := &SQLiteConnectionParams{Path: filepath.Join(t.TempDir(), "library.sqlite"), Mode: 0600}
	db := &SQLiteDAO{}
	if err := db.Connect(params); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Disconnect() })
	job := IngestionJob{ID: "durable", Total: 1, Payload: json.RawMessage(`["private-source"]`)}
	if err := db.CreateIngestionJob(job); err != nil {
		t.Fatal(err)
	}
	job.Status, job.Processed = "completed", 1
	job.Results = []ImportFileResult{{Filename: "book.txt", Status: "imported", DocumentID: "document-1"}}
	if err := db.UpdateIngestionJob(job); err != nil {
		t.Fatal(err)
	}
	if err := db.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if err := db.Connect(params); err != nil {
		t.Fatal(err)
	}
	saved, err := db.GetIngestionJob(job.ID)
	if err != nil || len(saved.Results) != 1 || saved.Results[0] != job.Results[0] {
		t.Fatalf("results after reopen: %#v, %v", saved, err)
	}
	public, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(public, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["Payload"]; ok {
		t.Fatal("private payload exposed")
	}
}

func TestSQLiteAddsResultsToExistingJobs(t *testing.T) {
	db := newTestSQLite(t)
	if err := db.CreateIngestionJob(IngestionJob{ID: "legacy", Total: 1, Payload: json.RawMessage(`[]`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`ALTER TABLE ingestion_jobs DROP COLUMN results_json; DELETE FROM schema_migrations WHERE version=4`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := migrateSQLiteSchema(db.db); err != nil {
			t.Fatal(err)
		}
	}
	job, err := db.GetIngestionJob("legacy")
	if err != nil || job.Total != 1 || len(job.Results) != 0 {
		t.Fatalf("legacy job after upgrade: %#v, %v", job, err)
	}
}
