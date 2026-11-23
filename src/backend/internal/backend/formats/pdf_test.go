package formats

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestPDFExtractorProcess is a subprocess fixture, not a shell script. Keeping
// this in Go exercises process cancellation without platform-specific commands.
func TestPDFExtractorProcess(t *testing.T) {
	if os.Getenv("SCRIPTORIUM_PDF_HELPER") != "1" {
		return
	}
	switch os.Getenv("SCRIPTORIUM_PDF_MODE") {
	case "sleep":
		time.Sleep(10 * time.Second)
	case "invalid":
		os.Stdout.Write([]byte("abcdef\xffmore"))
	case "fail":
		os.Stderr.WriteString(strings.Repeat("private path /secret/file.pdf", 10000))
		os.Exit(1)
	default:
		os.Stdout.WriteString(os.Getenv("SCRIPTORIUM_PDF_OUTPUT"))
	}
	os.Exit(0)
}

func pdfTestAdapter(t *testing.T, mode, output string) PDFAdapter {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return PDFAdapter{
		Executable: executable,
		command: func(ctx context.Context, path string, args ...string) *exec.Cmd {
			cmd := exec.CommandContext(ctx, path, "-test.run=^TestPDFExtractorProcess$")
			cmd.Env = append(os.Environ(), "SCRIPTORIUM_PDF_HELPER=1", "SCRIPTORIUM_PDF_MODE="+mode, "SCRIPTORIUM_PDF_OUTPUT="+output)
			return cmd
		},
	}
}

func TestPDFAdapterExtractsAndCleansPrivateSource(t *testing.T) {
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)
	adapter := pdfTestAdapter(t, "text", "Searchable programming text")
	command := adapter.command
	adapter.command = func(ctx context.Context, path string, args ...string) *exec.Cmd {
		if len(args) != 5 || strings.Join(args[:3], " ") != "-enc UTF-8 -nopgbrk" || args[4] != "-" {
			t.Fatalf("unexpected extraction arguments: %q", args)
		}
		info, err := os.Stat(args[3])
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("source is not private: %v, %v", info, err)
		}
		return command(ctx, path, args...)
	}
	result, err := adapter.Extract(context.Background(), strings.NewReader("%PDF-fixture"))
	if err != nil || result.Text != "Searchable programming text" || result.Truncated {
		t.Fatalf("extraction = %#v, %v", result, err)
	}
	files, err := os.ReadDir(temp)
	if err != nil || len(files) != 0 {
		t.Fatalf("staging files not removed: %v, %v", files, err)
	}
}

func TestPDFAdapterBoundsOutputAtUTF8Boundary(t *testing.T) {
	adapter := pdfTestAdapter(t, "text", "abcdefé more text")
	adapter.MaxBytes = 7
	result, err := adapter.Extract(context.Background(), strings.NewReader("pdf"))
	if err != nil || result.Text != "abcdef" || !result.Truncated {
		t.Fatalf("truncated extraction = %#v, %v", result, err)
	}
}

func TestPDFAdapterRejectsInvalidUTF8(t *testing.T) {
	adapter := pdfTestAdapter(t, "invalid", "")
	adapter.MaxBytes = 7
	if _, err := adapter.Extract(context.Background(), strings.NewReader("pdf")); err == nil {
		t.Fatal("invalid UTF-8 was accepted")
	}
}

func TestPDFAdapterReportsUnavailableTool(t *testing.T) {
	adapter := PDFAdapter{Executable: "/nonexistent/scriptorium-pdftotext"}
	_, err := adapter.Extract(context.Background(), strings.NewReader("pdf"))
	if !errors.Is(err, ErrExtractorUnavailable) {
		t.Fatalf("expected unavailable extractor, got %v", err)
	}
}

func TestPDFAdapterFailureAndTimeoutCleanUp(t *testing.T) {
	for _, mode := range []string{"fail", "sleep"} {
		t.Run(mode, func(t *testing.T) {
			temp := t.TempDir()
			t.Setenv("TMPDIR", temp)
			adapter := pdfTestAdapter(t, mode, "")
			if mode == "sleep" {
				adapter.Timeout = 100 * time.Millisecond
			}
			_, err := adapter.Extract(context.Background(), strings.NewReader("pdf"))
			if err == nil || strings.Contains(err.Error(), "/secret/") {
				t.Fatalf("unexpected public error: %v", err)
			}
			files, err := os.ReadDir(temp)
			if err != nil || len(files) != 0 {
				t.Fatalf("staging files not removed: %v, %v", files, err)
			}
		})
	}
}

func TestPDFAdapterHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	adapter := pdfTestAdapter(t, "sleep", "")
	timer := time.AfterFunc(50*time.Millisecond, cancel)
	defer timer.Stop()
	_, err := adapter.Extract(ctx, strings.NewReader("pdf"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestPDFAdapterBoundsConcurrentProcesses(t *testing.T) {
	pdfSlots <- struct{}{}
	pdfSlots <- struct{}{}
	defer func() { <-pdfSlots; <-pdfSlots }()
	adapter := pdfTestAdapter(t, "text", "should not run")
	adapter.Timeout = 20 * time.Millisecond
	_, err := adapter.Extract(context.Background(), strings.NewReader("pdf"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected bounded queue wait, got %v", err)
	}
}

func TestPDFAdapterWithPoppler(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("optional pdftotext executable is not installed")
	}
	file, err := os.Open("testdata/searchable.pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	result, err := (PDFAdapter{}).Extract(context.Background(), file)
	if err != nil || !strings.Contains(result.Text, "RarePDFPhrase") || result.Truncated {
		t.Fatalf("Poppler extraction = %#v, %v", result, err)
	}
}

type zeroPDFReader struct{}

func (zeroPDFReader) Read(data []byte) (int, error) {
	clear(data)
	return len(data), nil
}

func TestPDFAdapterRejectsOversizedSources(t *testing.T) {
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)
	adapter := pdfTestAdapter(t, "text", "must not run")
	adapter.command = func(context.Context, string, ...string) *exec.Cmd {
		t.Fatal("oversized source reached the extractor")
		return nil
	}
	_, err := adapter.Extract(context.Background(), io.LimitReader(zeroPDFReader{}, MaxPDFInputBytes+1))
	if err == nil || !strings.Contains(err.Error(), "input limit") {
		t.Fatalf("expected input limit error, got %v", err)
	}
	files, err := os.ReadDir(temp)
	if err != nil || len(files) != 0 {
		t.Fatalf("oversized source was not removed: %v, %v", files, err)
	}
}

func TestPDFTextWriterClampsMemoryAndCountsAllBytes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	writer := &pdfTextWriter{limit: 8, cancel: cancel}
	for _, data := range []string{"12345", "67890", "discarded"} {
		n, err := writer.Write([]byte(data))
		if n != len(data) || err != nil {
			t.Fatalf("write = %d, %v", n, err)
		}
	}
	if string(writer.data) != "12345678" || !writer.truncated || ctx.Err() != context.Canceled {
		t.Fatalf("unexpected bounded output: %#v", writer)
	}
}
