package formats

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
	"unicode/utf8"
)

// ErrExtractorUnavailable indicates that an optional extraction tool is not installed.
var ErrExtractorUnavailable = errors.New("extraction tool is unavailable")

const (
	// DefaultPDFTimeout bounds staging, queueing, and text extraction for one PDF.
	DefaultPDFTimeout = 30 * time.Second
	// MaxPDFInputBytes bounds temporary disk use for a single PDF extraction.
	MaxPDFInputBytes int64 = 100 << 20
)

// pdfSlots also bounds subprocesses started by synchronous uploads. Batch-worker
// limits alone do not limit requests to the synchronous upload endpoint.
var pdfSlots = make(chan struct{}, 2)

// PDFAdapter extracts text using Poppler's optional pdftotext executable.
// It does not perform OCR or extract embedded metadata. Its zero value is usable.
// Subprocesses are time/output/concurrency bounded, but are not OS-sandboxed.
type PDFAdapter struct {
	Executable string        // Defaults to pdftotext, resolved through PATH without a shell.
	MaxBytes   int64         // Output limit; clamped to DefaultTextExtractionLimit.
	Timeout    time.Duration // Clamped to DefaultPDFTimeout.
	command    func(context.Context, string, ...string) *exec.Cmd
}

// Extensions returns the filename extensions handled by PDFAdapter.
func (PDFAdapter) Extensions() []string { return []string{"pdf"} }

// Extract stages a bounded private source, extracts UTF-8 text, and removes the
// temporary file on every return path. Reaching the text limit stops the process
// and returns a truncated UTF-8 prefix. Parent cancellation remains an error.
func (a PDFAdapter) Extract(ctx context.Context, source io.Reader) (ExtractedMetadata, error) {
	if source == nil {
		return ExtractedMetadata{}, errors.New("PDF source is required")
	}
	if err := ctx.Err(); err != nil {
		return ExtractedMetadata{}, err
	}
	binary := a.Executable
	if binary == "" {
		binary = "pdftotext"
	}
	path, err := exec.LookPath(binary)
	if err != nil {
		return ExtractedMetadata{}, fmt.Errorf("%w: pdftotext", ErrExtractorUnavailable)
	}
	timeout := a.Timeout
	if timeout <= 0 || timeout > DefaultPDFTimeout {
		timeout = DefaultPDFTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	select {
	case pdfSlots <- struct{}{}:
		defer func() { <-pdfSlots }()
	case <-runCtx.Done():
		return ExtractedMetadata{}, runCtx.Err()
	}

	file, err := os.CreateTemp("", "scriptorium-pdf-*")
	if err != nil {
		return ExtractedMetadata{}, errors.New("cannot stage PDF for extraction")
	}
	defer os.Remove(file.Name())
	defer file.Close()
	written, err := io.Copy(file, io.LimitReader(pdfContextReader{ctx: runCtx, source: source}, MaxPDFInputBytes+1))
	if err != nil {
		if runCtx.Err() != nil {
			return ExtractedMetadata{}, runCtx.Err()
		}
		return ExtractedMetadata{}, errors.New("cannot read PDF source")
	}
	if written > MaxPDFInputBytes {
		return ExtractedMetadata{}, errors.New("PDF exceeds extraction input limit")
	}
	if err := file.Close(); err != nil {
		return ExtractedMetadata{}, errors.New("cannot finish staging PDF")
	}

	limit := a.MaxBytes
	if limit <= 0 || limit > DefaultTextExtractionLimit {
		limit = DefaultTextExtractionLimit
	}
	output := &pdfTextWriter{limit: int(limit), cancel: cancel}
	command := a.command
	if command == nil {
		command = exec.CommandContext
	}
	cmd := command(runCtx, path, "-enc", "UTF-8", "-nopgbrk", file.Name(), "-")
	cmd.Stdout = output
	// Tool diagnostics can contain private paths or grow without bound. They are
	// intentionally discarded rather than buffered or included in public errors.
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	err = cmd.Run()
	if ctx.Err() != nil {
		return ExtractedMetadata{}, ctx.Err()
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return ExtractedMetadata{}, errors.New("PDF text extraction timed out")
	}
	if err != nil && !output.truncated {
		return ExtractedMetadata{}, errors.New("PDF text extraction failed")
	}
	data := output.data
	// A byte limit can split the final UTF-8 rune. Trim only an incomplete suffix;
	// invalid bytes elsewhere still cause extraction to fail.
	if output.truncated && len(data) > 0 {
		start := len(data) - 1
		for start > 0 && !utf8.RuneStart(data[start]) {
			start--
		}
		if !utf8.FullRune(data[start:]) {
			data = data[:start]
		}
	}
	if !utf8.Valid(data) {
		return ExtractedMetadata{}, errors.New("PDF extractor returned invalid UTF-8")
	}
	return ExtractedMetadata{Text: string(data), Truncated: output.truncated}, nil
}

// pdfTextWriter bounds captured output and stops extraction once it exceeds the
// limit. Write reports consumed bytes so os/exec can finish draining its pipe.
type pdfTextWriter struct {
	data      []byte
	limit     int
	truncated bool
	cancel    context.CancelFunc
}

func (w *pdfTextWriter) Write(data []byte) (int, error) {
	n := len(data)
	remaining := w.limit - len(w.data)
	if len(data) > remaining {
		w.data = append(w.data, data[:remaining]...)
		w.truncated = true
		w.cancel()
	} else {
		w.data = append(w.data, data...)
	}
	return n, nil
}

type pdfContextReader struct {
	ctx    context.Context
	source io.Reader
}

func (r pdfContextReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.source.Read(data)
}
