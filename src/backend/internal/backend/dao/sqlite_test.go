package dao

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func newTestSQLite(t *testing.T) *SQLiteDAO {
	t.Helper()
	db := &SQLiteDAO{}
	if err := db.Connect(&SQLiteConnectionParams{Path: filepath.Join(t.TempDir(), "library.sqlite"), Mode: 0600}); err != nil {
		t.Fatalf("connect SQLite: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Disconnect(); err != nil {
			t.Errorf("disconnect SQLite: %v", err)
		}
	})
	return db
}

func TestSQLiteCRUDAndFullTextSearch(t *testing.T) {
	db := newTestSQLite(t)
	id := uuid.New()
	doc := &Notes{Metadata: MetaData{Uuid: id.String(), Title: "The Go Programming Language", Author: "Alan A. A. Donovan", Subjects: []string{"Programming", "Compilers"}, DeweyDecimal: "005.13"}}
	if err := db.Create(doc); err != nil {
		t.Fatalf("create: %v", err)
	}
	results, err := db.FuzzySearch("compilers")
	if err != nil {
		t.Fatalf("full text search: %v", err)
	}
	if len(results) != 1 || results[0].Uuid != id.String() {
		t.Fatalf("unexpected search results: %#v", results)
	}

	doc.Metadata.Title = "Updated Go Reference"
	if err := db.Update(doc); err != nil {
		t.Fatalf("update: %v", err)
	}
	all, err := db.GetAll()
	if err != nil || len(all) != 1 || all[0].Title != "Updated Go Reference" {
		t.Fatalf("get all = %#v, err = %v", all, err)
	}
	if err := db.Delete(id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	all, err = db.GetAll()
	if err != nil || len(all) != 0 {
		t.Fatalf("expected empty catalogue, got %#v, err=%v", all, err)
	}
}

func TestSQLiteRejectsDuplicateID(t *testing.T) {
	db := newTestSQLite(t)
	id := uuid.New().String()
	doc := &Notes{Metadata: MetaData{Uuid: id, Title: "First"}}
	if err := db.Create(doc); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(doc); err == nil {
		t.Fatal("expected duplicate primary key error")
	}
}

func TestMigrateBoltToSQLitePreservesBackupAndRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.db")
	legacy := &BoltDao{}
	if err := legacy.Connect(&BoltConnectionParams{Path: path, Mode: 0600}); err != nil {
		t.Fatal(err)
	}
	id := uuid.New().String()
	if err := legacy.Create(&Notes{Metadata: MetaData{Uuid: id, Title: "Legacy item"}}); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Disconnect(); err != nil {
		t.Fatal(err)
	}

	migrated, err := MigrateBoltToSQLite(path, 0600)
	if err != nil {
		t.Fatalf("migration: %v", err)
	}
	if !migrated {
		t.Fatal("expected migration to occur")
	}
	if _, err := os.Stat(path + ".bolt.bak"); err != nil {
		t.Fatalf("legacy backup missing: %v", err)
	}

	db := &SQLiteDAO{}
	if err := db.Connect(&SQLiteConnectionParams{Path: path, Mode: 0600}); err != nil {
		t.Fatal(err)
	}
	defer db.Disconnect()
	all, err := db.GetAll()
	if err != nil || len(all) != 1 || all[0].Uuid != id {
		t.Fatalf("migrated data = %#v, err=%v", all, err)
	}
	migrated, err = MigrateBoltToSQLite(path, 0600)
	if err != nil || migrated {
		t.Fatalf("second migration should be a no-op: migrated=%v err=%v", migrated, err)
	}
}

func TestSQLiteIndexesExtractedTextAndPreservesItOnMetadataUpdate(t *testing.T) {
	db := newTestSQLite(t)
	id := uuid.New().String()
	doc := &Notes{Metadata: MetaData{Uuid: id, Title: "A document", ExtractedText: "rarephrase searchable body content"}}
	if err := db.Create(doc); err != nil {
		t.Fatal(err)
	}
	matches, err := db.FuzzySearch("rarephrase")
	if err != nil || len(matches) != 1 {
		t.Fatalf("search extracted text: matches=%d err=%v", len(matches), err)
	}

	updated := &Notes{Metadata: MetaData{Uuid: id, Title: "Renamed document"}}
	if err := db.Update(updated); err != nil {
		t.Fatal(err)
	}
	matches, err = db.FuzzySearch("rarephrase")
	if err != nil || len(matches) != 1 {
		t.Fatalf("extracted text lost after metadata update: matches=%d err=%v", len(matches), err)
	}
}

func TestSQLiteEnforcesContentHashUniqueness(t *testing.T) {
	db := newTestSQLite(t)
	first := &Notes{Metadata: MetaData{Uuid: uuid.New().String(), Title: "First", SHA256: "same-content"}}
	second := &Notes{Metadata: MetaData{Uuid: uuid.New().String(), Title: "Second", SHA256: "same-content"}}
	if err := db.Create(first); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(second); err == nil {
		t.Fatal("expected duplicate content hash to be rejected")
	}
	all, err := db.GetAll()
	if err != nil || len(all) != 1 {
		t.Fatalf("expected one record after duplicate rollback, got %d err=%v", len(all), err)
	}
}

func TestBoltMigrationPreservesDuplicateRecordsAndPromotesHash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy := &BoltDao{}
	if err := legacy.Connect(&BoltConnectionParams{Path: path, Mode: 0600}); err != nil {
		t.Fatal(err)
	}
	first := &Notes{Metadata: MetaData{Uuid: uuid.New().String(), Title: "First copy", SHA256: "same-content"}}
	second := &Notes{Metadata: MetaData{Uuid: uuid.New().String(), Title: "Second copy", SHA256: "same-content"}}
	if err := legacy.Create(first); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Create(second); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Disconnect(); err != nil {
		t.Fatal(err)
	}
	migrated, err := MigrateBoltToSQLite(path, 0600)
	if err != nil || !migrated {
		t.Fatalf("migrate = %v, err=%v", migrated, err)
	}

	db := &SQLiteDAO{}
	if err := db.Connect(&SQLiteConnectionParams{Path: path, Mode: 0600}); err != nil {
		t.Fatal(err)
	}
	defer db.Disconnect()
	all, err := db.GetAll()
	if err != nil || len(all) != 2 {
		t.Fatalf("migration lost duplicate catalogue records: count=%d err=%v", len(all), err)
	}
	canonical, err := db.SearchByKeyValue("SHA256", "same-content")
	if err != nil || len(canonical) != 1 {
		t.Fatalf("hash registry = %#v err=%v", canonical, err)
	}
	id, err := uuid.Parse(canonical[0].Uuid)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(id); err != nil {
		t.Fatal(err)
	}
	promoted, err := db.SearchByKeyValue("SHA256", "same-content")
	if err != nil || len(promoted) != 1 || promoted[0].Uuid == canonical[0].Uuid {
		t.Fatalf("duplicate hash owner was not promoted: %#v err=%v", promoted, err)
	}
}
