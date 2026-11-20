package ingest

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"scriptorium/internal/backend/dao"
)

func TestJobManagerPersistsFileOutcomes(t *testing.T) {
	manager, repo := newTestJobManager(t, nil)
	manager.ingestor.catalog = &memoryCatalog{}
	job, err := manager.EnqueueUploads(context.Background(), []Upload{
		{Filename: "first.txt", Reader: strings.NewReader("same text")},
		{Filename: "duplicate.txt", Reader: strings.NewReader("same text")},
		{Filename: "missing.txt", Reader: strings.NewReader("missing text")},
	})
	if err != nil {
		t.Fatal(err)
	}
	var items []jobItem
	if err := json.Unmarshal(job.Payload, &items); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(items[2].Path); err != nil {
		t.Fatal(err)
	}
	manager.process(context.Background(), job)
	finished := repo.jobs[job.ID]
	if finished.Status != "completed_with_errors" || finished.Processed != 3 || finished.Failed != 1 || len(finished.Results) != 3 {
		t.Fatalf("unexpected completed job: %#v", finished)
	}
	for i, status := range []string{"imported", "duplicate", "failed"} {
		if finished.Results[i].Status != status || finished.Results[i].Filename != items[i].Filename {
			t.Fatalf("result %d = %#v", i, finished.Results[i])
		}
	}
	if finished.Results[0].DocumentID == "" || finished.Results[0].DocumentID != finished.Results[1].DocumentID {
		t.Fatalf("missing canonical document links: %#v", finished.Results)
	}
	public, err := json.Marshal(finished)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(public), manager.spool) || strings.Contains(string(public), items[2].Path) {
		t.Fatalf("private path exposed in job: %s", public)
	}
	if finished.Results[2].DocumentID != "" || finished.Results[2].Error == "" {
		t.Fatalf("unexpected failure result: %#v", finished.Results[2])
	}
}

func TestJobManagerResumesAfterCommittedOutcomes(t *testing.T) {
	manager, repo := newTestJobManager(t, nil)
	manager.ingestor.catalog = &memoryCatalog{}
	job, err := manager.EnqueueUploads(context.Background(), []Upload{
		{Filename: "first.txt", Reader: strings.NewReader("first text")},
		{Filename: "second.txt", Reader: strings.NewReader("second text")},
	})
	if err != nil {
		t.Fatal(err)
	}
	var items []jobItem
	if err := json.Unmarshal(job.Payload, &items); err != nil {
		t.Fatal(err)
	}
	// Persist just the first result to simulate interruption between files.
	file, err := os.Open(items[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	first, err := manager.ingestor.Import(context.Background(), Request{Filename: items[0].Filename}, file)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	job.Results = append(job.Results, dao.ImportFileResult{Filename: items[0].Filename, Status: "imported", DocumentID: first.Metadata.Uuid})
	job.Processed = 1
	if err := os.Remove(items[0].Path); err != nil {
		t.Fatal(err)
	}
	manager.process(context.Background(), job)
	finished := repo.jobs[job.ID]
	if finished.Status != "completed" || finished.Processed != 2 || finished.Failed != 0 || len(finished.Results) != 2 {
		t.Fatalf("unexpected resumed job: %#v", finished)
	}
	if finished.Results[0].Status != "imported" || finished.Results[0].DocumentID != first.Metadata.Uuid {
		t.Fatalf("committed result changed: %#v", finished.Results[0])
	}
}
