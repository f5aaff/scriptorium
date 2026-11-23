package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"scriptorium/internal/backend/auth"
	"scriptorium/internal/backend/dao"
)

func TestReadingStateIsPrivateAndWritableByReadOnlyAccounts(t *testing.T) {
	handler, metadata := previewFixture(t, "document")
	manager, err := auth.NewManager(filepath.Join(t.TempDir(), "auth.json"), "owner", "a-secure-test-password", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.CreateUser("reader", "a-secure-test-password", auth.Permissions{}); err != nil {
		t.Fatal(err)
	}
	token, csrf, err := manager.Login("reader", "a-secure-test-password", "reader")
	if err != nil {
		t.Fatal(err)
	}
	ownerToken, _, err := manager.Login("owner", "a-secure-test-password", "owner")
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewHTTPServer("127.0.0.1", 8080, t.TempDir(), manager, nil, nil, handler)
	if err != nil {
		t.Fatal(err)
	}
	url := "/file/reading-state/" + metadata.Uuid
	request := func(method, body, bearer string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, url, strings.NewReader(body))
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		response := httptest.NewRecorder()
		server.Handler.ServeHTTP(response, req)
		return response
	}
	if response := request("GET", "", ""); response.Code != 401 {
		t.Fatal("anonymous state access allowed")
	}
	initial := request("GET", "", token)
	var state dao.ReadingState
	if initial.Code != 200 || json.Unmarshal(initial.Body.Bytes(), &state) != nil {
		t.Fatalf("load: %d %s", initial.Code, initial.Body)
	}
	body, _ := json.Marshal(map[string]any{"source_revision": state.SourceRevision, "expected_version": 0, "position": dao.ReadingPosition{Kind: "pdf", Page: 7, Scale: 1.25, Offset: .5}})
	// Cookie sessions must retain CSRF enforcement, even though personal state
	// requires no catalogue-edit permission. Bearer sessions are handled separately.
	for _, validCSRF := range []bool{false, true} {
		req := httptest.NewRequest("PUT", url, strings.NewReader(string(body)))
		req.AddCookie(&http.Cookie{Name: "scriptorium_session", Value: token})
		if validCSRF {
			req.AddCookie(&http.Cookie{Name: "scriptorium_csrf", Value: csrf})
			req.Header.Set("X-CSRF-Token", csrf)
		}
		response := httptest.NewRecorder()
		server.Handler.ServeHTTP(response, req)
		expected := 403
		if validCSRF {
			expected = 200
		}
		if response.Code != expected {
			t.Fatalf("cookie state save: %d %s", response.Code, response.Body)
		}
	}
	saved := request("GET", "", token)
	if err := json.Unmarshal(saved.Body.Bytes(), &state); err != nil || state.Position == nil || state.Position.Page != 7 || state.Version != 1 {
		t.Fatalf("place not saved: %s", saved.Body)
	}
	if response := request("PUT", string(body), token); response.Code != 409 {
		t.Fatalf("stale position accepted: %d", response.Code)
	}
	other := request("GET", "", ownerToken)
	var private dao.ReadingState
	if err := json.Unmarshal(other.Body.Bytes(), &private); err != nil || private.Position != nil || private.Version != 0 {
		t.Fatalf("private place leaked: %s", other.Body)
	}
	forged := strings.TrimSuffix(string(body), "}") + `,"account_id":"owner"}`
	if response := request("PUT", forged, token); response.Code != 400 {
		t.Fatalf("client-supplied account accepted: %d", response.Code)
	}
	if response := request("PUT", string(body)+"{}", token); response.Code != 400 {
		t.Fatal("trailing JSON accepted")
	}
	if response := request("PUT", strings.Repeat(" ", 8193)+string(body), token); response.Code != 400 {
		t.Fatal("oversized state accepted")
	}
}
