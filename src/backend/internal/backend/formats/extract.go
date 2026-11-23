package formats

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const DefaultTextExtractionLimit = 1 << 20 // 1 MiB per item; keeps catalogue indexing bounded.

type ExtractedMetadata struct {
	Title         string
	Author        string
	PublishedDate string
	Text          string
	Subjects      []string
	Truncated     bool
}

type Adapter interface {
	Extensions() []string
	Extract(context.Context, io.Reader) (ExtractedMetadata, error)
}

type AdapterRegistry struct {
	adapters map[string]Adapter
}

func NewAdapterRegistry(adapters ...Adapter) *AdapterRegistry {
	r := &AdapterRegistry{adapters: make(map[string]Adapter)}
	for _, adapter := range adapters {
		r.Register(adapter)
	}
	return r
}

func (r *AdapterRegistry) Register(adapter Adapter) {
	if r == nil || adapter == nil {
		return
	}
	if r.adapters == nil {
		r.adapters = make(map[string]Adapter)
	}
	for _, ext := range adapter.Extensions() {
		r.adapters[strings.ToLower(strings.TrimPrefix(ext, "."))] = adapter
	}
}

func (r *AdapterRegistry) Supports(filename string) bool {
	if r == nil {
		return false
	}
	_, ok := r.adapters[strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), "."))]
	return ok
}

func (r *AdapterRegistry) Extract(ctx context.Context, filename string, source io.Reader) (ExtractedMetadata, error) {
	if r == nil {
		return ExtractedMetadata{}, nil
	}
	adapter := r.adapters[strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), "."))]
	if adapter == nil {
		return ExtractedMetadata{}, nil
	}
	return adapter.Extract(ctx, source)
}

// PlainTextAdapter extracts searchable text from plain-text and Markdown files.
// It intentionally does not execute markup, scripts, macros, or embedded content.
type PlainTextAdapter struct{ MaxBytes int64 }

func (a PlainTextAdapter) Extensions() []string {
	return []string{"txt", "text", "md", "markdown", "csv", "log", "rst"}
}

func (a PlainTextAdapter) Extract(ctx context.Context, source io.Reader) (ExtractedMetadata, error) {
	if source == nil {
		return ExtractedMetadata{}, errors.New("text source is required")
	}
	limit := a.MaxBytes
	if limit <= 0 || limit > DefaultTextExtractionLimit {
		limit = DefaultTextExtractionLimit
	}
	limited := io.LimitReader(source, limit+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return ExtractedMetadata{}, err
	}
	truncated := int64(len(data)) > limit
	if truncated {
		data = data[:limit]
	}
	if !utf8.Valid(data) {
		return ExtractedMetadata{}, errors.New("text file is not valid UTF-8; original retained without text indexing")
	}
	select {
	case <-ctx.Done():
		return ExtractedMetadata{}, ctx.Err()
	default:
	}
	return ExtractedMetadata{Text: string(data), Truncated: truncated}, nil
}

// NewDefaultAdapterRegistry registers plain-text and optional Poppler PDF extraction.
// Missing optional tools are reported during extraction, not application startup.
func NewDefaultAdapterRegistry() *AdapterRegistry {
	return NewAdapterRegistry(PlainTextAdapter{MaxBytes: DefaultTextExtractionLimit}, PDFAdapter{})
}
