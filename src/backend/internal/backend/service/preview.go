package service

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"scriptorium/internal/backend/dao"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// PreviewFile serves an authenticated, bounded original with byte-range support.
// Preview types come from file signatures, never editable metadata or extensions.
// Active document formats are served only as inert text, never HTML or SVG.
func (f FileHandler) PreviewFile(c *gin.Context) {
	id, err := uuid.Parse(c.Param("uuid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid document UUID"})
		return
	}
	raw, err := f.APIHandler.DaoService.ReadRaw(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "document not found"})
		return
	}
	var metadata dao.MetaData
	if err := json.Unmarshal(raw, &metadata); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid document metadata"})
		return
	}
	file, err := f.FaoService.GetFile(metadata.Path)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "stored original is unavailable"})
		return
	}
	defer file.Close()
	seekable, ok := file.(io.ReadSeeker)
	if !ok {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "this storage backend does not support file previews"})
		return
	}
	size, err := seekable.Seek(0, io.SeekEnd)
	if err != nil || size < 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot read original size"})
		return
	}
	if size > maxFileSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "original exceeds the 100 MiB preview limit; download it instead"})
		return
	}
	if _, err := seekable.Seek(0, io.SeekStart); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot read original"})
		return
	}
	sample := make([]byte, 512)
	n, err := io.ReadFull(seekable, sample)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot identify original"})
		return
	}
	kind, contentType := previewType(sample[:n])
	if kind == "" {
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "this format has no built-in preview; download the original instead"})
		return
	}
	if _, err := seekable.Seek(0, io.SeekStart); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot read original"})
		return
	}
	name := metadata.OriginalFilename
	if name == "" {
		name = metadata.Title
	}
	if name == "" {
		name = id.String()
	}
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": name}))
	c.Header("X-Scriptorium-Preview-Kind", kind)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "no-store")
	// This endpoint is fetched as data by the viewer. Direct navigation must not
	// turn an uploaded document into an active page on the application's origin.
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox; frame-ancestors 'none'")
	http.ServeContent(c.Writer, c.Request, name, time.Time{}, seekable)
}

// previewType allows only browser media with recognized signatures. HTML, XML,
// SVG, scripts, and Markdown may be inspected as text but are never executed.
func previewType(sample []byte) (kind, contentType string) {
	sniffed := http.DetectContentType(sample)
	mediaType, _, _ := mime.ParseMediaType(sniffed)
	switch mediaType {
	case "application/pdf":
		return "pdf", "application/pdf"
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp", "image/x-icon":
		return "image", mediaType
	case "audio/mpeg", "audio/wave", "audio/aiff", "audio/midi":
		return "audio", mediaType
	case "video/mp4", "video/webm", "video/avi":
		return "video", mediaType
	case "application/ogg":
		if bytes.Contains(sample, []byte("theora")) {
			return "video", "video/ogg"
		}
		return "audio", "audio/ogg"
	}
	if strings.HasPrefix(mediaType, "text/") || mediaType == "application/xml" {
		return "text", "text/plain; charset=utf-8"
	}
	return "", ""
}
