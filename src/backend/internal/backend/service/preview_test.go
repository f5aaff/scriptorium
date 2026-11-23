package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"scriptorium/internal/backend/auth"
	"scriptorium/internal/backend/dao"
	"scriptorium/internal/backend/fao"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func previewFixture(t *testing.T, source string) (*FileHandler, dao.MetaData) {
	t.Helper()
	handler, db, storage, metadata := reextractHandlerFixture(t)
	if err := storage.DeleteFile(metadata.Path); err != nil {
		t.Fatal(err)
	}
	if err := storage.SaveFile(metadata.Path, strings.NewReader(source)); err != nil {
		t.Fatal(err)
	}
	// Deliberately misleading metadata must not select an active response type.
	metadata.MediaType, metadata.OriginalFilename = "application/pdf", "misleading.pdf"
	if err := db.Update(&dao.Notes{Metadata: metadata}); err != nil {
		t.Fatal(err)
	}
	return handler, metadata
}

func previewRouter(handler *FileHandler) *gin.Engine {
	router := gin.New()
	router.GET("/preview/:uuid", handler.PreviewFile)
	router.HEAD("/preview/:uuid", handler.PreviewFile)
	return router
}

func TestPreviewFileUsesContentSignaturesAndInertText(t *testing.T) {
	for _, test := range []struct {
		name, source, kind, mediaType string
		status                        int
	}{
		{"PDF", "%PDF-1.7\nfixture", "pdf", "application/pdf", 200},
		{"PNG", "\x89PNG\r\n\x1a\nfixture", "image", "image/png", 200},
		{"audio", "RIFF\x00\x00\x00\x00WAVEfixture", "audio", "audio/wave", 200},
		{"video", "\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom", "video", "video/mp4", 200},
		{"plain text", "searchable text", "text", "text/plain; charset=utf-8", 200},
		{"HTML source", "<!DOCTYPE html><script>alert(1)</script>", "text", "text/plain; charset=utf-8", 200},
		{"SVG source", "<svg xmlns=\"http://www.w3.org/2000/svg\"><script>alert(1)</script></svg>", "text", "text/plain; charset=utf-8", 200},
		{"archive", "PK\x03\x04\x00\x00\x00\x00", "", "", 415},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler, metadata := previewFixture(t, test.source)
			response := httptest.NewRecorder()
			previewRouter(handler).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/preview/"+metadata.Uuid, nil))
			if response.Code != test.status {
				t.Fatalf("preview: %d %s", response.Code, response.Body)
			}
			if test.status != 200 {
				return
			}
			if response.Header().Get("Content-Type") != test.mediaType || response.Header().Get("X-Scriptorium-Preview-Kind") != test.kind || response.Body.String() != test.source {
				t.Fatalf("untrusted metadata affected preview: %v %q", response.Header(), response.Body.String())
			}
			if response.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(response.Header().Get("Content-Security-Policy"), "sandbox") || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("missing preview isolation headers: %v", response.Header())
			}
		})
	}
}

func TestPreviewFileSupportsHeadAndByteRanges(t *testing.T) {
	const source = "%PDF-1.7\nrange-fixture"
	handler, metadata := previewFixture(t, source)
	router := previewRouter(handler)
	for _, test := range []struct {
		method, byteRange, body string
		status                  int
	}{
		{"GET", "bytes=0-4", source[:5], 206},
		{"GET", "bytes=-4", source[len(source)-4:], 206},
		{"HEAD", "", "", 200},
		{"GET", "bytes=999-1000", "", 416},
	} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(test.method, "/preview/"+metadata.Uuid, nil)
		if test.byteRange != "" {
			request.Header.Set("Range", test.byteRange)
		}
		router.ServeHTTP(response, request)
		if response.Code != test.status || (test.status != 416 && response.Body.String() != test.body) {
			t.Fatalf("%s %s: %d %q", test.method, test.byteRange, response.Code, response.Body.String())
		}
		if test.status == 206 && response.Header().Get("Content-Range") == "" {
			t.Fatal("partial response lacks Content-Range")
		}
		if test.method == "HEAD" && response.Header().Get("Content-Length") != fmt.Sprint(len(source)) {
			t.Fatal("HEAD lacks original size")
		}
	}
}

type previewStorage struct {
	fao.FAO
	file io.ReadCloser
}

func (s previewStorage) GetFile(string) (io.ReadCloser, error) { return s.file, nil }

