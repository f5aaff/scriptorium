package service

import (
	"context"
	"errors"
	"net/http"

	"scriptorium/internal/backend/dao"
	"scriptorium/internal/backend/formats"
	"scriptorium/internal/backend/ingest"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ReextractText refreshes derived text for an existing item. Route registration
// requires edit permission; neither request metadata nor arbitrary paths are accepted.
// Failures retain previous indexed text and return only public, path-free errors.
func (f FileHandler) ReextractText(c *gin.Context) {
	id, err := uuid.Parse(c.Param("uuid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid document UUID"})
		return
	}
	if f.Reprocessor == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "text reprocessing is unavailable with this catalogue"})
		return
	}
	metadata, err := f.Reprocessor.Reextract(c.Request.Context(), id)
	if err == nil {
		c.JSON(http.StatusOK, gin.H{"message": "text extraction and search index refreshed", "metadata": metadata})
		return
	}
	status, message := http.StatusInternalServerError, "could not refresh extracted text; previous indexed text was retained"
	switch {
	case errors.Is(err, dao.ErrDocumentNotFound):
		status, message = http.StatusNotFound, "document not found"
	case errors.Is(err, dao.ErrDocumentChanged):
		status, message = http.StatusConflict, dao.ErrDocumentChanged.Error()
	case errors.Is(err, ingest.ErrReprocessCapacity):
		c.Header("Retry-After", "2")
		status, message = http.StatusTooManyRequests, ingest.ErrReprocessCapacity.Error()
	case errors.Is(err, ingest.ErrReprocessBusy):
		status, message = http.StatusConflict, ingest.ErrReprocessBusy.Error()
	case errors.Is(err, ingest.ErrOriginalUnavailable):
		status, message = http.StatusConflict, ingest.ErrOriginalUnavailable.Error()
	case errors.Is(err, ingest.ErrOriginalChanged):
		status, message = http.StatusConflict, ingest.ErrOriginalChanged.Error()
	case errors.Is(err, formats.ErrExtractorUnavailable):
		status, message = http.StatusServiceUnavailable, "extraction tool unavailable; install Poppler (pdftotext) for PDFs; previous indexed text was retained"
	case errors.Is(err, ingest.ErrExtractionUnsupported):
		status, message = http.StatusUnprocessableEntity, ingest.ErrExtractionUnsupported.Error()
	case errors.Is(err, ingest.ErrReprocessFailed):
		status, message = http.StatusUnprocessableEntity, ingest.ErrReprocessFailed.Error()
	case errors.Is(err, context.DeadlineExceeded):
		status, message = http.StatusGatewayTimeout, "text extraction timed out; previous indexed text was retained"
	case errors.Is(err, context.Canceled):
		status, message = http.StatusRequestTimeout, "text extraction was cancelled"
	}
	c.JSON(status, gin.H{"error": message})
}
