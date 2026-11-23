package ingest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"scriptorium/internal/backend/dao"
	"scriptorium/internal/backend/fao"
	"scriptorium/internal/backend/formats"
)

type fixedPDFAdapter struct {
	result formats.ExtractedMetadata
	err    error
}

func (fixedPDFAdapter) Extensions() []string { return []string{"pdf"} }
func (a fixedPDFAdapter) Extract(context.Context, io.Reader) (formats.ExtractedMetadata, error) {
	return a.result, a.err
}

func TestPDFExtractionStatusPreservesOriginal(t *testing.T) {
	for _, test := range []struct {
		name, status string
		adapter      fixedPDFAdapter
	}{
		{"missing tool", "unavailable", fixedPDFAdapter{err: formats.ErrExtractorUnavailable}},
		{"invalid PDF", "failed", fixedPDFAdapter{err: errors.New("invalid PDF")}},
		{"image-only PDF", "no_text", fixedPDFAdapter{result: formats.ExtractedMetadata{Text: " \n"}}},
		{"bounded text", "truncated", fixedPDFAdapter{result: formats.ExtractedMetadata{Text: "programming", Truncated: true}}},
		{"full text", "complete", fixedPDFAdapter{result: formats.ExtractedMetadata{Text: "programming software"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			catalog := &memoryCatalog{}
			storage := fao.NewLocalFao(t.TempDir())
			service, err := New(catalog, storage, formats.NewAdapterRegistry(test.adapter), DefaultMaxFileSize)
			if err != nil {
				t.Fatal(err)
			}
			const source = "%PDF-1.4\nfixture"
			result, err := service.Import(context.Background(), Request{Filename: "document.pdf"}, strings.NewReader(source))
			if err != nil || result.Metadata.ExtractionStatus != test.status {
				t.Fatalf("import = %#v, %v", result, err)
			}
			original, err := storage.GetFile(result.Metadata.Path)
			if err != nil {
				t.Fatal(err)
			}
			defer original.Close()
			data, err := io.ReadAll(original)
			if err != nil || string(data) != source {
				t.Fatalf("original changed: %q, %v", data, err)
			}
			if test.status == "complete" && (result.Metadata.SuggestedDeweyDecimal != "004" || result.Metadata.DeweyDecimal != "") {
				t.Fatalf("PDF text not used for suggested classification: %#v", result.Metadata)
			}
		})
	}
}

func TestPDFImportWithoutPoppler(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	storage := fao.NewLocalFao(t.TempDir())
	service, err := New(&memoryCatalog{}, storage, nil, DefaultMaxFileSize)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Import(context.Background(), Request{Filename: "without-tool.pdf"}, strings.NewReader("%PDF-1.4\nfixture"))
	if err != nil || result.Metadata.ExtractionStatus != "unavailable" || !storage.FileExists(result.Metadata.Path) {
		t.Fatalf("PDF without Poppler = %#v, %v", result, err)
	}
}

func TestSignatureIdentifiedPDFUsesPDFAdapter(t *testing.T) {
	for _, filename := range []string{"no-extension", "misnamed.txt", "unknown.bin"} {
		t.Run(filename, func(t *testing.T) {
			adapter := fixedPDFAdapter{result: formats.ExtractedMetadata{Text: "Searchable PDF text"}}
			service, err := New(&memoryCatalog{}, fao.NewLocalFao(t.TempDir()), formats.NewAdapterRegistry(adapter, formats.PlainTextAdapter{}), DefaultMaxFileSize)
			if err != nil {
				t.Fatal(err)
			}
			result, err := service.Import(context.Background(), Request{Filename: filename}, strings.NewReader("%PDF-1.4\nfixture"))
			if err != nil || result.Metadata.ExtractedText != adapter.result.Text || result.Metadata.OriginalFilename != filename {
				t.Fatalf("signature-selected extraction = %#v, %v", result, err)
			}
		})
	}
}

func TestPDFImportWithoutTextWithPoppler(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("optional pdftotext executable is not installed")
	}
	data, err := os.ReadFile("../formats/testdata/searchable.pdf")
	if err != nil {
		t.Fatal(err)
	}
	// Keep the PDF's byte offsets and stream length unchanged while replacing
	// the text drawing instructions with legal whitespace-only page content.
	const instructions = "BT /F1 12 Tf 72 720 Td (RarePDFPhrase programming software databases) Tj ET"
	blank := bytes.Replace(data, []byte(instructions), bytes.Repeat([]byte(" "), len(instructions)), 1)
	service, err := New(&memoryCatalog{}, fao.NewLocalFao(t.TempDir()), nil, DefaultMaxFileSize)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, status string
		data         []byte
	}{
		{"blank.pdf", "no_text", blank},
		{"broken.pdf", "failed", []byte("%PDF-1.4\nnot a valid document")},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := service.Import(context.Background(), Request{Filename: test.name}, bytes.NewReader(test.data))
			if err != nil || result.Metadata.ExtractionStatus != test.status {
				t.Fatalf("PDF import = %#v, %v", result, err)
			}
		})
	}
}

func TestPDFImportIndexesTextWithPoppler(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("optional pdftotext executable is not installed")
	}
	catalog := &dao.SQLiteDAO{}
	if err := catalog.Connect(&dao.SQLiteConnectionParams{Path: filepath.Join(t.TempDir(), "library.sqlite"), Mode: 0600}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { catalog.Disconnect() })
	service, err := New(catalog, fao.NewLocalFao(t.TempDir()), formats.NewDefaultAdapterRegistry(), DefaultMaxFileSize)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open("../formats/testdata/searchable.pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	result, err := service.Import(context.Background(), Request{Filename: "fixture.pdf"}, file)
	if err != nil || result.Metadata.ExtractionStatus != "complete" {
		t.Fatalf("PDF import = %#v, %v", result, err)
	}
	matches, err := catalog.FuzzySearch("RarePDFPhrase")
	if err != nil || len(matches) != 1 || matches[0].Uuid != result.Metadata.Uuid {
		t.Fatalf("PDF text not indexed: %#v, %v", matches, err)
	}
	if result.Metadata.SuggestedDeweyDecimal != "004" {
		t.Fatalf("missing PDF classification suggestion: %#v", result.Metadata)
	}
}
