package service

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net"
    "net/http"
    "path/filepath"
    "scriptorium/internal/backend/dao"
    "scriptorium/internal/backend/fao"
    "scriptorium/internal/backend/service/pb"
    "strconv"
    "strings"

    "github.com/gin-contrib/cors"
    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "google.golang.org/grpc"
)

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
    FaoService        FileHandlerService
    FileServiceClient pb.FileServiceClient
    APIHandler        *APIHandler
}

func (f *FileHandler) GetService() any {
    return f.FaoService
}

func NewFileHandler(faos FileHandlerService, conn *grpc.ClientConn, apiHandler *APIHandler) *FileHandler {
    return &FileHandler{
        FaoService:        faos,
        FileServiceClient: pb.NewFileServiceClient(conn),
        APIHandler:        apiHandler,
    }
}

// UploadFile handles file uploads with optional metadata for database record creation.
//
// Expected form data:
//   - "file": The file to upload
//   - "metadata" (optional): JSON string containing document metadata
//     Example metadata:
//     {
//     "DocType": "Notes",
//     "Title": "My Document",
//     "Author": "John Doe",
//     "PublishDate": "2024-01-01"
//     }
//
// The function will:
// 1. Generate a unique filename for the uploaded file
// 2. Save the file to the server's storage location
// 3. If metadata is provided, create a database record with the file path
// 4. Return the generated file path and document UUID
func (f FileHandler) UploadFile(c *gin.Context) {
    file, header, err := c.Request.FormFile("file")
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file Upload", "output": err.Error()})
        return
    }

    defer file.Close()

    // Parse metadata from form data
    var metadata map[string]any
    if metadataStr := c.PostForm("metadata"); metadataStr != "" {
        if err := json.Unmarshal([]byte(metadataStr), &metadata); err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid metadata format", "output": err.Error()})
            return
        }
    }

    // Generate unique filename to avoid conflicts
    fileExt := filepath.Ext(header.Filename)
    uniqueFilename := uuid.New().String() + fileExt
    filePath := uniqueFilename

    stream, err := f.FileServiceClient.UploadFile(context.Background())
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create upload stream", "output": err.Error()})
        return
    }

    buf := make([]byte, 4096)
    firstChunk := true
    for {
        n, err := file.Read(buf)
        if err == io.EOF {
            break
        }
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "error reading file", "output": err.Error()})
            return
        }

        chunk := &pb.FileChunk{Data: buf[:n]}
        if firstChunk {
            chunk.Filename = filePath // Use the generated path instead of original filename
            firstChunk = false
        }

        if err := stream.Send(chunk); err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to send file chunk", "output": err.Error()})
            return
        }
    }
    // Close the stream and get the response
    resp, err := stream.CloseAndRecv()
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Upload failed"})
        return
    }

    // Create database record with the file path
    if len(metadata) > 0 {
        // Add the file path to metadata
        metadata["Path"] = filePath
        metadata["FileType"] = fileExt

        // Create document record
        docType, ok := metadata["DocType"].(string)
        if !ok || docType == "" {
            c.JSON(http.StatusBadRequest, gin.H{"error": "Missing or invalid document type in metadata"})
            return
        }

        // Use the API handler's document factory to create the document
        doc, err := f.APIHandler.DocumentFactory.NewDocument(docType)
        if err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown document type"})
            return
        }

        // Set metadata
        meta := doc.GetMetaData()
        meta.Uuid = uuid.New().String()
        meta.DocType = docType
        meta.Path = filePath
        meta.FileType = fileExt
        if title, ok := metadata["Title"].(string); ok {
            meta.Title = title
        }
        if author, ok := metadata["Author"].(string); ok {
            meta.Author = author
        }
        if publishDate, ok := metadata["PublishDate"].(string); ok {
            meta.PublishDate = publishDate
        }

        err = doc.SetMetaData(meta)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to set metadata"})
            return
        }

        // Set other document fields
        docJSON, _ := json.Marshal(metadata)
        if err := json.Unmarshal(docJSON, &doc); err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode document"})
            return
        }

        // Save to database
        err = f.APIHandler.DaoService.Create(doc)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create database record", "output": err.Error()})
            return
        }

        // Respond with success message and document info
        c.JSON(http.StatusOK, gin.H{
            "message":           resp.Message,
            "file_path":         filePath,
            "document_uuid":     doc.GetID(),
            "original_filename": header.Filename,
        })
    } else {
        // Just file upload without database record
        c.JSON(http.StatusOK, gin.H{
            "message":           resp.Message,
            "file_path":         filePath,
            "original_filename": header.Filename,
        })
    }
}

