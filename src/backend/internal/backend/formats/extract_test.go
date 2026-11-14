package formats

import (
	"context"
	"strings"
	"testing"
)

func TestPlainTextAdapterExtractsBoundedUTF8(t *testing.T) {
	adapter := PlainTextAdapter{MaxBytes: 8}
	result, err := adapter.Extract(context.Background(), strings.NewReader("1234567890"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "12345678" || !result.Truncated {
		t.Fatalf("unexpected bounded extraction: %#v", result)
	}
}

func TestPlainTextAdapterRejectsInvalidUTF8(t *testing.T) {
	adapter := PlainTextAdapter{}
	if _, err := adapter.Extract(context.Background(), strings.NewReader(string([]byte{0xff, 0xfe}))); err == nil {
		t.Fatal("expected invalid UTF-8 to be rejected for indexing")
	}
}

func TestAdapterRegistryReportsUnsupportedFormat(t *testing.T) {
	registry := NewDefaultAdapterRegistry()
	if registry.Supports("unknown.bin") {
		t.Fatal("unexpected adapter for unknown format")
	}
	result, err := registry.Extract(context.Background(), "unknown.bin", strings.NewReader("opaque"))
	if err != nil || result.Text != "" {
		t.Fatalf("unsupported extraction = %#v, err=%v", result, err)
	}
}
