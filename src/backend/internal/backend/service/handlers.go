package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"scriptorium/internal/backend/auth"
	"scriptorium/internal/backend/classification"
	"scriptorium/internal/backend/converter"
	"scriptorium/internal/backend/dao"
	"scriptorium/internal/backend/fao"
	"scriptorium/internal/backend/formats"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const maxFileSize = 100 * 1024 * 1024 // 100MB

//---------------------------------------------------
//---------------------HANDLER-----------------------
//---------------------------------------------------

type Handler interface {
	GetRouterGroups() (string, map[string]gin.HandlerFunc)
	GetService() any
}

//---------------------------------------------------
//-------------------FILE-HANDLER--------------------
//---------------------------------------------------

type FileHandler struct {
	FaoService fao.FAO
	APIHandler *APIHandler
	Converter  converter.Converter
}

func (f *FileHandler) GetService() any {
	return f.FaoService
}

func NewFileHandler(storage fao.FAO, apiHandler *APIHandler, conv converter.Converter) *FileHandler {
	return &FileHandler{FaoService: storage, APIHandler: apiHandler, Converter: conv}
}

// UploadFile preserves and catalogues every uploaded file, including unknown formats.
func (f FileHandler) UploadFile(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxFileSize)
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "file exceeds the 100 MiB upload limit"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file upload", "details": err.Error()})
		return
	}
	defer file.Close()

	var supplied map[string]any
	if raw := c.PostForm("metadata"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &supplied); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid metadata JSON", "details": err.Error()})
			return
		}
	}

	// Peek at the content for MIME identification, then replay those bytes into
	// storage. The extension is descriptive only; it is not an allowlist.
	prefix := make([]byte, 512)
	n, readErr := io.ReadFull(file, prefix)
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not inspect uploaded file"})
		return
	}
	prefix = prefix[:n]
	identification, err := formats.Identify(header.Filename, bytes.NewReader(prefix))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not identify uploaded file", "details": err.Error()})
		return
	}
	if identification.MediaType == "application/octet-stream" {
		if byExt := mime.TypeByExtension(identification.Extension); byExt != "" {
			identification.MediaType = byExt
		}
	}

	id := uuid.New()
	storagePath := id.String()
	hasher := sha256.New()
	content := io.MultiReader(bytes.NewReader(prefix), file)
	if err := f.FaoService.SaveFile(storagePath, io.TeeReader(content, hasher)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store uploaded file", "details": err.Error()})
		return
	}

	docType := "File"
	if value, ok := supplied["DocType"].(string); ok && strings.TrimSpace(value) != "" {
		docType = value
	}
	doc, err := f.APIHandler.DocumentFactory.NewDocument(docType)
	if err != nil {
		_ = f.FaoService.DeleteFile(storagePath)
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown document type", "details": docType})
		return
	}

	baseName := filepath.Base(header.Filename)
	title := strings.TrimSuffix(baseName, filepath.Ext(baseName))
	meta := dao.MetaData{
		Uuid:                 id.String(),
		Title:                title,
		DocType:              docType,
		FileType:             identification.Extension,
		Path:                 storagePath,
		OriginalFilename:     baseName,
		MediaType:            identification.MediaType,
		Size:                 header.Size,
		SHA256:               hex.EncodeToString(hasher.Sum(nil)),
		ClassificationStatus: "unclassified",
		LastUpdated:          time.Now().UTC().Format(time.RFC3339),
	}
	if value, ok := supplied["Title"].(string); ok && strings.TrimSpace(value) != "" {
		meta.Title = value
	}
	if value, ok := supplied["Author"].(string); ok {
		meta.Author = value
	}
	if value, ok := supplied["PublishDate"].(string); ok {
		meta.PublishDate = value
	}
	if value, ok := supplied["DeweyDecimal"].(string); ok && value != "" {
		meta.DeweyDecimal = value
		meta.ClassificationStatus = "confirmed"
	}
	if meta.DeweyDecimal == "" {
		content, _ := supplied["Content"].(string)
		suggestions := classification.Suggest(meta.Title+" "+meta.Author+" "+content, 1)
		if len(suggestions) > 0 {
			meta.SuggestedDeweyDecimal = suggestions[0].Code
			meta.SuggestedDeweyName = suggestions[0].Name
			meta.ClassificationConfidence = suggestions[0].Confidence
			meta.ClassificationEvidence = suggestions[0].Evidence
			meta.ClassificationStatus = "suggested"
		}
	}
	if err := doc.SetMetaData(meta); err != nil {
		_ = f.FaoService.DeleteFile(storagePath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to initialise document metadata"})
		return
	}
	if err := f.APIHandler.DaoService.Create(doc); err != nil {
		_ = f.FaoService.DeleteFile(storagePath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to catalogue uploaded file", "details": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"message":           "file imported and catalogued",
		"file_path":         storagePath,
		"document_uuid":     id.String(),
		"original_filename": baseName,
		"media_type":        identification.MediaType,
		"kind":              identification.Kind,
		"sha256":            meta.SHA256,
	})
}