func (f FileHandler) DownloadFile(c *gin.Context) {
    uuidStr := c.Param("uuid")
    if uuidStr == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Missing UUID parameter"})
        return
    }

    // Parse UUID
    uuid, err := uuid.Parse(uuidStr)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID format"})
        return
    }

    // Get document metadata from database
    rawData, err := f.APIHandler.DaoService.ReadRaw(uuid)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Document not found"})
        return
    }

    // Parse metadata to get file path
    var metadata dao.MetaData
    if err := json.Unmarshal(rawData, &metadata); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse document metadata"})
        return
    }

    if metadata.Path == "" {
        c.JSON(http.StatusNotFound, gin.H{"error": "File path not found in document metadata"})
        return
    }

    // Call gRPC DownloadFile method with the file path from database
    stream, err := f.FileServiceClient.DownloadFile(context.Background(), &pb.FileRequest{Filename: metadata.Path})
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to download file"})
        return
    }

    // Set response headers with original filename if available
    downloadFilename := metadata.Title
    if downloadFilename == "" {
        downloadFilename = metadata.Path // fallback to UUID filename
    }
    c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", downloadFilename))
    c.Header("Content-Type", "application/octet-stream")

    // Stream file chunks
    for {
        chunk, err := stream.Recv()
        if err == io.EOF {
            break
        }
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Error receiving file chunk", "output": err.Error()})
            return
        }

        // Write chunk to HTTP response
        _, err = c.Writer.Write(chunk.Data)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write file chunk", "output": err.Error()})
            return
        }
    }
}
func (f *FileHandler) GetRouterGroups() (string, map[string]gin.HandlerFunc) {
    groupName := "/file"

    // Define the routes and corresponding handlers
    routes := map[string]gin.HandlerFunc{
        "POST /upload":        f.UploadFile,
        "GET /download/:uuid": f.DownloadFile,
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
    // Get query parameters
    key := c.Query("key")
    value := c.Query("value")
    pageStr := c.DefaultQuery("page", "1")
    limitStr := c.DefaultQuery("limit", "10")

    // Parse pagination parameters
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

    allResults, err := h.DaoService.SearchByKeyValue(key, value)
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
    c.Header("Access-Control-Allow-Origin", "*")
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

    meta := doc.GetMetaData()
    meta.Uuid = uuid.New().String()
    meta.DocType = docType
    err = doc.SetMetaData(meta)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to set metadata"})
        return
    }
    docJSON, _ := json.Marshal(reqData)
    if err := json.Unmarshal(docJSON, &doc); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to decode document"})
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

    rawData, err := h.DaoService.ReadRaw(uuid) // Modify DAO to return raw JSON
    fmt.Println(string(rawData))
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"message": "document retrieved", "value": string(rawData)})
}

func (h *APIHandler) Update(c *gin.Context) {

    var document dao.Document

    if err := c.ShouldBindJSON(&document); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
    }

    err := h.DaoService.Update(document)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"message": "update successful", "value": document.GetID()})
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

func (h *APIHandler) GetRouterGroups() (string, map[string]gin.HandlerFunc) {
    groupName := "/data"

    // Define the routes and corresponding handlers
    routes := map[string]gin.HandlerFunc{
        "POST /create":    h.Create,
        "GET /read/:uuid": h.Read,
        "PUT /update":     h.Update,
        "GET /search":     h.SearchByKeyValue,
        "DELETE /delete":  h.Delete,
    }

    return groupName, routes
}

func StartGrcpService(grpcServer *grpc.Server, fileHandlerService FileHandlerService) <-chan error {
    errCh := make(chan error, 1)
    go func() {
        lis, err := net.Listen("tcp", ":5001")
        if err != nil {
            errCh <- err
            log.Fatalf("failed to listen: %v", err)
        }

        pb.RegisterFileServiceServer(grpcServer, fileHandlerService)

        log.Println("gRPC server listening on :5001")
        if err := grpcServer.Serve(lis); err != nil {
            errCh <- err
        }
    }()
    return errCh
}

func StartRestAPI(handlers ...Handler) <-chan error {
    errCh := make(chan error, 1) // Buffered channel to capture errors

    go func() {
        r := gin.Default()
        r.Use(cors.Default())
        for _, handler := range handlers {
            path, routes := handler.GetRouterGroups()
            group := r.Group(path) // Create a RouterGroup dynamically

            for route, fn := range routes {
                parts := strings.Split(route, " ") // Extract method and route path
                if len(parts) != 2 {
                    log.Printf("invalid route format: %s", route)
                    errCh <- fmt.Errorf("invalid route format: %s", route)
                    return
                }
                method, endpoint := parts[0], parts[1]

                // Register route dynamically based on method
                switch method {
                case "GET":
                    group.GET(endpoint, fn)
                case "POST":
                    group.POST(endpoint, fn)
                case "PUT":
                    group.PUT(endpoint, fn)
                case "DELETE":
                    group.DELETE(endpoint, fn)
                default:
                    log.Printf("unsupported method: %s", method)
                    errCh <- fmt.Errorf("unsupported method: %s", method)
                    return
                }
            }
        }

        // Start the server and capture any errors
        if err := r.Run(":8080"); err != nil {
            errCh <- err
        }
    }()

    return errCh // Return the error channel to listen for errors
}
