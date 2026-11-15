package auth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestBootstrapPersistsOwnerCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "auth.json")
	manager, err := NewManager(path, "owner", "a-strong-test-password", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected credential file: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("expected 0600 permissions, got %o", info.Mode().Perm())
	}
	if _, err := NewManager(path, "ignored", "ignored-password", false); err != nil {
		t.Fatalf("existing credentials should load without bootstrap values: %v", err)
	}
	if _, _, err := manager.Login("owner", "wrong-password", "127.0.0.1"); err != ErrInvalidCredentials {
		t.Fatalf("expected invalid credentials, got %v", err)
	}
}

func TestLoginSessionAndCSRF(t *testing.T) {
	manager, err := NewManager(filepath.Join(t.TempDir(), "auth.json"), "owner", "a-strong-test-password", false)
	if err != nil {
		t.Fatal(err)
	}
	sessionToken, csrfToken, err := manager.Login("owner", "a-strong-test-password", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/data/create", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: sessionToken})
	request.AddCookie(&http.Cookie{Name: csrfCookie, Value: csrfToken})
	request.Header.Set("X-CSRF-Token", csrfToken)
	if !manager.Authenticated(request) || !manager.ValidCSRF(request) {
		t.Fatal("expected valid authenticated request")
	}
	request.Header.Set("X-CSRF-Token", "invalid")
	if manager.ValidCSRF(request) {
		t.Fatal("invalid CSRF token should be rejected")
	}
	manager.Logout(request)
	if manager.Authenticated(request) {
		t.Fatal("logged out session should be invalid")
	}
}

func TestChangePasswordRevokesSessions(t *testing.T) {
	manager, err := NewManager(filepath.Join(t.TempDir(), "auth.json"), "owner", "a-strong-test-password", false)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := manager.Login("owner", "a-strong-test-password", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ChangePassword("a-strong-test-password", "another-strong-password"); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/auth/me", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	if manager.Authenticated(request) {
		t.Fatal("password change should revoke existing sessions")
	}
	if _, _, err := manager.Login("owner", "another-strong-password", "127.0.0.1"); err != nil {
		t.Fatalf("new password should authenticate: %v", err)
	}
}

func TestFreshInstallRequiresBootstrapCredentials(t *testing.T) {
	_, err := NewManager(filepath.Join(t.TempDir(), "auth.json"), "", "", false)
	if err == nil {
		t.Fatal("fresh installation must not start without an owner account")
	}
}

func TestLoginRateLimit(t *testing.T) {
	manager, err := NewManager(filepath.Join(t.TempDir(), "auth.json"), "owner", "a-strong-test-password", false)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		_, _, err := manager.Login("owner", "wrong-password", "192.0.2.10")
		if err != ErrInvalidCredentials {
			t.Fatalf("attempt %d: expected invalid credentials, got %v", i+1, err)
		}
	}
	if _, _, err := manager.Login("owner", "a-strong-test-password", "192.0.2.10"); err != ErrRateLimited {
		t.Fatalf("expected rate limit after five failures, got %v", err)
	}
}

func TestMultipleUsersPersistPermissionsAndRevokeDisabledSessions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	manager, err := NewManager(path, "owner", "a-strong-test-password", false)
	if err != nil {
		t.Fatal(err)
	}
	permissions := Permissions{Upload: true}
	if err := manager.CreateUser("reader", "another-strong-password", permissions); err != nil {
		t.Fatal(err)
	}

	token, _, err := manager.Login("reader", "another-strong-password", "192.0.2.20")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	user, ok := manager.SessionUser(request)
	if !ok || user.Username != "reader" || user.Admin || !user.Permissions.Upload || user.Permissions.Edit || user.Permissions.Delete {
		t.Fatalf("unexpected user or permissions: %#v, authenticated=%v", user, ok)
	}
	if err := manager.UpdateUser("reader", Permissions{}, true); err != nil {
		t.Fatal(err)
	}
	if manager.Authenticated(request) {
		t.Fatal("disabled user's existing session should be revoked")
	}

	reloaded, err := NewManager(path, "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	users := reloaded.ListUsers()
	found := false
	for _, candidate := range users {
		if candidate.Username == "reader" {
			found = true
			if !candidate.Disabled || candidate.Permissions.Upload {
				t.Fatalf("unexpected persisted account: %#v", candidate)
			}
		}
		if candidate.PasswordHash != "" {
			t.Fatal("password hashes must not be exposed by ListUsers")
		}
	}
	if !found {
		t.Fatal("expected reader account to persist")
	}
}

func TestAdministratorRetainsFullPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	manager, err := NewManager(path, "owner", "a-strong-test-password", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.CreateUser("reader", "another-strong-password", Permissions{}); err != nil {
		t.Fatal(err)
	}
	users := manager.ListUsers()
	if len(users) != 2 {
		t.Fatalf("expected two users, got %d", len(users))
	}
	for _, user := range users {
		if user.Username == "owner" && (!user.Admin || !user.Permissions.Upload || !user.Permissions.Edit || !user.Permissions.Delete) {
			t.Fatalf("owner did not retain administrator access: %#v", user)
		}
	}
}