func (f FileHandler) DownloadFile(c *gin.Context) {
	id, err := uuid.Parse(c.Param("uuid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID"})
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
		c.JSON(http.StatusNotFound, gin.H{"error": "stored file not found"})
		return
	}
	defer file.Close()
	filename := metadata.OriginalFilename
	if filename == "" {
		filename = metadata.Title
	}
	if filename == "" {
		filename = id.String()
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", url.PathEscape(filename)))
	contentType := metadata.MediaType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Header("Content-Type", contentType)
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, file); err != nil {
		log.Printf("download stream failed for %s: %v", id, err)
	}
}

func (f FileHandler) ConvertFile(c *gin.Context) {
	uuidStr := c.Param("uuid")
	if uuidStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing UUID parameter"})
		return
	}

	format := c.DefaultQuery("format", "pdf")

	parsedUUID, err := uuid.Parse(uuidStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID format"})
		return
	}

	rawData, err := f.APIHandler.DaoService.ReadRaw(parsedUUID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Document not found"})
		return
	}

	var metadata dao.MetaData
	if err := json.Unmarshal(rawData, &metadata); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse document metadata"})
		return
	}

	if metadata.Path == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "File path not found in document metadata"})
		return
	}

	outputPath, err := f.Converter.ConvertDocumentByUUID(uuidStr, strings.TrimPrefix(metadata.FileType, "."), format)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Conversion failed", "output": err.Error()})
		return
	}

	file, err := f.APIHandler.FaoService.GetFile(outputPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve converted file"})
		return
	}
	defer file.Close()

	contentType := "application/octet-stream"
	switch format {
	case "pdf":
		contentType = "application/pdf"
	case "html":
		contentType = "text/html"
	case "docx":
		contentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	}

	downloadName := metadata.Title
	if downloadName == "" {
		downloadName = "converted"
	}
	downloadName += "." + format

	c.Header("Content-Type", contentType)
	disposition := mime.FormatMediaType("inline", map[string]string{"filename": downloadName})
	if disposition == "" {
		disposition = "inline"
	}
	c.Header("Content-Disposition", disposition)
	io.Copy(c.Writer, file)
}

func (f *FileHandler) GetRouterGroups() (string, map[string]gin.HandlerFunc) {
	groupName := "/file"

	routes := map[string]gin.HandlerFunc{
		"POST /upload":        f.UploadFile,
		"GET /download/:uuid": f.DownloadFile,
		"POST /convert/:uuid": f.ConvertFile,
	}

	return groupName, routes
}

//---------------------------------------------------
//-------------------API-HANDLER---------------------
//---------------------------------------------------

type APIHandler struct {
	DaoService      DaoService
	DocumentFactory *dao.DocumentFactory
	FaoService      fao.FAO
}

func (h *APIHandler) GetService() any {
	return h.DaoService
}

func NewAPIHandler(daos DaoService, documentFactory *dao.DocumentFactory, faoService fao.FAO) *APIHandler {
	return &APIHandler{DaoService: daos, DocumentFactory: documentFactory, FaoService: faoService}
}

