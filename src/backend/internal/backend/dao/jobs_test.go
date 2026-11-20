package dao

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func TestSQLiteIngestionJobLifecycleAndRecovery(t *testing.T) {
	db := newTestSQLite(t)
	payload, _ := json.Marshal([]string{"staged-file"})
	job := IngestionJob{ID: "job-1", Payload: payload, Total: 3}
	if err := db.CreateIngestionJob(job); err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := db.ClaimIngestionJob()
	if err != nil || !ok || claimed.Status != "running" || claimed.Total != 3 {
		t.Fatalf("claim = %#v, %v, %v", claimed, ok, err)
	}
	claimed.Processed, claimed.Failed = 2, 1
	claimed.Results = []ImportFileResult{
		{Filename: "book.txt", Status: "imported", DocumentID: "document-1"},
		{Filename: "broken.txt", Status: "failed", Error: "Import failed"},
	}
	if err := db.UpdateIngestionJob(claimed); err != nil {
		t.Fatal(err)
	}
	if err := db.RecoverIngestionJobs(); err != nil {
		t.Fatal(err)
	}
	recovered, err := db.GetIngestionJob("job-1")
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != "queued" || recovered.Processed != 2 || recovered.Failed != 1 {
		t.Fatalf("unexpected recovered state: %#v", recovered)
	}
	if len(recovered.Results) != 2 || recovered.Results[0].DocumentID != "document-1" || recovered.Results[1].Status != "failed" {
		t.Fatalf("results lost during recovery: %#v", recovered.Results)
	}
	if string(recovered.Payload) != string(payload) {
		t.Fatalf("payload changed during recovery: %s", recovered.Payload)
	}
	listed, err := db.ListIngestionJobs(10)
	if err != nil || len(listed) != 1 || listed[0].ID != "job-1" || len(listed[0].Results) != 0 {
		t.Fatalf("list = %#v, err=%v", listed, err)
	}
}

func TestSQLiteBoundsActiveIngestionJobs(t *testing.T) {
	db := newTestSQLite(t)
	payload := json.RawMessage(`[]`)
	for i := 0; i < MaxActiveIngestionJobs; i++ {
		job := IngestionJob{ID: fmt.Sprintf("job-%02d", i), Payload: payload, Total: 1}
		if err := db.CreateIngestionJob(job); err != nil {
			t.Fatalf("create job %d: %v", i, err)
		}
	}
	err := db.CreateIngestionJob(IngestionJob{ID: "overflow", Payload: payload, Total: 1})
	if !errors.Is(err, ErrJobQueueFull) {
		t.Fatalf("expected queue limit error, got %v", err)
	}
}
