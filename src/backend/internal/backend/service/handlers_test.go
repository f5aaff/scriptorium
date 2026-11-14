package service

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"scriptorium/internal/backend/auth"
	"scriptorium/internal/backend/dao"
	"scriptorium/internal/backend/fao"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
	docFactory.RegisterDocumentType("File", func() dao.Document { return &dao.Notes{} })
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

func TestUploadCataloguesUnknownFileFormats(t *testing.T) {
	apiRouter, api, cleanup := setupTestRouter(t)
	defer cleanup()

	storage := api.FaoService
	fileHandler := NewFileHandler(storage, api, nil)
	router := gin.New()
	_, routes := fileHandler.GetRouterGroups()
	group := router.Group("/file")
	for route, handler := range routes {
		parts := strings.Split(route, " ")
		if len(parts) == 2 && parts[0] == http.MethodPost {
			group.POST(parts[1], handler)
		}
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "research.customfmt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("proprietary document payload")); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("metadata", `{"Title":"Research notes"}`); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/file/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", response.Code, response.Body.String())
	}

	var result struct {
		UUID string `json:"document_uuid"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.UUID == "" {
		t.Fatal("upload response did not include document UUID")
	}

	id, err := uuid.Parse(result.UUID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := api.DaoService.ReadRaw(id)
	if err != nil {
		t.Fatal(err)
	}
	var metadata dao.MetaData
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.OriginalFilename != "research.customfmt" {
		t.Fatalf("original filename not preserved: %+v", metadata)
	}
	if metadata.SHA256 == "" || metadata.Size == 0 {
		t.Fatalf("integrity metadata missing: %+v", metadata)
	}
	if metadata.Path == "" || !storage.FileExists(metadata.Path) {
		t.Fatalf("stored file missing: %+v", metadata)
	}

	updateBody := []byte(`{"Uuid":"` + result.UUID + `","DocType":"Notes","Title":"Updated research notes","DeweyDecimal":"004"}`)
	updateRequest := httptest.NewRequest(http.MethodPut, "/data/update", bytes.NewReader(updateBody))
	updateRequest.Header.Set("Content-Type", "application/json")
	updateResponse := httptest.NewRecorder()
	apiRouter.ServeHTTP(updateResponse, updateRequest)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("metadata update failed: %d %s", updateResponse.Code, updateResponse.Body.String())
	}

	updatedRaw, err := api.DaoService.ReadRaw(id)
	if err != nil {
		t.Fatal(err)
	}
	var updated dao.MetaData
	if err := json.Unmarshal(updatedRaw, &updated); err != nil {
		t.Fatal(err)
	}
	if updated.SHA256 != metadata.SHA256 || updated.Path != metadata.Path || updated.Size != metadata.Size {
		t.Fatalf("metadata update discarded ingestion fields: before=%+v after=%+v", metadata, updated)
	}
	if updated.ClassificationStatus != "confirmed" || updated.DeweyDecimal != "004" {
		t.Fatalf("manual classification was not confirmed: %+v", updated)
	}
}

func TestSuggestClassificationEndpoint(t *testing.T) {
	router, _, cleanup := setupTestRouter(t)
	defer cleanup()

	request := httptest.NewRequest(http.MethodPost, "/data/classify", strings.NewReader(`{"title":"Go programming and software architecture"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"code":"004"`) {
		t.Fatalf("expected computer science suggestion, got %s", response.Body.String())
	}
}

func TestHTTPServerServesHealthAndBrowserFrontend(t *testing.T) {
	_, api, cleanup := setupTestRouter(t)
	defer cleanup()

	webRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(webRoot, "index.html"), []byte("scriptorium-ui"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(webRoot, "assets"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webRoot, "assets", "app.js"), []byte("console.log('ok')"), 0600); err != nil {
		t.Fatal(err)
	}

	fileHandler := NewFileHandler(api.FaoService, api, nil)
	authManager, err := auth.NewManager(filepath.Join(t.TempDir(), "auth.json"), "owner", "a-secure-test-password", false)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewHTTPServer("127.0.0.1", 8080, webRoot, authManager, nil, nil, api, fileHandler)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct{ path, want string }{
		{path: "/healthz", want: `"status":"ok"`},
		{path: "/library/entry/123", want: "scriptorium-ui"},
		{path: "/assets/app.js", want: "console.log('ok')"},
	} {
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		response := httptest.NewRecorder()
		server.Handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), test.want) {
			t.Errorf("GET %s: got status %d body %q", test.path, response.Code, response.Body.String())
		}
	}
}

func TestHTTPServerProtectsLibraryRoutes(t *testing.T) {
	_, api, cleanup := setupTestRouter(t)
	defer cleanup()
	manager, err := auth.NewManager(filepath.Join(t.TempDir(), "auth.json"), "owner", "a-secure-test-password", false)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewHTTPServer("127.0.0.1", 8080, t.TempDir(), manager, nil, nil, api)
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/data/search", nil)
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated library request should be rejected, got %d", response.Code)
	}

	loginBody := bytes.NewBufferString(`{"username":"owner","password":"a-secure-test-password"}`)
	request = httptest.NewRequest(http.MethodPost, "/auth/login", loginBody)
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("login should succeed, got %d: %s", response.Code, response.Body.String())
	}
	var loginResult struct {
		CSRFToken    string `json:"csrf_token"`
		SessionToken string `json:"session_token"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &loginResult); err != nil || loginResult.CSRFToken == "" || loginResult.SessionToken == "" {
		t.Fatalf("login response should contain CSRF token: %v", err)
	}
	cookies := response.Result().Cookies()
	request = httptest.NewRequest(http.MethodGet, "/data/search", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response = httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("authenticated cookie request should succeed, got %d: %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/data/create", bytes.NewBufferString(`{"DocType":"Notes","Title":"Missing CSRF"}`))
	request.Header.Set("Content-Type", "application/json")
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response = httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("cookie-authenticated mutation without CSRF should be rejected, got %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/data/create", bytes.NewBufferString(`{"DocType":"Notes","Title":"Bearer client"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+loginResult.SessionToken)
	response = httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("authenticated bearer mutation should succeed, got %d: %s", response.Code, response.Body.String())
	}
}
