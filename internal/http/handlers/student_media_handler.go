package handlers

import (
	"net/http"
	"strings"

	"pqq/be/internal/media"

	"github.com/gin-gonic/gin"
)

type StudentMediaHandler struct {
	service *media.Service
}

func NewStudentMediaHandler(service *media.Service) *StudentMediaHandler {
	return &StudentMediaHandler{service: service}
}

func (h *StudentMediaHandler) ListStudentAvatars(c *gin.Context) {
	items, err := h.service.ListStudentAvatars(c.Request.Context(), c.Param("studentId"))
	if err != nil {
		handleStudentMediaError(c, err)
		return
	}

	c.JSON(http.StatusOK, media.ListStudentAvatarsResponse{Items: items})
}

func (h *StudentMediaHandler) UploadStudentAvatar(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	defer file.Close()

	avatar, err := h.service.UploadStudentAvatar(
		c.Request.Context(),
		c.Param("studentId"),
		header.Filename,
		header.Header.Get("Content-Type"),
		file,
		header.Size,
	)
	if err != nil {
		handleStudentMediaError(c, err)
		return
	}

	c.JSON(http.StatusOK, media.UploadStudentAvatarResponse{Avatar: *avatar})
}

func (h *StudentMediaHandler) SetPrimaryAvatar(c *gin.Context) {
	avatar, err := h.service.SetPrimaryAvatar(c.Request.Context(), c.Param("studentId"), c.Param("mediaId"))
	if err != nil {
		handleStudentMediaError(c, err)
		return
	}

	c.JSON(http.StatusOK, media.SetPrimaryStudentAvatarResponse{Avatar: *avatar})
}

func (h *StudentMediaHandler) DeleteAvatar(c *gin.Context) {
	if err := h.service.DeleteAvatar(c.Request.Context(), c.Param("studentId"), c.Param("mediaId")); err != nil {
		handleStudentMediaError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *StudentMediaHandler) AnalyzeAvatarImport(c *gin.Context) {
	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "files are required"})
		return
	}

	fileHeaders := form.File["files"]
	if len(fileHeaders) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "files are required"})
		return
	}

	files := make([]media.AvatarImportUpload, 0, len(fileHeaders))
	for _, header := range fileHeaders {
		file, err := header.Open()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		defer file.Close()

		files = append(files, media.AvatarImportUpload{
			Filename:    header.Filename,
			ContentType: header.Header.Get("Content-Type"),
			Size:        header.Size,
			Reader:      file,
		})
	}

	response, err := h.service.AnalyzeAvatarImport(c.Request.Context(), files)
	if err != nil {
		handleStudentMediaError(c, err)
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *StudentMediaHandler) GetAvatarImportBatch(c *gin.Context) {
	response, err := h.service.GetAvatarImportBatch(c.Request.Context(), c.Param("batchId"))
	if err != nil {
		handleStudentMediaError(c, err)
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *StudentMediaHandler) ConfirmAvatarImport(c *gin.Context) {
	var request media.ConfirmAvatarImportRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	response, err := h.service.ConfirmAvatarImport(c.Request.Context(), c.Param("batchId"), request)
	if err != nil {
		handleStudentMediaError(c, err)
		return
	}

	c.JSON(http.StatusOK, response)
}

func handleStudentMediaError(c *gin.Context, err error) {
	message := err.Error()

	switch {
	case message == "unauthorized":
		c.JSON(http.StatusUnauthorized, gin.H{"error": message})
	case message == "forbidden":
		c.JSON(http.StatusForbidden, gin.H{"error": message})
	case strings.Contains(message, "does not exist"):
		c.JSON(http.StatusNotFound, gin.H{"error": message})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": message})
	}
}
