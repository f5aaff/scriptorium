package fao

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalFAORoundTrip(t *testing.T) {
	store := NewLocalFao(t.TempDir())
	if err := store.SaveFile("objects/a.bin", bytes.NewReader([]byte("payload"))); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetFile("objects/a.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer got.Close()
	data, err := io.ReadAll(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "payload" {
		t.Fatalf("got %q", data)
	}
	if !store.FileExists("objects/a.bin") {
		t.Fatal("expected stored file to exist")
	}
}

func TestLocalFAORejectsTraversalAndAbsolutePaths(t *testing.T) {
	root := t.TempDir()
	store := NewLocalFao(root)
	for _, name := range []string{"../outside", "../../etc/passwd", filepath.Join(root, "outside"), ""} {
		if err := store.SaveFile(name, strings.NewReader("no")); err == nil {
			t.Errorf("expected %q to be rejected", name)
		}
	}
}

func TestLocalFAORejectsSymlinkEscape(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	store := NewLocalFao(root)
	if err := store.SaveFile("escape/pwned", strings.NewReader("no")); err == nil {
		t.Fatal("expected symlink traversal to be rejected")
	}
	if _, err := os.Stat(filepath.Join(outside, "pwned")); !os.IsNotExist(err) {
		t.Fatal("file escaped storage root")
	}
}
