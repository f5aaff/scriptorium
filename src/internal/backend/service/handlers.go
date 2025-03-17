package service

import (
    "encoding/json"
    "net/http"
    "scriptorium/internal/backend/dao"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
)

//---------------------------------------------------
//-------------------API-HANDLER---------------------
//---------------------------------------------------

type APIHandler struct {
    DaoService DaoService
}

func NewAPIHandler(daos DaoService) *APIHandler {
    return &APIHandler{DaoService: daos}
}

func (h *APIHandler) SearchByKeyValue(c *gin.Context) {
}

func (h *APIHandler) Create(c *gin.Context) {

    var document dao.Document

    if err := c.ShouldBindJSON(&document); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    err := h.DaoService.Create(document)
    if err != nil {

        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"message": "Document inserted into DB", "UUID": document.GetID().String()})
}

type RequestBody struct {
    Udid string `json:"udid"`
}

func (h *APIHandler) Read(c *gin.Context) {

    // instantiate RequestBody var
    var req RequestBody
    // bind gin.Context to readRequestBody
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Missing or invalid 'UDID' parameter."})
    }

    // instantiate a document var
    var doc dao.Document

    // parse a uuid from the request body
    uuid, err := uuid.Parse(req.Udid)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
    }

    // read the document from the daoService
    res, err := h.DaoService.Read(&doc, uuid)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
    }

    // marshall the result to json, send to consumer
    resJson, err := json.Marshal(res)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
    }
    c.JSON(http.StatusOK, gin.H{"message": "document", "value": string(resJson)})
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

    c.JSON(http.StatusOK, gin.H{"message": "update successful", "value": document.GetID().String()})
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