func (h *APIHandler) SearchByKeyValue(c *gin.Context) {
	query := c.Query("q")
	key := c.Query("key")
	value := c.Query("value")
	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "10")

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid page parameter. Must be a positive integer."})
		return
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid limit parameter. Must be between 1 and 100."})
		return
	}

	var results []dao.MetaData
	var totalCount int

	var allResults []dao.MetaData
	if query != "" {
		allResults, err = h.DaoService.FuzzySearch(query)
	} else {
		allResults, err = h.DaoService.SearchByKeyValue(key, value)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	totalCount = len(allResults)

	// Apply pagination
	start := (page - 1) * limit
	end := start + limit

	if start >= totalCount {
		// Page is beyond available data
		results = []dao.MetaData{}
	} else if end > totalCount {
		// Last page
		results = allResults[start:totalCount]
	} else {
		// Regular page
		results = allResults[start:end]
	}

	// Calculate pagination info
	totalPages := (totalCount + limit - 1) / limit // Ceiling division
	hasNext := page < totalPages
	hasPrev := page > 1

	c.JSON(http.StatusOK, gin.H{
		"message":     "Search completed",
		"count":       len(results),
		"total_count": totalCount,
		"page":        page,
		"limit":       limit,
		"total_pages": totalPages,
		"has_next":    hasNext,
		"has_prev":    hasPrev,
		"results":     results,
	})
}

func (h *APIHandler) Create(c *gin.Context) {

	var reqData map[string]any

	if err := c.ShouldBindJSON(&reqData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	docType, ok := reqData["DocType"].(string)
	if !ok || docType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing or invalid document type"})
		return
	}

	doc, err := h.DocumentFactory.NewDocument(docType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown document type"})
		return
	}

	docJSON, _ := json.Marshal(reqData)
	if err := json.Unmarshal(docJSON, &doc); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to decode document"})
		return
	}

	meta := doc.GetMetaData()
	meta.Uuid = uuid.New().String()
	meta.DocType = docType
	if title, ok := reqData["Title"].(string); ok {
		meta.Title = title
	}
	if author, ok := reqData["Author"].(string); ok {
		meta.Author = author
	}
	if publishDate, ok := reqData["PublishDate"].(string); ok {
		meta.PublishDate = publishDate
	}
	if dewey, ok := reqData["DeweyDecimal"].(string); ok {
		meta.DeweyDecimal = dewey
	}
	err = doc.SetMetaData(meta)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to set metadata"})
		return
	}

	err = h.DaoService.Create(doc)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Document inserted into DB", "UUID": doc.GetID()})
}

func (h *APIHandler) Read(c *gin.Context) {
	// Get UUID from URL parameter
	uuidStr := c.Param("uuid")
	if uuidStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing UUID parameter"})
		return
	}

	uuid, err := uuid.Parse(uuidStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rawData, err := h.DaoService.ReadRaw(uuid)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "document retrieved", "value": string(rawData)})
}

