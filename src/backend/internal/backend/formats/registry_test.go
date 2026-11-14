package formats

import (
	"bytes"
	"strings"
	"testing"
)

func TestIdentifyRecognisesKnownContent(t *testing.T) {
	got, err := Identify("paper.pdf", strings.NewReader("%PDF-1.7\nexample"))
	if err != nil {
		t.Fatal(err)
	}
	if got.MediaType != "application/pdf" || !got.Known || got.Kind != KindDocument {
		t.Fatalf("unexpected identification: %+v", got)
	}
}

func TestIdentifyAcceptsUnknownFormats(t *testing.T) {
	got, err := Identify("archive.custom", bytes.NewReader([]byte{0x00, 0x01, 0xff, 0x10}))
	if err != nil {
		t.Fatal(err)
	}
	if got.Extension != ".custom" || got.Kind != KindOther {
		t.Fatalf("unexpected identification: %+v", got)
	}
}

func TestIdentifyRecognisesOfficeContainerByExtension(t *testing.T) {
	got, err := Identify("report.docx", strings.NewReader("PK\x03\x04container"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != KindDocument {
		t.Fatalf("expected document kind for DOCX, got %+v", got)
	}
}
