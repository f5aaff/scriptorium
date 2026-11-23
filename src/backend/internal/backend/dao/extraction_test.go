package dao

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
)

func extractionFixture(t *testing.T, db *SQLiteDAO) (MetaData, []byte) {
	t.Helper()
	metadata := MetaData{
		Uuid: uuid.NewString(), Title: "User title", Author: "User author", PublishDate: "2000-01-01",
		DocType: "Book", Path: "original-key", SHA256: "original-hash", Size: 123,
		OriginalFilename: "original.pdf", FileType: ".pdf", MediaType: "application/pdf",
		Subjects: []string{"User subject"}, ExtraMetadata: map[string]string{"note": "User note"},
		ExtractionStatus: "failed", ExtractedText: "olduniquephrase",
	}
	if err := db.Create(&Notes{Metadata: metadata}); err != nil {
		t.Fatal(err)
	}
	raw, err := db.ReadRaw(uuid.MustParse(metadata.Uuid))
	if err != nil {
		t.Fatal(err)
	}
	return metadata, raw
}

func TestReplaceExtractionPreservesMetadataAndReplacesIndex(t *testing.T) {
	db := newTestSQLite(t)
	before, raw := extractionFixture(t, db)
	result, err := db.ReplaceExtraction(context.Background(), uuid.MustParse(before.Uuid), raw, ExtractionUpdate{
		Text: "newuniquephrase programming", Status: "complete", Dewey: "004", DeweyName: "Computer science", Confidence: .6, Evidence: []string{"programming"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Title != before.Title || result.Author != before.Author || result.Path != before.Path || result.SHA256 != before.SHA256 || result.Size != before.Size || result.PublishDate != before.PublishDate || result.DocType != before.DocType || !reflect.DeepEqual(result.Subjects, before.Subjects) || !reflect.DeepEqual(result.ExtraMetadata, before.ExtraMetadata) {
		t.Fatalf("editable metadata or identity changed: %#v", result)
	}
	if result.ExtractionStatus != "complete" || result.SuggestedDeweyDecimal != "004" || result.ClassificationStatus != "suggested" {
		t.Fatalf("derived metadata not refreshed: %#v", result)
	}
	for query, count := range map[string]int{"olduniquephrase": 0, "newuniquephrase": 1} {
		results, err := db.FuzzySearch(query)
		if err != nil || len(results) != count {
			t.Fatalf("search %q = %#v, %v", query, results, err)
		}
	}
	// An editor opened before extraction can still save editable fields, but
	// must not undo the newer derived fields or reintroduce stale indexed text.
	before.Title = "Edited later"
	if err := db.Update(&Notes{Metadata: before}); err != nil {
		t.Fatal(err)
	}
	savedRaw, err := db.ReadRaw(uuid.MustParse(before.Uuid))
	if err != nil {
		t.Fatal(err)
	}
	var saved MetaData
	if err := json.Unmarshal(savedRaw, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Title != "Edited later" || saved.ExtractionStatus != "complete" || saved.SuggestedDeweyDecimal != "004" {
		t.Fatalf("stale editor overwrote extraction: %#v", saved)
	}
	matches, err := db.FuzzySearch("newuniquephrase")
	if err != nil || len(matches) != 1 {
		t.Fatalf("stale editor lost indexed text: %#v, %v", matches, err)
	}
}

func TestReplaceExtractionPreservesConfirmedDewey(t *testing.T) {
	db := newTestSQLite(t)
	metadata, _ := extractionFixture(t, db)
	metadata.DeweyDecimal, metadata.ClassificationStatus = "900", "confirmed"
	if err := db.Update(&Notes{Metadata: metadata}); err != nil {
		t.Fatal(err)
	}
	raw, _ := db.ReadRaw(uuid.MustParse(metadata.Uuid))
	result, err := db.ReplaceExtraction(context.Background(), uuid.MustParse(metadata.Uuid), raw, ExtractionUpdate{Text: "programming", Status: "complete", Dewey: "004"})
	if err != nil || result.DeweyDecimal != "900" || result.ClassificationStatus != "confirmed" || result.SuggestedDeweyDecimal != "" {
		t.Fatalf("confirmed classification overwritten: %#v, %v", result, err)
	}
}

func TestReplaceExtractionRejectsConcurrentEditOrDeletion(t *testing.T) {
	for _, deleted := range []bool{false, true} {
		t.Run(map[bool]string{false: "edited", true: "deleted"}[deleted], func(t *testing.T) {
			db := newTestSQLite(t)
			metadata, raw := extractionFixture(t, db)
			id := uuid.MustParse(metadata.Uuid)
			want := ErrDocumentChanged
			if deleted {
				if err := db.Delete(id); err != nil {
					t.Fatal(err)
				}
				want = ErrDocumentNotFound
			} else {
				metadata.Title = "Concurrent edit"
				if err := db.Update(&Notes{Metadata: metadata}); err != nil {
					t.Fatal(err)
				}
			}
			_, err := db.ReplaceExtraction(context.Background(), id, raw, ExtractionUpdate{Text: "new text", Status: "complete"})
			if !errors.Is(err, want) {
				t.Fatalf("expected %v, got %v", want, err)
			}
			all, err := db.GetAll()
			if err != nil || (deleted && len(all) != 0) || (!deleted && (len(all) != 1 || all[0].Title != "Concurrent edit")) {
				t.Fatalf("concurrent change lost: %#v, %v", all, err)
			}
		})
	}
}

func TestReplaceExtractionRollsBackOnIndexFailure(t *testing.T) {
	db := newTestSQLite(t)
	metadata, raw := extractionFixture(t, db)
	if _, err := db.db.Exec(`DROP TABLE documents_fts`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ReplaceExtraction(context.Background(), uuid.MustParse(metadata.Uuid), raw, ExtractionUpdate{Text: "replacement", Status: "complete"}); err == nil {
		t.Fatal("expected index failure")
	}
	after, err := db.ReadRaw(uuid.MustParse(metadata.Uuid))
	if err != nil || string(after) != string(raw) {
		t.Fatalf("failed index write changed metadata: %s, %v", after, err)
	}
	var text string
	if err := db.db.QueryRow(`SELECT extracted_text FROM documents WHERE id=?`, metadata.Uuid).Scan(&text); err != nil || text != "olduniquephrase" {
		t.Fatalf("failed index write changed text: %q, %v", text, err)
	}
}

func TestReplaceExtractionClearsOldTextForNoTextResult(t *testing.T) {
	db := newTestSQLite(t)
	metadata, raw := extractionFixture(t, db)
	result, err := db.ReplaceExtraction(context.Background(), uuid.MustParse(metadata.Uuid), raw, ExtractionUpdate{Status: "no_text"})
	if err != nil || result.ExtractedText != "" || result.ExtractionStatus != "no_text" {
		t.Fatalf("no-text result = %#v, %v", result, err)
	}
	matches, err := db.FuzzySearch("olduniquephrase")
	if err != nil || len(matches) != 0 {
		t.Fatalf("old text retained after successful empty extraction: %#v, %v", matches, err)
	}
}