func (h *APIHandler) Update(c *gin.Context) {
	var request struct {
		UUID        string  `json:"Uuid"`
		DocType     string  `json:"DocType"`
		Title       *string `json:"Title"`
		Author      *string `json:"Author"`
		PublishDate *string `json:"PublishDate"`
		Dewey       *string `json:"DeweyDecimal"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid update request", "details": err.Error()})
		return
	}
	id, err := uuid.Parse(request.UUID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing or invalid UUID"})
		return
	}
	if request.DocType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "document type is required"})
		return
	}
	raw, err := h.DaoService.ReadRaw(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "document not found"})
		return
	}
	var metadata dao.MetaData
	if err := json.Unmarshal(raw, &metadata); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "stored metadata is invalid"})
		return
	}
	doc, err := h.DocumentFactory.NewDocument(request.DocType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown document type"})
		return
	}
	metadata.DocType = request.DocType
	metadata.LastUpdated = time.Now().UTC().Format(time.RFC3339)
	if request.Title != nil {
		metadata.Title = strings.TrimSpace(*request.Title)
	}
	if request.Author != nil {
		metadata.Author = strings.TrimSpace(*request.Author)
	}
	if request.PublishDate != nil {
		metadata.PublishDate = *request.PublishDate
	}
	if request.Dewey != nil {
		metadata.DeweyDecimal = strings.TrimSpace(*request.Dewey)
		if metadata.DeweyDecimal != "" {
			metadata.ClassificationStatus = "confirmed"
		} else if metadata.SuggestedDeweyDecimal != "" {
			metadata.ClassificationStatus = "suggested"
		} else {
			metadata.ClassificationStatus = "unclassified"
		}
	}
	if err := doc.SetMetaData(metadata); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update metadata"})
		return
	}
	if err := h.DaoService.Update(doc); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "update successful", "value": id.String()})
}

func (h *APIHandler) Delete(c *gin.Context) {

	// Parse JSON body to get UUIDs
	var req struct {
		Uuids []string `json:"uuids"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing or invalid 'uuids' parameter."})
		return
	}

	if len(req.Uuids) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "At least one UUID is required"})
		return
	}

	var deletedUUIDs []string
	var errors []string

	// Delete each UUID
	for _, uuidStr := range req.Uuids {
		uuid, err := uuid.Parse(uuidStr)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Invalid UUID '%s': %s", uuidStr, err.Error()))
			continue
		}

		// Get the document metadata to find the file path before deleting the record
		rawData, err := h.DaoService.ReadRaw(uuid)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Failed to read document metadata for UUID '%s': %s", uuidStr, err.Error()))
			continue
		}

		// Parse metadata to get file path
		var metadata dao.MetaData
		if err := json.Unmarshal(rawData, &metadata); err != nil {
			errors = append(errors, fmt.Sprintf("Failed to parse document metadata for UUID '%s': %s", uuidStr, err.Error()))
			continue
		}

		// Delete the physical file if path exists
		if metadata.Path != "" {
			if err := h.FaoService.DeleteFile(metadata.Path); err != nil {
				// Log the file deletion error but continue with database deletion
				errors = append(errors, fmt.Sprintf("Failed to delete file '%s' for UUID '%s': %s", metadata.Path, uuidStr, err.Error()))
				// Don't continue here - we still want to delete the database record
			}
		}

		// delete the record via the DaoService
		err = h.DaoService.Delete(uuid)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Failed to delete UUID '%s': %s", uuidStr, err.Error()))
			continue
		}

		deletedUUIDs = append(deletedUUIDs, uuidStr)
	}

	// Prepare response
	response := gin.H{
		"deleted_count": len(deletedUUIDs),
		"deleted_uuids": deletedUUIDs,
	}

	if len(errors) > 0 {
		response["errors"] = errors
		response["error_count"] = len(errors)
	}

	if len(deletedUUIDs) > 0 {
		c.JSON(http.StatusOK, response)
	} else {
		c.JSON(http.StatusBadRequest, response)
	}
}

func (h *APIHandler) GetDocumentTypes(c *gin.Context) {
	types := h.DocumentFactory.GetRegisteredTypes()
	c.JSON(http.StatusOK, gin.H{"types": types})
}

func (h *APIHandler) GetDeweyCategories(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"categories": dao.DeweyCategories})
}

