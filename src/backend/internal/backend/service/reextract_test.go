package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"scriptorium/internal/backend/auth"
	"scriptorium/internal/backend/dao"
	"scriptorium/internal/backend/fao"
	"scriptorium/internal/backend/formats"
	"scriptorium/internal/backend/ingest"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func reextractHandlerFixture(t *testing.T) (*FileHandler, *dao.SQLiteDAO, *fao.LocalFao, dao.MetaData) {
	t.Helper()
	db := &dao.SQLiteDAO{}
	if err := db.Connect(&dao.SQLiteConnectionParams{Path: filepath.Join(t.TempDir(), "library.sqlite"), Mode: 0600}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Disconnect() })
	storage := fao.NewLocalFao(t.TempDir())
	const source = "programming apireindexphrase"
	if err := storage.SaveFile("original-key", strings.NewReader(source)); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(source))
	metadata := dao.MetaData{Uuid: uuid.NewString(), Path: "original-key", SHA256: hex.EncodeToString(hash[:]), OriginalFilename: "notes.txt", FileType: ".txt", Title: "User title", DocType: "File", ExtractionStatus: "failed", ExtractedText: "oldapiphrase"}
	if err := db.Create(&dao.Notes{Metadata: metadata}); err != nil {
		t.Fatal(err)
	}
	factory := dao.NewDocumentFactory()
	factory.RegisterDocumentType("File", func() dao.Document { return &dao.Notes{} })
	api := NewAPIHandler(DaoService{dao: db}, factory, storage)
	return NewFileHandler(storage, api, nil), db, storage, metadata
}

func TestReextractEndpointRefreshesOnlyDerivedFields(t *testing.T) {
	handler, db, _, metadata := reextractHandlerFixture(t)
	router := gin.New()
	router.POST("/file/reextract/:uuid", handler.ReextractText)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/file/reextract/"+metadata.Uuid, strings.NewReader(`{"Title":"injected","Path":"/private/server/path"}`))
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("re-extraction failed: %d %s", response.Code, response.Body)
	}
	var result struct {
		Metadata dao.MetaData `json:"metadata"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Metadata.Title != metadata.Title || result.Metadata.Path != metadata.Path || result.Metadata.ExtractionStatus != "complete" || strings.Contains(response.Body.String(), "apireindexphrase") {
		t.Fatalf("unexpected response or leaked extracted text: %s", response.Body)
	}
	matches, err := db.FuzzySearch("apireindexphrase")
	if err != nil || len(matches) != 1 {
		t.Fatalf("replacement text not searchable: %#v, %v", matches, err)
	}
}

func TestReextractEndpointRejectsInvalidMissingAndUnavailableOriginal(t *testing.T) {
	handler, _, storage, metadata := reextractHandlerFixture(t)
	router := gin.New()
	router.POST("/file/reextract/:uuid", handler.ReextractText)
	for _, test := range []struct {
		id     string
		status int
	}{
		{"invalid", http.StatusBadRequest}, {uuid.NewString(), http.StatusNotFound},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/file/reextract/"+test.id, nil))
		if response.Code != test.status {
			t.Fatalf("invalid/missing request: %d %s", response.Code, response.Body)
		}
	}
	if err := storage.DeleteFile(metadata.Path); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/file/reextract/"+metadata.Uuid, nil))
	if response.Code != http.StatusConflict || strings.Contains(response.Body.String(), "/tmp/") {
		t.Fatalf("missing original error exposed private data: %d %s", response.Code, response.Body)
	}
}

type reextractErrorRepository struct{ err error }

func (r reextractErrorRepository) ReadRaw(uuid.UUID) ([]byte, error) { return nil, r.err }
func (reextractErrorRepository) ReplaceExtraction(context.Context, uuid.UUID, []byte, dao.ExtractionUpdate) (dao.MetaData, error) {
	return dao.MetaData{}, errors.New("unexpected extraction update")
}

func TestReextractEndpointMapsPublicErrors(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		status int
	}{
		{"changed metadata", dao.ErrDocumentChanged, http.StatusConflict},
		{"busy", ingest.ErrReprocessBusy, http.StatusConflict},
		{"capacity", ingest.ErrReprocessCapacity, http.StatusTooManyRequests},
		{"integrity failure", ingest.ErrOriginalChanged, http.StatusConflict},
		{"missing extractor", formats.ErrExtractorUnavailable, http.StatusServiceUnavailable},
		{"unsupported", ingest.ErrExtractionUnsupported, http.StatusUnprocessableEntity},
		{"parser failure", ingest.ErrReprocessFailed, http.StatusUnprocessableEntity},
		{"timeout", context.DeadlineExceeded, http.StatusGatewayTimeout},
		{"cancelled", context.Canceled, http.StatusRequestTimeout},
		{"private error", errors.New("database /private/server/path"), http.StatusInternalServerError},
	} {
		t.Run(test.name, func(t *testing.T) {
			processor, err := ingest.NewReprocessor(reextractErrorRepository{test.err}, fao.NewLocalFao(t.TempDir()), nil)
			if err != nil {
				t.Fatal(err)
			}
			handler := FileHandler{Reprocessor: processor}
			router := gin.New()
			router.POST("/reextract/:uuid", handler.ReextractText)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/reextract/"+uuid.NewString(), nil))
			if test.status == http.StatusTooManyRequests && response.Header().Get("Retry-After") != "2" {
				t.Fatal("busy response is missing Retry-After")
			}
			if response.Code != test.status || strings.Contains(response.Body.String(), "/private/") {
				t.Fatalf("unexpected response: %d %s", response.Code, response.Body)
			}
		})
	}
}

func TestReextractEndpointRequiresEditPermission(t *testing.T) {
	handler, _, _, metadata := reextractHandlerFixture(t)
	manager, err := auth.NewManager(filepath.Join(t.TempDir(), "auth.json"), "owner", "a-secure-test-password", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, user := range []struct {
		name        string
		permissions auth.Permissions
	}{
		{"reader", auth.Permissions{}}, {"uploader", auth.Permissions{Upload: true}}, {"editor", auth.Permissions{Edit: true}},
	} {
		if err := manager.CreateUser(user.name, "a-secure-test-password", user.permissions); err != nil {
			t.Fatal(err)
		}
	}
	server, err := NewHTTPServer("127.0.0.1", 8080, t.TempDir(), manager, nil, nil, handler.APIHandler, handler)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		user   string
		status int
	}{
		{"", http.StatusUnauthorized}, {"reader", http.StatusForbidden}, {"uploader", http.StatusForbidden}, {"editor", http.StatusOK}, {"owner", http.StatusOK},
	} {
		t.Run(test.user, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/file/reextract/"+metadata.Uuid, nil)
			if test.user != "" {
				token, _, err := manager.Login(test.user, "a-secure-test-password", test.user)
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("Authorization", "Bearer "+token)
			}
			response := httptest.NewRecorder()
			server.Handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("%s: %d %s", test.user, response.Code, response.Body)
			}
		})
	}
	login := httptest.NewRecorder()
	server.Handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"username":"editor","password":"a-secure-test-password"}`)))
	if login.Code != http.StatusOK {
		t.Fatalf("cookie login failed: %s", login.Body)
	}
	request := httptest.NewRequest(http.MethodPost, "/file/reextract/"+metadata.Uuid, nil)
	for _, cookie := range login.Result().Cookies() {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("cookie mutation without CSRF was accepted: %d", response.Code)
	}
}
