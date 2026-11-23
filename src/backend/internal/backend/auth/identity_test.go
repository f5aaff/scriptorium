package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAccountIdentitySurvivesMigrationAndPasswordChangesButNotRecreation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	manager, err := NewManager(path, "owner", "a-secure-test-password", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.CreateUser("reader", "a-secure-test-password", Permissions{}); err != nil {
		t.Fatal(err)
	}
	// Simulate a pre-identity multi-account credential file without rehashing.
	for name, user := range manager.users {
		user.ID = ""
		manager.users[name] = user
	}
	if err := manager.persistLocked(); err != nil {
		t.Fatal(err)
	}
	migrated, err := NewManager(path, "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	identity := migrated.users["reader"].ID
	if identity == "" || identity == migrated.users["owner"].ID {
		t.Fatal("missing or shared identities")
	}
	reloaded, err := NewManager(path, "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.users["reader"].ID != identity {
		t.Fatal("account identity was not persisted")
	}
	if err := reloaded.ChangePassword("reader", "a-secure-test-password", "another-secure-password"); err != nil {
		t.Fatal(err)
	}
	if reloaded.users["reader"].ID != identity {
		t.Fatal("password change lost reading identity")
	}
	if err := reloaded.DeleteUser("reader"); err != nil {
		t.Fatal(err)
	}
	if err := reloaded.CreateUser("reader", "a-secure-test-password", Permissions{}); err != nil {
		t.Fatal(err)
	}
	if reloaded.users["reader"].ID == identity {
		t.Fatal("recreated account inherited the previous identity")
	}
}

func TestLegacyOwnerReceivesPersistentAccountIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	stored, _ := json.Marshal(credentialFile{Username: "owner", PasswordHash: "preserved-legacy-hash"})
	if err := os.WriteFile(path, stored, 0600); err != nil {
		t.Fatal(err)
	}
	first, err := NewManager(path, "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewManager(path, "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if first.users["owner"].ID == "" || first.users["owner"].ID != second.users["owner"].ID || second.users["owner"].PasswordHash != "preserved-legacy-hash" {
		t.Fatal("legacy account identity migration lost credentials or identity")
	}
}
