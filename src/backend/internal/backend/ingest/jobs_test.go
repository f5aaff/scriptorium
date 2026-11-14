package ingest

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"scriptorium/internal/backend/dao"
	"scriptorium/internal/backend/formats"
)

type memoryJobRepository struct{ jobs map[string]dao.IngestionJob }

func (r *memoryJobRepository) ActiveIngestionJobCount() (int, error) {
	count := 0
	for _, job := range r.jobs {
		if job.Status == "queued" || job.Status == "running" {
			count++
		}
	}
	return count, nil
}
func (r *memoryJobRepository) CreateIngestionJob(job dao.IngestionJob) error {
	if r.jobs == nil {
		r.jobs = map[string]dao.IngestionJob{}
	}
	job.Status = "queued"
	r.jobs[job.ID] = job
	return nil
}
func (r *memoryJobRepository) GetIngestionJob(id string) (dao.IngestionJob, error) {
	job, ok := r.jobs[id]
	if !ok {
		return dao.IngestionJob{}, os.ErrNotExist
	}
	return job, nil
}
func (r *memoryJobRepository) ListIngestionJobs(int) ([]dao.IngestionJob, error) {
	out := make([]dao.IngestionJob, 0, len(r.jobs))
	for _, job := range r.jobs {
		out = append(out, job)
	}
	return out, nil
}
func (r *memoryJobRepository) ClaimIngestionJob() (dao.IngestionJob, bool, error) {
	return dao.IngestionJob{}, false, nil
}
func (r *memoryJobRepository) UpdateIngestionJob(dao.IngestionJob) error { return nil }
func (r *memoryJobRepository) RecoverIngestionJobs() error               { return nil }

type unusedCatalog struct{}

func (unusedCatalog) Create(dao.Document) error                               { return nil }
func (unusedCatalog) SearchByKeyValue(string, string) ([]dao.MetaData, error) { return nil, nil }

type unusedStorage struct{}

func (unusedStorage) SaveFile(string, io.Reader) error { return nil }
func (unusedStorage) DeleteFile(string) error          { return nil }

func newTestJobManager(t *testing.T, roots []string) (*JobManager, *memoryJobRepository) {
	t.Helper()
	catalog := unusedCatalog{}
	storage := unusedStorage{}
	ingestor, err := New(catalog, storage, formats.NewDefaultAdapterRegistry(), DefaultMaxFileSize)
	if err != nil {
		t.Fatal(err)
	}
	repo := &memoryJobRepository{}
	manager, err := NewJobManager(repo, ingestor, filepath.Join(t.TempDir(), "spool"), roots, 1)
	if err != nil {
		t.Fatal(err)
	}
	return manager, repo
}
func TestJobManagerQueuesDurableUploadFiles(t *testing.T) {
	manager, repo := newTestJobManager(t, nil)
	job, err := manager.EnqueueUploads(context.Background(), []Upload{{Filename: "hello.txt", Reader: strings.NewReader("hello")}})
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "queued" || job.Total != 1 {
		t.Fatalf("unexpected job: %#v", job)
	}
	var items []jobItem
	if err := json.Unmarshal(job.Payload, &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one staged item, got %d", len(items))
	}
	contents, err := os.ReadFile(items[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "hello" {
		t.Fatalf("staged content = %q", contents)
	}
	if len(repo.jobs) != 1 {
		t.Fatalf("job not persisted: %#v", repo.jobs)
	}
}
func TestJobManagerRestrictsDirectoryImportsToConfiguredRoots(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "book.txt"), []byte("book"), 0600); err != nil {
		t.Fatal(err)
	}
	manager, _ := newTestJobManager(t, []string{root})
	roots := manager.Roots()
	if len(roots) != 1 {
		t.Fatalf("roots = %#v", roots)
	}
	if _, err := manager.EnqueueDirectory(context.Background(), roots[0].ID, "../outside"); err == nil {
		t.Fatal("expected traversal path to be rejected")
	}
	job, err := manager.EnqueueDirectory(context.Background(), roots[0].ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if job.Total != 1 {
		t.Fatalf("expected one imported file, got %d", job.Total)
	}
}
