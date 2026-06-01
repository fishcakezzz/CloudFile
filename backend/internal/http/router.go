package http

import (
	nethttp "net/http"
	"net/url"
	"strconv"

	"cloudfile/backend/internal/service"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *service.Service
}

func NewRouter(svc *service.Service) *gin.Engine {
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "http://127.0.0.1:5173")
		c.Header("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == nethttp.MethodOptions {
			c.AbortWithStatus(nethttp.StatusNoContent)
			return
		}
		c.Next()
	})
	handler := &Handler{service: svc}

	router.GET("/health", func(c *gin.Context) { c.JSON(nethttp.StatusOK, gin.H{"status": "ok"}) })

	api := router.Group("/api")
	api.POST("/uploads/init", handler.initUpload)
	api.GET("/uploads/:upload_id/status", handler.uploadStatus)
	api.POST("/uploads/:upload_id/chunks/:chunk_index", handler.uploadChunk)
	api.POST("/uploads/:upload_id/merge", handler.mergeUpload)
	api.GET("/files", handler.listFiles)
	api.GET("/files/:file_id/download", handler.downloadFile)
	api.DELETE("/files/:file_id", handler.deleteFile)
	api.GET("/media-tasks/:task_id", handler.mediaTask)
	api.POST("/worker/media-tasks/:task_id/run", handler.runMediaTask)
	api.POST("/admin/compensate", handler.compensate)

	return router
}

func (h *Handler) initUpload(c *gin.Context) {
	var req service.InitUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(nethttp.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	result, err := h.service.InitUpload(c.Request.Context(), req)
	if err != nil {
		c.JSON(nethttp.StatusInternalServerError, gin.H{"detail": err.Error()})
		return
	}
	c.JSON(nethttp.StatusOK, result)
}

func (h *Handler) uploadStatus(c *gin.Context) {
	uploadID, ok := uintParam(c, "upload_id")
	if !ok {
		return
	}
	task, chunks, err := h.service.UploadStatus(uploadID, c.Query("user_id"))
	if err != nil {
		c.JSON(nethttp.StatusNotFound, gin.H{"detail": "upload task not found"})
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{
		"upload_id": task.ID, "status": task.Status, "uploaded_count": task.UploadedCount,
		"chunk_count": task.ChunkCount, "uploaded_chunks": chunks,
	})
}

func (h *Handler) uploadChunk(c *gin.Context) {
	uploadID, ok := uintParam(c, "upload_id")
	if !ok {
		return
	}
	chunkIndex64, err := strconv.ParseInt(c.Param("chunk_index"), 10, 64)
	if err != nil {
		c.JSON(nethttp.StatusBadRequest, gin.H{"detail": "invalid chunk_index"})
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(nethttp.StatusBadRequest, gin.H{"detail": "file is required"})
		return
	}
	task, already, err := h.service.UploadChunk(
		c.Request.Context(),
		uploadID,
		c.PostForm("user_id"),
		int(chunkIndex64),
		c.PostForm("chunk_hash"),
		file,
	)
	if err != nil {
		c.JSON(nethttp.StatusConflict, gin.H{"detail": err.Error()})
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{
		"upload_id": task.ID, "chunk_index": chunkIndex64, "uploaded_count": task.UploadedCount,
		"chunk_count": task.ChunkCount, "already_uploaded": already, "can_merge": task.UploadedCount == task.ChunkCount,
	})
}

func (h *Handler) mergeUpload(c *gin.Context) {
	uploadID, ok := uintParam(c, "upload_id")
	if !ok {
		return
	}
	task, meta, userFile, tasks, err := h.service.MergeUpload(c.Request.Context(), uploadID, c.Query("user_id"))
	if err != nil {
		c.JSON(nethttp.StatusConflict, gin.H{"detail": err.Error()})
		return
	}
	ids := make([]uint, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	var fileID any
	if meta != nil {
		fileID = meta.ID
	}
	var userFileID any
	if userFile != nil {
		userFileID = userFile.ID
	}
	c.JSON(nethttp.StatusOK, gin.H{
		"upload_id": task.ID, "status": task.Status, "file_id": fileID,
		"user_file_id": userFileID, "media_task_ids": ids, "message": "merge completed",
	})
}

func (h *Handler) listFiles(c *gin.Context) {
	files, err := h.service.ListFiles(c.Query("user_id"))
	if err != nil {
		c.JSON(nethttp.StatusInternalServerError, gin.H{"detail": err.Error()})
		return
	}
	c.JSON(nethttp.StatusOK, files)
}

func (h *Handler) downloadFile(c *gin.Context) {
	fileID, ok := uintParam(c, "file_id")
	if !ok {
		return
	}
	file, reader, err := h.service.DownloadFile(c.Request.Context(), c.Query("user_id"), fileID)
	if err != nil {
		c.JSON(nethttp.StatusNotFound, gin.H{"detail": "file not found"})
		return
	}
	defer reader.Close()

	contentType := file.File.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	encodedName := url.PathEscape(file.FileName)
	c.DataFromReader(nethttp.StatusOK, file.File.FileSize, contentType, reader, map[string]string{
		"Content-Disposition": `attachment; filename="` + file.FileName + `"; filename*=UTF-8''` + encodedName,
	})
}

func (h *Handler) deleteFile(c *gin.Context) {
	fileID, ok := uintParam(c, "file_id")
	if !ok {
		return
	}
	result, err := h.service.DeletePhysicalFile(c.Request.Context(), c.Query("user_id"), fileID)
	if err != nil {
		c.JSON(nethttp.StatusNotFound, gin.H{"detail": err.Error()})
		return
	}
	c.JSON(nethttp.StatusOK, result)
}

func (h *Handler) mediaTask(c *gin.Context) {
	taskID, ok := uintParam(c, "task_id")
	if !ok {
		return
	}
	task, err := h.service.MediaTask(taskID)
	if err != nil {
		c.JSON(nethttp.StatusNotFound, gin.H{"detail": "media task not found"})
		return
	}
	c.JSON(nethttp.StatusOK, task)
}

func (h *Handler) runMediaTask(c *gin.Context) {
	taskID, ok := uintParam(c, "task_id")
	if !ok {
		return
	}
	action, err := h.service.RunMediaTask(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(nethttp.StatusInternalServerError, gin.H{"detail": err.Error()})
		return
	}
	task, _ := h.service.MediaTask(taskID)
	c.JSON(nethttp.StatusOK, gin.H{"task": task, "queue_action": action})
}

func (h *Handler) compensate(c *gin.Context) {
	result, err := h.service.Compensate(c.Request.Context())
	if err != nil {
		c.JSON(nethttp.StatusInternalServerError, gin.H{"detail": err.Error()})
		return
	}
	c.JSON(nethttp.StatusOK, result)
}

func uintParam(c *gin.Context, name string) (uint, bool) {
	value, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil {
		c.JSON(nethttp.StatusBadRequest, gin.H{"detail": "invalid " + name})
		return 0, false
	}
	return uint(value), true
}
