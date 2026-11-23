package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"scriptorium/internal/backend/dao"
	"scriptorium/internal/backend/fao"
	"scriptorium/internal/backend/formats"

	"github.com/google/uuid"
)

type reprocessAdapter struct {
	extract func(context.Context) (formats.ExtractedMetadata, error)
}

func (reprocessAdapter) Extensions() []string { return []string{"pdf"} }
func (a reprocessAdapter) Extract(ctx context.Context, _ io.Reader) (formats.ExtractedMetadata, error) {
	return a.extract(ctx)
}

func reprocessFixture(t *testing.T, adapters *formats.AdapterRegistry, source string) (*Reprocessor, *dao.SQLiteDAO, *fao.LocalFao, dao.MetaData) {
	t.Helper()
	db := &dao.SQLiteDAO{}
	if err := db.Connect(&dao.SQLiteConnectionParams{Path: filepath.Join(t.TempDir(), "library.sqlite"), Mode: 0600}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Disconnect() })
	storage := fao.NewLocalFao(t.TempDir())
	if err := storage.SaveFile("original-key", strings.NewReader(source)); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(source))
	metadata := dao.MetaData{
		Uuid: uuid.NewString(), Path: "original-key", OriginalFilename: "original.pdf", FileType: ".pdf", MediaType: "application/pdf",
		Title: "My title", Author: "My author", DocType: "Book", SHA256: hex.EncodeToString(hash[:]), Size: int64(len(source)),
		ExtractionStatus: "failed", ExtractedText: "previousuniquephrase", DeweyDecimal: "900", ClassificationStatus: "confirmed",
	}
	if err := db.Create(&dao.Notes{Metadata: metadata}); err != nil {
		t.Fatal(err)
	}
	processor, err := NewReprocessor(db, storage, adapters)
	if err != nil {
		t.Fatal(err)
	}
	return processor, db, storage, metadata
}

func TestReextractRefreshesIndexAndPreservesOriginalAndMetadata(t *testing.T) {
	adapter := fixedPDFAdapter{result: formats.ExtractedMetadata{Text: "replacementuniquephrase programming", Title: "Extractor title", Author: "Extractor author"}}
	processor, db, storage, metadata := reprocessFixture(t, formats.NewAdapterRegistry(adapter), "%PDF-1.4\nfixture")
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)
	result, err := processor.Reextract(context.Background(), uuid.MustParse(metadata.Uuid))
	if err != nil || result.Title != metadata.Title || result.Author != metadata.Author || result.DeweyDecimal != "900" || result.ClassificationStatus != "confirmed" || result.SHA256 != metadata.SHA256 || result.Path != metadata.Path {
		t.Fatalf("re-extraction changed user metadata: %#v, %v", result, err)
	}
	for query, count := range map[string]int{"previousuniquephrase": 0, "replacementuniquephrase": 1} {
		matches, err := db.FuzzySearch(query)
		if err != nil || len(matches) != count {
			t.Fatalf("search %q = %#v, %v", query, matches, err)
		}
	}
	original, err := storage.GetFile(metadata.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer original.Close()
	bytes, err := io.ReadAll(original)
	if err != nil || string(bytes) != "%PDF-1.4\nfixture" {
		t.Fatalf("original changed: %q, %v", bytes, err)
	}
	files, err := os.ReadDir(temp)
	if err != nil || len(files) != 0 {
		t.Fatalf("staging files not removed: %v, %v", files, err)
	}
}

func TestReextractFailuresRetainPreviousText(t *testing.T) {
	for _, test := range []struct {
		name    string
		adapter fixedPDFAdapter
		want    error
	}{
		{"missing tool", fixedPDFAdapter{err: formats.ErrExtractorUnavailable}, formats.ErrExtractorUnavailable},
		{"parser failure", fixedPDFAdapter{err: errors.New("/private/parser-error")}, ErrReprocessFailed},
	} {
		t.Run(test.name, func(t *testing.T) {
			processor, db, _, metadata := reprocessFixture(t, formats.NewAdapterRegistry(test.adapter), "%PDF-1.4\nfixture")
			id := uuid.MustParse(metadata.Uuid)
			before, _ := db.ReadRaw(id)
			temp := t.TempDir()
			t.Setenv("TMPDIR", temp)
			_, err := processor.Reextract(context.Background(), id)
			if !errors.Is(err, test.want) || strings.Contains(err.Error(), "/private/") {
				t.Fatalf("unexpected public failure: %v", err)
			}
			after, _ := db.ReadRaw(id)
			matches, err := db.FuzzySearch("previousuniquephrase")
			if err != nil || len(matches) != 1 || string(before) != string(after) {
				t.Fatalf("failed extraction replaced previous text or metadata: %s, %v", after, err)
			}
			files, err := os.ReadDir(temp)
			if err != nil || len(files) != 0 {
				t.Fatalf("failed extraction left staging files: %v, %v", files, err)
			}
		})
	}
}

func TestReextractRejectsUnsupportedOrUnavailableOriginal(t *testing.T) {
	for _, mode := range []string{"unsupported", "missing", "changed"} {
		t.Run(mode, func(t *testing.T) {
			processor, db, storage, metadata := reprocessFixture(t, formats.NewAdapterRegistry(), "%PDF-1.4\nfixture")
			want := ErrExtractionUnsupported
			if mode == "missing" || mode == "changed" {
				if err := storage.DeleteFile(metadata.Path); err != nil {
					t.Fatal(err)
				}
				want = ErrOriginalUnavailable
				if mode == "changed" {
					if err := storage.SaveFile(metadata.Path, strings.NewReader("%PDF-1.4\nmodified")); err != nil {
						t.Fatal(err)
					}
					want = ErrOriginalChanged
				}
			}
			_, err := processor.Reextract(context.Background(), uuid.MustParse(metadata.Uuid))
			if !errors.Is(err, want) {
				t.Fatalf("expected %v, got %v", want, err)
			}
			matches, err := db.FuzzySearch("previousuniquephrase")
			if err != nil || len(matches) != 1 {
				t.Fatalf("failure lost previous text: %#v, %v", matches, err)
			}
		})
	}
}

