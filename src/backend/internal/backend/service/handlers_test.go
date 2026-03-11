package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"scriptorium/internal/backend/dao"
	"scriptorium/internal/backend/fao"

	"github.com/gin-gonic/gin"
)

func setupTestRouter(t *testing.T) (*gin.Engine, *APIHandler, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	storagePath := filepath.Join(tmpDir, "storage")
	os.MkdirAll(storagePath, 0755)

	conparams := &dao.BoltConnectionParams{Path: dbPath, Mode: 0600, Opts: nil}
	d := &dao.BoltDao{}
	if err := d.Connect(conparams); err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}

	f := fao.NewLocalFao(storagePath)

	docFactory := dao.NewDocumentFactory()
	docFactory.RegisterDocumentType("Notes", func() dao.Document { return &dao.Notes{} })
	docFactory.RegisterDocumentType("Book", func() dao.Document { return &dao.Notes{} })

	daos := DaoService{dao: d}
	handler := NewAPIHandler(daos, docFactory, f)

	r := gin.New()
	_, routes := handler.GetRouterGroups()
	group := r.Group("/data")
	for route, fn := range routes {
		parts := strings.Split(route, " ")
		if len(parts) != 2 {
			continue
		}
		method, endpoint := parts[0], parts[1]
		switch method {
		case "GET":
			group.GET(endpoint, fn)
		case "POST":
			group.POST(endpoint, fn)
		case "PUT":
			group.PUT(endpoint, fn)
		case "DELETE":
			group.DELETE(endpoint, fn)
		}
	}

	cleanup := func() {
		d.Disconnect()
	}

	return r, handler, cleanup
}

func TestCreateDocument(t *testing.T) {
	r, _, cleanup := setupTestRouter(t)
	defer cleanup()

	body := map[string]any{
		"DocType": "Notes",
		"Title":   "Test Document",
		"Content": "Test content",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/data/create", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	uuid, ok := resp["UUID"].(string)
	if !ok || uuid == "" {
		t.Fatalf("expected UUID in response, got: %v", resp)
	}
}

func TestCreateAndReadDocument(t *testing.T) {
	r, _, cleanup := setupTestRouter(t)
	defer cleanup()

	body := map[string]any{
		"DocType": "Notes",
		"Title":   "Readable Doc",
		"Content": "Some content",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/data/create", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("create failed: %d: %s", w.Code, w.Body.String())
	}

	var createResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &createResp)
	uuid := createResp["UUID"].(string)

	req = httptest.NewRequest(http.MethodGet, "/data/read/"+uuid, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("read failed: %d: %s", w.Code, w.Body.String())
	}
}

func TestSearchDocuments(t *testing.T) {
	r, _, cleanup := setupTestRouter(t)
	defer cleanup()

	body := map[string]any{
		"DocType": "Notes",
		"Title":   "Searchable Doc",
		"Content": "Some content",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/data/create", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("create failed: %d: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/data/search?key=Title&value=Searchable+Doc", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("search failed: %d: %s", w.Code, w.Body.String())
	}

	var searchResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &searchResp)
	count := searchResp["total_count"].(float64)
	if count < 1 {
		t.Fatalf("expected at least 1 result, got %v", count)
	}
}

func TestSearchAllDocuments(t *testing.T) {
	r, _, cleanup := setupTestRouter(t)
	defer cleanup()

	for i := 0; i < 3; i++ {
		body := map[string]any{
			"DocType": "Notes",
			"Title":   "Doc",
			"Content": "Content",
		}
		bodyBytes, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/data/create", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}

	req := httptest.NewRequest(http.MethodGet, "/data/search", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("search all failed: %d: %s", w.Code, w.Body.String())
	}

	var searchResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &searchResp)
	count := searchResp["total_count"].(float64)
	if count != 3 {
		t.Fatalf("expected 3 results, got %v", count)
	}
}

func TestDeleteDocument(t *testing.T) {
	r, _, cleanup := setupTestRouter(t)
	defer cleanup()

	body := map[string]any{
		"DocType": "Notes",
		"Title":   "To Delete",
		"Content": "Will be deleted",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/data/create", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var createResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &createResp)
	uuid := createResp["UUID"].(string)

	deleteBody := map[string]any{"uuids": []string{uuid}}
	deleteBytes, _ := json.Marshal(deleteBody)

	req = httptest.NewRequest(http.MethodDelete, "/data/delete", bytes.NewReader(deleteBytes))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("delete failed: %d: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/data/read/"+uuid, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Fatal("expected document to be deleted, but read succeeded")
	}
}

func TestUpdateDocument(t *testing.T) {
	r, _, cleanup := setupTestRouter(t)
	defer cleanup()

	body := map[string]any{
		"DocType": "Notes",
		"Title":   "Original Title",
		"Content": "Original content",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/data/create", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var createResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &createResp)
	uuid := createResp["UUID"].(string)

	updateBody := map[string]any{
		"DocType": "Notes",
		"Uuid":    uuid,
		"Title":   "Updated Title",
		"Content": "Updated content",
	}
	updateBytes, _ := json.Marshal(updateBody)

	req = httptest.NewRequest(http.MethodPut, "/data/update", bytes.NewReader(updateBytes))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("update failed: %d: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/data/read/"+uuid, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("read after update failed: %d: %s", w.Code, w.Body.String())
	}

	var readResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &readResp)
	valueStr := readResp["value"].(string)

	var docData map[string]any
	json.Unmarshal([]byte(valueStr), &docData)
	if docData["Title"] != "Updated Title" {
		t.Fatalf("expected 'Updated Title', got '%v'", docData["Title"])
	}
}

func TestCreateWithInvalidDocType(t *testing.T) {
	r, _, cleanup := setupTestRouter(t)
	defer cleanup()

	body := map[string]any{
		"DocType": "InvalidType",
		"Title":   "Test",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/data/create", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid doc type, got %d", w.Code)
	}
}