func (h *APIHandler) SuggestClassification(c *gin.Context) {
	var request struct {
		Title   string `json:"title"`
		Author  string `json:"author"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid classification request", "details": err.Error()})
		return
	}
	text := strings.Join([]string{request.Title, request.Author, request.Content}, " ")
	c.JSON(http.StatusOK, gin.H{"suggestions": classification.Suggest(text, 5)})
}

func (h *APIHandler) GetRouterGroups() (string, map[string]gin.HandlerFunc) {
	groupName := "/data"

	routes := map[string]gin.HandlerFunc{
		"POST /create":    h.Create,
		"GET /read/:uuid": h.Read,
		"PUT /update":     h.Update,
		"GET /search":     h.SearchByKeyValue,
		"DELETE /delete":  h.Delete,
		"GET /types":      h.GetDocumentTypes,
		"GET /dewey":      h.GetDeweyCategories,
		"POST /classify":  h.SuggestClassification,
	}

	return groupName, routes
}

func securityHeaders(allowedOrigins []string) gin.HandlerFunc {
	connectSources := []string{"'self'"}
	for _, origin := range allowedOrigins {
		connectSources = append(connectSources, origin)
	}
	contentSecurityPolicy := "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; media-src 'self' blob:; connect-src " + strings.Join(connectSources, " ") + "; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		c.Header("Content-Security-Policy", contentSecurityPolicy)
		if strings.HasPrefix(c.Request.URL.Path, "/auth/") || strings.HasPrefix(c.Request.URL.Path, "/data/") || strings.HasPrefix(c.Request.URL.Path, "/file/") {
			c.Header("Cache-Control", "no-store")
		}
		c.Next()
	}
}

func NewHTTPServer(host string, port int, webRoot string, authManager *auth.Manager, allowedOrigins, trustedProxies []string, handlers ...Handler) (*http.Server, error) {
	if authManager == nil {
		return nil, fmt.Errorf("authentication manager is required")
	}
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), securityHeaders(allowedOrigins))
	// Forwarded client addresses are trusted only from explicitly configured
	// proxy addresses. The reverse proxy must strip client-supplied headers.
	if err := r.SetTrustedProxies(trustedProxies); err != nil {
		return nil, fmt.Errorf("disable implicit trusted proxies: %w", err)
	}
	if len(allowedOrigins) > 0 {
		r.Use(cors.New(cors.Config{
			AllowOrigins:     allowedOrigins,
			AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
			AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-CSRF-Token"},
			AllowCredentials: true,
			MaxAge:           12 * time.Hour,
		}))
	}
	authHandlers := &auth.Handlers{Manager: authManager}
	r.POST("/auth/login", authHandlers.Login)
	authGroup := r.Group("/auth", authHandlers.RequireSession())
	authGroup.GET("/me", authHandlers.Me)
	authGroup.POST("/logout", authHandlers.Logout)
	authGroup.POST("/password", authHandlers.ChangePassword)
	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	for _, handler := range handlers {
		path, routes := handler.GetRouterGroups()
		group := r.Group(path, authHandlers.RequireSession())
		for route, fn := range routes {
			parts := strings.Split(route, " ")
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid route format: %s", route)
			}
			method, endpoint := parts[0], parts[1]
			switch method {
			case http.MethodGet:
				group.GET(endpoint, fn)
			case http.MethodPost:
				group.POST(endpoint, fn)
			case http.MethodPut:
				group.PUT(endpoint, fn)
			case http.MethodDelete:
				group.DELETE(endpoint, fn)
			default:
				return nil, fmt.Errorf("unsupported HTTP method %q for route %s", method, route)
			}
		}
	}

	if info, err := os.Stat(webRoot); err == nil && info.IsDir() {
		root, absErr := filepath.Abs(webRoot)
		if absErr == nil {
			root, absErr = filepath.EvalSymlinks(root)
		}
		if absErr == nil {
			indexPath := filepath.Join(root, "index.html")
			if indexInfo, indexErr := os.Stat(indexPath); indexErr == nil && indexInfo.Mode().IsRegular() {
				r.NoRoute(func(c *gin.Context) {
					if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
						c.Status(http.StatusNotFound)
						return
					}
					relative := strings.TrimPrefix(filepath.Clean("/"+c.Request.URL.Path), string(filepath.Separator))
					if relative != "" && filepath.IsLocal(relative) {
						candidate := filepath.Join(root, relative)
						resolved, resolveErr := filepath.EvalSymlinks(candidate)
						if resolveErr == nil {
							withinRoot, relErr := filepath.Rel(root, resolved)
							fileInfo, statErr := os.Stat(resolved)
							if relErr == nil && withinRoot != ".." && !strings.HasPrefix(withinRoot, ".."+string(filepath.Separator)) && statErr == nil && fileInfo.Mode().IsRegular() {
								c.File(resolved)
								return
							}
						}
					}
					c.File(indexPath)
				})
			}
		}
	}

	return &http.Server{
		Addr:              net.JoinHostPort(host, strconv.Itoa(port)),
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}, nil
}