func TestReextractRejectsOverlappingRequests(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	adapter := reprocessAdapter{extract: func(ctx context.Context) (formats.ExtractedMetadata, error) {
		close(started)
		select {
		case <-release:
			return formats.ExtractedMetadata{Text: "replacement"}, nil
		case <-ctx.Done():
			return formats.ExtractedMetadata{}, ctx.Err()
		}
	}}
	processor, _, _, metadata := reprocessFixture(t, formats.NewAdapterRegistry(adapter), "%PDF-1.4\nfixture")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	id := uuid.MustParse(metadata.Uuid)
	go func() { _, err := processor.Reextract(ctx, id); done <- err }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("first extraction did not start")
	}
	if _, err := processor.Reextract(ctx, id); !errors.Is(err, ErrReprocessBusy) {
		t.Errorf("expected busy conflict, got %v", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestReextractBoundsActiveStagingAndExtraction(t *testing.T) {
	started, release := make(chan struct{}, MaxConcurrentReextractions), make(chan struct{})
	adapter := reprocessAdapter{extract: func(ctx context.Context) (formats.ExtractedMetadata, error) {
		started <- struct{}{}
		select {
		case <-release:
			return formats.ExtractedMetadata{Text: "replacement"}, nil
		case <-ctx.Done():
			return formats.ExtractedMetadata{}, ctx.Err()
		}
	}}
	processor, db, _, metadata := reprocessFixture(t, formats.NewAdapterRegistry(adapter), "%PDF-1.4\nfixture")
	ids := []uuid.UUID{uuid.MustParse(metadata.Uuid)}
	for i := 0; i < MaxConcurrentReextractions; i++ {
		metadata.Uuid, metadata.SHA256 = uuid.NewString(), ""
		if err := db.Create(&dao.Notes{Metadata: metadata}); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, uuid.MustParse(metadata.Uuid))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, MaxConcurrentReextractions)
	for _, id := range ids[:MaxConcurrentReextractions] {
		go func(id uuid.UUID) { _, err := processor.Reextract(ctx, id); done <- err }(id)
	}
	for i := 0; i < MaxConcurrentReextractions; i++ {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("extraction did not start")
		}
	}
	if _, err := processor.Reextract(ctx, ids[MaxConcurrentReextractions]); !errors.Is(err, ErrReprocessCapacity) {
		t.Errorf("expected capacity limit, got %v", err)
	}
	close(release)
	for i := 0; i < MaxConcurrentReextractions; i++ {
		if err := <-done; err != nil {
			t.Error(err)
		}
	}
}

func TestReextractRejectsConcurrentEditAndHonorsCancellation(t *testing.T) {
	for _, mode := range []string{"edit", "delete", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var db *dao.SQLiteDAO
			var metadata dao.MetaData
			adapter := reprocessAdapter{extract: func(context.Context) (formats.ExtractedMetadata, error) {
				switch mode {
				case "edit":
					metadata.Title = "Concurrent edit"
					if err := db.Update(&dao.Notes{Metadata: metadata}); err != nil {
						t.Fatal(err)
					}
				case "delete":
					if err := db.Delete(uuid.MustParse(metadata.Uuid)); err != nil {
						t.Fatal(err)
					}
				case "cancel":
					cancel()
				}
				return formats.ExtractedMetadata{Text: "replacement"}, nil
			}}
			var processor *Reprocessor
			processor, db, _, metadata = reprocessFixture(t, formats.NewAdapterRegistry(adapter), "%PDF-1.4\nfixture")
			_, err := processor.Reextract(ctx, uuid.MustParse(metadata.Uuid))
			want := map[string]error{"edit": dao.ErrDocumentChanged, "delete": dao.ErrDocumentNotFound, "cancel": context.Canceled}[mode]
			if !errors.Is(err, want) {
				t.Fatalf("expected %v, got %v", want, err)
			}
			if mode != "delete" {
				matches, err := db.FuzzySearch("previousuniquephrase")
				if err != nil || len(matches) != 1 {
					t.Fatalf("old index lost: %#v, %v", matches, err)
				}
			} else if all, err := db.GetAll(); err != nil || len(all) != 0 {
				t.Fatalf("deleted record resurrected: %#v, %v", all, err)
			}
		})
	}
}

func TestReextractExistingPDFWithPoppler(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("optional pdftotext is not installed")
	}
	data, err := os.ReadFile("../formats/testdata/searchable.pdf")
	if err != nil {
		t.Fatal(err)
	}
	processor, db, _, metadata := reprocessFixture(t, nil, string(data))
	result, err := processor.Reextract(context.Background(), uuid.MustParse(metadata.Uuid))
	if err != nil || result.ExtractionStatus != "complete" {
		t.Fatalf("real PDF reprocessing = %#v, %v", result, err)
	}
	matches, err := db.FuzzySearch("RarePDFPhrase")
	if err != nil || len(matches) != 1 {
		t.Fatalf("PDF reprocessing did not index text: %#v, %v", matches, err)
	}
	public, err := json.Marshal(result)
	if err != nil || strings.Contains(string(public), "RarePDFPhrase") {
		t.Fatalf("extracted text leaked in public metadata: %s, %v", public, err)
	}
}
