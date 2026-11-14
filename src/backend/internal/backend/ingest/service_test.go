package ingest

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"scriptorium/internal/backend/dao"
	"scriptorium/internal/backend/fao"
	"scriptorium/internal/backend/formats"
)

type memoryCatalog struct {
	items []dao.MetaData
	fail  bool
}

func (m *memoryCatalog) Create(doc dao.Document) error {
	if m.fail {
		return errors.New("catalogue unavailable")
	}
	m.items = append(m.items, doc.GetMetaData())
	return nil
}
func (m *memoryCatalog) SearchByKeyValue(key, value string) ([]dao.MetaData, error) {
	var found []dao.MetaData
	for _, item := range m.items {
		if key == "SHA256" && item.SHA256 == value {
			found = append(found, item)
		}
	}
	return found, nil
}

func TestImportPreservesUnknownFormatsAndDeduplicates(t *testing.T) {
	root := t.TempDir()
	storage := fao.NewLocalFao(root)
	catalog := &memoryCatalog{}
	service, err := New(catalog, storage, formats.NewDefaultAdapterRegistry(), 1024)
	if err != nil {
		t.Fatal(err)
	}

	first, err := service.Import(context.Background(), Request{Filename: "archive.custom"}, strings.NewReader("opaque bytes"))
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	if first.Kind != formats.KindOther || first.Metadata.ExtractionStatus != "unsupported" {
		t.Fatalf("unexpected result: %#v", first)
	}
	if !storage.FileExists(first.Metadata.Path) {
		t.Fatal("original file was not retained")
	}

	second, err := service.Import(context.Background(), Request{Filename: "renamed.custom"}, strings.NewReader("opaque bytes"))
	if err != nil {
		t.Fatalf("duplicate import: %v", err)
	}
	if !second.Duplicate || second.Metadata.Uuid != first.Metadata.Uuid {
		t.Fatalf("expected existing catalogue record, got %#v", second)
	}
	if len(catalog.items) != 1 {
		t.Fatalf("expected one catalogue record, got %d", len(catalog.items))
	}
}

func TestImportExtractsAndIndexesText(t *testing.T) {
	storage := fao.NewLocalFao(t.TempDir())
	catalog := &memoryCatalog{}
	service, err := New(catalog, storage, formats.NewDefaultAdapterRegistry(), 1024)
	if err != nil {
		t.Fatal(err)
	}
	content := "A guide to distributed systems and consensus algorithms."
	result, err := service.Import(context.Background(), Request{Filename: "distributed-systems.md"}, strings.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	if result.Metadata.ExtractedText != content {
		t.Fatalf("extracted text = %q", result.Metadata.ExtractedText)
	}
	if result.Metadata.ExtractionStatus != "complete" {
		t.Fatalf("extraction status = %q", result.Metadata.ExtractionStatus)
	}
}

func TestImportEnforcesSizeLimit(t *testing.T) {
	service, err := New(&memoryCatalog{}, fao.NewLocalFao(t.TempDir()), nil, 4)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Import(context.Background(), Request{Filename: "too-large.bin"}, strings.NewReader("12345"))
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
}

func TestImportRollsBackStoredFileWhenCatalogueFails(t *testing.T) {
	root := t.TempDir()
	storage := fao.NewLocalFao(root)
	catalog := &memoryCatalog{fail: true}
	service, err := New(catalog, storage, nil, 1024)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Import(context.Background(), Request{Filename: "plain.txt"}, strings.NewReader("hello"))
	if err == nil {
		t.Fatal("expected catalogue failure")
	}
	entries, readErr := os.ReadDir(root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("expected rollback to remove stored original, found %d files", len(entries))
	}
}
