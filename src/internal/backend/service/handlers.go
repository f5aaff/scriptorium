package service

import (
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "scriptorium/internal/backend/dao"
    "strings"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
)

//---------------------------------------------------
//-------------------API-HANDLER---------------------
//---------------------------------------------------

type Handler interface {
    GetRouterGroups() (string, map[string]gin.HandlerFunc)
    GetService() any
}

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

type RequestBody struct {
    Udid string `json:"udid"`
}

func (h *APIHandler) Read(c *gin.Context) {
    var req RequestBody

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Missing or invalid 'UDID' parameter."})
        return
    }

    uuid, err := uuid.Parse(req.Udid)
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
        return
    }

    err := h.DaoService.Update(document)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
    }

    c.JSON(http.StatusOK, gin.H{"message": "update successful", "value": document.GetID()})
}

func (h *APIHandler) Delete(c *gin.Context) {

    // instantiate RequestBody var
    var req RequestBody
    // bind gin.Context to readRequestBody
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Missing or invalid 'UDID' parameter."})
    }

    // parse a uuid from the request body
    uuid, err := uuid.Parse(req.Udid)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
    }

    // delete the record via the DaoService
    err = h.DaoService.Delete(uuid)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
    }
    c.JSON(http.StatusBadRequest, gin.H{"message": "record deleted successfully", "value": req.Udid})
}

func (h *APIHandler) GetRouterGroups() (string, map[string]gin.HandlerFunc) {
    groupName := "/data"

    // Define the routes and corresponding handlers
    routes := map[string]gin.HandlerFunc{
        "POST /create": h.Create,
        "POST /read":   h.Read,
        "PUT /update":  h.Update,
        "GET /search":  h.SearchByKeyValue,
    }

    return groupName, routes
}

func StartRestAPI(handlers ...Handler) <-chan error {
    errCh := make(chan error, 1) // Buffered channel to capture errors

    go func() {
        r := gin.Default()

        for _, handler := range handlers {
            path, routes := handler.GetRouterGroups()
            group := r.Group(path) // Create a RouterGroup dynamically

            for route, fn := range routes {
                parts := strings.Split(route, " ") // Extract method and route path
                if len(parts) != 2 {
                    log.Printf("Invalid route format: %s", route)
                    errCh <- fmt.Errorf("Invalid route format: %s", route)
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
                    log.Printf("Unsupported method: %s", method)
                    errCh <- fmt.Errorf("Unsupported method: %s", method)
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
