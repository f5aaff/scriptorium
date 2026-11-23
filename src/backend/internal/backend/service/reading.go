package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"scriptorium/internal/backend/auth"
	"scriptorium/internal/backend/dao"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type readingRepository interface {
	ReadReadingState(context.Context, string, string) (dao.ReadingState, error)
	SaveReadingState(context.Context, string, string, string, int64, dao.ReadingPosition) (dao.ReadingState, error)
}

// ReadingState loads or saves the authenticated account's private place. The
// session middleware supplies the principal and enforces CSRF on PUT; catalogue
// edit permission is deliberately not required for personal reader state.
func (f *FileHandler) ReadingState(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	id, err := uuid.Parse(c.Param("uuid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid document UUID"})
		return
	}
	repository, ok := f.APIHandler.DaoService.dao.(readingRepository)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "reading state is unavailable with this catalogue"})
		return
	}
	var state dao.ReadingState
	if c.Request.Method == http.MethodGet {
		state, err = repository.ReadReadingState(c.Request.Context(), user.ID, id.String())
	} else {
		var request struct {
			SourceRevision  string              `json:"source_revision"`
			ExpectedVersion int64               `json:"expected_version"`
			Position        dao.ReadingPosition `json:"position"`
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
		decoder := json.NewDecoder(c.Request.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF || request.ExpectedVersion < 0 || len(request.SourceRevision) > 256 || request.SourceRevision == "" || request.Position.Validate() != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid reading position"})
			return
		}
		state, err = repository.SaveReadingState(c.Request.Context(), user.ID, id.String(), request.SourceRevision, request.ExpectedVersion, request.Position)
	}
	if err == nil {
		c.JSON(http.StatusOK, state)
		return
	}
	status, message := http.StatusInternalServerError, "could not access saved reading position"
	switch {
	case errors.Is(err, dao.ErrDocumentNotFound):
		status, message = http.StatusNotFound, "document not found"
	case errors.Is(err, dao.ErrDocumentChanged):
		status, message = http.StatusConflict, "the original changed; reopen the reader before saving a place"
	case errors.Is(err, dao.ErrReadingConflict):
		status, message = http.StatusConflict, dao.ErrReadingConflict.Error()
	}
	c.JSON(status, gin.H{"error": message})
}
