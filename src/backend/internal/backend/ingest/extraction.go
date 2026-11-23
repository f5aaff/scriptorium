package ingest

import (
	"context"
	"errors"
	"io"
	"strings"

	"scriptorium/internal/backend/formats"
)

// extractDocument is shared by ingestion and reprocessing so format selection,
// limits, and outcome labels do not drift between the two workflows.
func extractDocument(ctx context.Context, adapters *formats.AdapterRegistry, filename, mediaType string, source io.Reader) (formats.ExtractedMetadata, string, error) {
	if mediaType == "application/pdf" {
		// Prefer a PDF signature to a missing or misleading filename extension.
		filename = "document.pdf"
	}
	if !adapters.Supports(filename) {
		return formats.ExtractedMetadata{}, "unsupported", nil
	}
	result, err := adapters.Extract(ctx, filename, source)
	if ctx.Err() != nil {
		return formats.ExtractedMetadata{}, "failed", ctx.Err()
	}
	if errors.Is(err, formats.ErrExtractorUnavailable) {
		return formats.ExtractedMetadata{}, "unavailable", err
	}
	if err != nil {
		return formats.ExtractedMetadata{}, "failed", err
	}
	if result.Truncated {
		return result, "truncated", nil
	}
	if mediaType == "application/pdf" && strings.TrimSpace(result.Text) == "" {
		result.Text = ""
		return result, "no_text", nil
	}
	return result, "complete", nil
}
