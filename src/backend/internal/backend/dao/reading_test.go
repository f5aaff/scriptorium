package dao

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/google/uuid"
)

func TestReadingStateIsPersonalVersionedAndIndependentOfCatalogueEdits(t *testing.T) {
	db := newTestSQLite(t)
	metadata, _ := extractionFixture(t, db)
	ctx := context.Background()
	initial, err := db.ReadReadingState(ctx, "account-a", metadata.Uuid)
	if err != nil {
		t.Fatal(err)
	}
	if initial.Position != nil || initial.Version != 0 {
		t.Fatal("unexpected initial position")
	}
	position := ReadingPosition{Kind: "pdf", Page: 12, Offset: .3, Scale: 1.25}
	saved, err := db.SaveReadingState(ctx, "account-a", metadata.Uuid, initial.SourceRevision, 0, position)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Version != 1 || saved.Position.Page != 12 {
		t.Fatal("position not saved")
	}
	if _, err := db.SaveReadingState(ctx, "account-a", metadata.Uuid, initial.SourceRevision, 0, position); !errors.Is(err, ErrReadingConflict) {
		t.Fatalf("stale window overwrote a newer place: %v", err)
	}
	other, err := db.ReadReadingState(ctx, "account-b", metadata.Uuid)
	if err != nil || other.Position != nil || other.Version != 0 {
		t.Fatalf("cross-account disclosure: %#v %v", other, err)
	}
	metadata.Title = "Edited title"
	if err := db.Update(&Notes{Metadata: metadata}); err != nil {
		t.Fatal(err)
	}
	restored, err := db.ReadReadingState(ctx, "account-a", metadata.Uuid)
	if err != nil || restored.Position == nil || *restored.Position != position {
		t.Fatalf("metadata edit lost saved place: %#v %v", restored, err)
	}
	if _, err := db.SaveReadingState(ctx, "account-a", metadata.Uuid, "changed-source", 1, position); !errors.Is(err, ErrDocumentChanged) {
		t.Fatalf("source mismatch accepted: %v", err)
	}
}

func TestDeletingDocumentCascadesReadingStateWithoutResurrection(t *testing.T) {
	db := newTestSQLite(t)
	metadata, _ := extractionFixture(t, db)
	ctx := context.Background()
	initial, _ := db.ReadReadingState(ctx, "account", metadata.Uuid)
	position := ReadingPosition{Kind: "text", Page: 1, Scale: 1}
	if _, err := db.SaveReadingState(ctx, "account", metadata.Uuid, initial.SourceRevision, 0, position); err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(uuid.MustParse(metadata.Uuid)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ReadReadingState(ctx, "account", metadata.Uuid); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("deleted document exposed position: %v", err)
	}
	if _, err := db.SaveReadingState(ctx, "account", metadata.Uuid, initial.SourceRevision, 1, position); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("deleted document resurrected: %v", err)
	}
	var count int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM reading_positions`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("orphaned state: %d %v", count, err)
	}
}

func TestReadingPositionValidation(t *testing.T) {
	for _, position := range []ReadingPosition{
		{Kind: "pdf", Page: 0, Scale: 1}, {Kind: "pdf", Page: 1, Scale: 1, Offset: math.NaN()}, {Kind: "pdf", Page: 1, Scale: math.Inf(1)},
		{Kind: "text", Page: 1, Scale: 1, Offset: 1.1}, {Kind: "unknown", Page: 1, Scale: 1}, {Kind: "audio", Offset: -1, Scale: 1},
	} {
		if position.Validate() == nil {
			t.Fatalf("invalid position accepted: %#v", position)
		}
	}
}