type largePreviewSource struct {
	*bytes.Reader
	closed bool
}

func (r *largePreviewSource) Seek(offset int64, whence int) (int64, error) {
	if whence == io.SeekEnd {
		return maxFileSize + 1, nil
	}
	return r.Reader.Seek(offset, whence)
}
func (r *largePreviewSource) Close() error { r.closed = true; return nil }

func TestPreviewFileRejectsOversizedAndNonSeekableOriginals(t *testing.T) {
	handler, metadata := previewFixture(t, "text")
	original := &largePreviewSource{Reader: bytes.NewReader(nil)}
	handler.FaoService = previewStorage{FAO: handler.FaoService, file: original}
	response := httptest.NewRecorder()
	previewRouter(handler).ServeHTTP(response, httptest.NewRequest("GET", "/preview/"+metadata.Uuid, nil))
	if response.Code != 413 || !original.closed {
		t.Fatalf("oversized source accepted or leaked: %d", response.Code)
	}
	handler.FaoService = previewStorage{FAO: handler.FaoService, file: io.NopCloser(strings.NewReader("text"))}
	response = httptest.NewRecorder()
	previewRouter(handler).ServeHTTP(response, httptest.NewRequest("GET", "/preview/"+metadata.Uuid, nil))
	if response.Code != 501 {
		t.Fatalf("non-seekable source accepted: %d", response.Code)
	}
}

func TestPreviewFileRejectsMissingAndInvalidRecords(t *testing.T) {
	handler, _ := previewFixture(t, "text")
	for _, test := range []struct {
		id     string
		status int
	}{{"invalid", 400}, {uuid.NewString(), 404}} {
		response := httptest.NewRecorder()
		previewRouter(handler).ServeHTTP(response, httptest.NewRequest("GET", "/preview/"+test.id, nil))
		if response.Code != test.status {
			t.Fatalf("unexpected response: %d %s", response.Code, response.Body)
		}
	}
}

func TestViewerSecurityPolicyAllowsLocalRenderingWithoutJavaScriptEval(t *testing.T) {
	router := gin.New()
	router.Use(securityHeaders(nil))
	router.GET("/", func(c *gin.Context) { c.String(200, "frontend") })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/", nil))
	policy := response.Header().Get("Content-Security-Policy")
	for _, directive := range []string{"worker-src 'self' blob:", "font-src 'self' data: blob:", "'wasm-unsafe-eval'", "object-src 'none'"} {
		if !strings.Contains(policy, directive) {
			t.Fatalf("viewer resources blocked by policy: %s", policy)
		}
	}
	if strings.Contains(policy, "'unsafe-eval'") {
		t.Fatalf("JavaScript eval was enabled: %s", policy)
	}
}

func TestPreviewFileRequiresSessionAndExposesRangeHeadersToAllowedClients(t *testing.T) {
	handler, metadata := previewFixture(t, "%PDF-1.7\nfixture")
	manager, err := auth.NewManager(filepath.Join(t.TempDir(), "auth.json"), "owner", "a-secure-test-password", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.CreateUser("reader", "a-secure-test-password", auth.Permissions{}); err != nil {
		t.Fatal(err)
	}
	server, err := NewHTTPServer("127.0.0.1", 8080, t.TempDir(), manager, []string{"http://localhost:5173"}, nil, handler)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"GET", "HEAD"} {
		response := httptest.NewRecorder()
		server.Handler.ServeHTTP(response, httptest.NewRequest(method, "/file/preview/"+metadata.Uuid, nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated preview allowed: %d", response.Code)
		}
	}
	token, _, err := manager.Login("reader", "a-secure-test-password", "reader")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/file/preview/"+metadata.Uuid, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Origin", "http://localhost:5173")
	request.Header.Set("Range", "bytes=0-4")
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != 206 || !strings.Contains(response.Header().Get("Access-Control-Expose-Headers"), "X-Scriptorium-Preview-Kind") {
		t.Fatalf("read-only cross-origin range preview failed: %d %v", response.Code, response.Header())
	}
	request = httptest.NewRequest("OPTIONS", "/file/preview/"+metadata.Uuid, nil)
	request.Header.Set("Origin", "http://localhost:5173")
	request.Header.Set("Access-Control-Request-Method", "HEAD")
	request.Header.Set("Access-Control-Request-Headers", "authorization, range")
	response = httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != 204 {
		t.Fatalf("preview preflight failed: %d", response.Code)
	}
}
