package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"scriptorium/internal/backend/dao"
	"scriptorium/internal/backend/fao"
	"scriptorium/internal/backend/ingest"

	"github.com/gin-gonic/gin"
)

func TestImportJobEndpointsExposeResultsOnlyInDetails(t *testing.T) {
	db := &dao.SQLiteDAO{}
	if err := db.Connect(&dao.SQLiteConnectionParams{Path: filepath.Join(t.TempDir(), "library.sqlite"), Mode: 0600}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Disconnect() })
	storage := fao.NewLocalFao(t.TempDir())
	ingestor, err := ingest.New(db, storage, nil, ingest.DefaultMaxFileSize)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := ingest.NewJobManager(db, ingestor, filepath.Join(t.TempDir(), "spool"), nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	job := dao.IngestionJob{ID: "job-1", Total: 1, Payload: json.RawMessage(`[{"path":"/private/staging/file"}]`)}
	if err := db.CreateIngestionJob(job); err != nil {
		t.Fatal(err)
	}
	job.Status, job.Processed = "completed", 1
	job.Results = []dao.ImportFileResult{{Filename: "book.txt", Status: "imported", DocumentID: "document-1"}}
	if err := db.UpdateIngestionJob(job); err != nil {
		t.Fatal(err)
	}
	handler := FileHandler{Jobs: manager}
	router := gin.New()
	router.GET("/jobs", handler.ListImportJobs)
	router.GET("/jobs/:id", handler.GetImportJob)
	for _, path := range []string{"/jobs", "/jobs/job-1"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s: %d %s", path, response.Code, response.Body)
		}
		body := response.Body.String()
		if strings.Contains(body, "/private/") || strings.Contains(body, "payload") {
			t.Fatalf("private ingestion data exposed: %s", body)
		}
		if strings.Contains(body, "document-1") != (path == "/jobs/job-1") {
			t.Fatalf("unexpected results in %s: %s", path, body)
		}
	}
}
