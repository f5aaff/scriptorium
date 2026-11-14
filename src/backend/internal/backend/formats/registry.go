// Package formats identifies uploaded files without restricting the library to
// a fixed extension allowlist. Unknown formats remain valid library objects.
package formats

import (
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
)

type Kind string

const (
	KindDocument Kind = "document"
	KindImage    Kind = "image"
	KindAudio    Kind = "audio"
	KindVideo    Kind = "video"
	KindArchive  Kind = "archive"
	KindOther    Kind = "other"
)

type Identification struct {
	Extension string `json:"extension"`
	MediaType string `json:"media_type"`
	Kind      Kind   `json:"kind"`
	Known     bool   `json:"known"`
}

// Identify uses both the filename extension and the file's actual content.
// Content sniffing is advisory: it does not reject unknown or mismatched files.
func Identify(filename string, source io.Reader) (Identification, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	var sample []byte
	if source != nil {
		buf := make([]byte, 512)
		n, err := io.ReadFull(source, buf)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return Identification{}, err
		}
		sample = buf[:n]
	}
	mediaType := "application/octet-stream"
	if len(sample) > 0 {
		mediaType = http.DetectContentType(sample)
	}
	if mediaType == "text/plain; charset=utf-8" {
		switch ext {
		case ".md":
			mediaType = "text/markdown"
		case ".csv":
			mediaType = "text/csv"
		case ".html", ".htm":
			mediaType = "text/html"
		case ".xml":
			mediaType = "application/xml"
		}
	}
	// Office and ebook containers are ZIP files internally. Their known
	// extensions provide the more useful document-level media type.
	switch ext {
	case ".docx", ".xlsx", ".pptx", ".odt", ".ods", ".odp", ".epub":
		if strings.Contains(mediaType, "zip") || mediaType == "application/octet-stream" {
			if byExt := mime.TypeByExtension(ext); byExt != "" {
				mediaType = byExt
			}
		}
	}
	kind := KindOther
	switch {
	case strings.HasPrefix(mediaType, "text/"), strings.Contains(mediaType, "pdf"), strings.Contains(mediaType, "officedocument"), strings.Contains(mediaType, "epub"), strings.Contains(mediaType, "rtf"):
		kind = KindDocument
	case strings.HasPrefix(mediaType, "image/"):
		kind = KindImage
	case strings.HasPrefix(mediaType, "audio/"):
		kind = KindAudio
	case strings.HasPrefix(mediaType, "video/"):
		kind = KindVideo
	case strings.Contains(mediaType, "zip"), strings.Contains(mediaType, "gzip"), strings.Contains(mediaType, "x-tar"), strings.Contains(mediaType, "7z"):
		kind = KindArchive
	}
	switch ext {
	case ".docx", ".xlsx", ".pptx", ".odt", ".ods", ".odp", ".epub":
		kind = KindDocument
	case ".pdf", ".doc", ".rtf", ".txt", ".md", ".html", ".htm", ".csv", ".xml":
		if kind == KindOther {
			kind = KindDocument
		}
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".tif", ".tiff", ".bmp", ".svg":
		if kind == KindOther {
			kind = KindImage
		}
	case ".mp3", ".wav", ".flac", ".aac", ".ogg", ".m4a":
		if kind == KindOther {
			kind = KindAudio
		}
	case ".mp4", ".avi", ".mov", ".mkv", ".webm":
		if kind == KindOther {
			kind = KindVideo
		}
	case ".zip", ".gz", ".tar", ".7z", ".rar":
		if kind == KindOther {
			kind = KindArchive
		}
	}
	known := mediaType != "application/octet-stream"
	return Identification{Extension: ext, MediaType: mediaType, Kind: kind, Known: known}, nil
}
