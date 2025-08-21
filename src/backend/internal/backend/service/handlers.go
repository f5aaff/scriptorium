package service

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net"
    "net/http"
    "scriptorium/internal/backend/dao"
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
}

func (f *FileHandler) GetService() any {
    return f.FaoService
}

func NewFileHandler(faos FileHandlerService, conn *grpc.ClientConn) *FileHandler {
    return &FileHandler{FaoService: faos, FileServiceClient: pb.NewFileServiceClient(conn)}
}

func (f FileHandler) UploadFile(c *gin.Context) {
    file, _, err := c.Request.FormFile("file")
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file Upload", "output": err.Error()})
        return
    }

    defer file.Close()

    stream, err := f.FileServiceClient.UploadFile(context.Background())
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create upload stream", "output": err.Error()})
        return
    }

    buf := make([]byte, 4096)
    for {
        n, err := file.Read(buf)
        if err == io.EOF {
            break
        }
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "error reading file", "output": err.Error()})
            return
        }

        if err := stream.Send(&pb.FileChunk{Data: buf[:n]}); err != nil {
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

    // Respond with success message
    c.JSON(http.StatusOK, gin.H{"message": resp.Message})
}

func (f FileHandler) DownloadFile(c *gin.Context) {
    filename := c.Param("filename")

    // Call gRPC DownloadFile method
    stream, err := f.FileServiceClient.DownloadFile(context.Background(), &pb.FileRequest{Filename: filename})
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to download file"})
        return
    }

    // Set response headers
    c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
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
        "POST /upload":           f.UploadFile,
        "GET /download:filename": f.DownloadFile,
    }

    return groupName, routes
}

//---------------------------------------------------
//-------------------API-HANDLER---------------------
//---------------------------------------------------

type APIHandler struct {
    DaoService      DaoService
    DocumentFactory *dao.DocumentFactory
}

func (h *APIHandler) GetService() any {
    return h.DaoService
}

func NewAPIHandler(daos DaoService, documentFactory *dao.DocumentFactory) *APIHandler {
    return &APIHandler{DaoService: daos, DocumentFactory: documentFactory}
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
