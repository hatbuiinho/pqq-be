package router

import (
	"net/http"
	"strings"

	"pqq/be/internal/config"
	"pqq/be/internal/http/handlers"

	"github.com/gin-gonic/gin"
)

func New(
	cfg config.Config,
	syncHandler *handlers.SyncHandler,
	realtimeHandler *handlers.RealtimeHandler,
	studentMediaHandler *handlers.StudentMediaHandler,
) *gin.Engine {
	engine := gin.New()
	engine.Use(gin.Recovery(), gin.Logger(), corsMiddleware(cfg.AllowedOrigin))

	engine.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	api := engine.Group("/api/v1")
	{
		api.POST("/sync/push", syncHandler.Push)
		api.GET("/sync/pull", syncHandler.Pull)
		api.GET("/sync/rebase", syncHandler.Rebase)
		api.POST("/clubs/import", syncHandler.ImportClubs)
		api.POST("/belt-ranks/import", syncHandler.ImportBeltRanks)
		api.POST("/students/import", syncHandler.ImportStudents)
		api.GET("/students/import-template", syncHandler.DownloadStudentImportTemplate)
		api.GET("/sync/ws", realtimeHandler.ServeWS)
		if studentMediaHandler != nil {
			api.GET("/students/:studentId/avatars", studentMediaHandler.ListStudentAvatars)
			api.POST("/students/:studentId/avatars", studentMediaHandler.UploadStudentAvatar)
			api.POST("/students/:studentId/avatars/:mediaId/primary", studentMediaHandler.SetPrimaryAvatar)
			api.POST("/students/:studentId/avatars/:mediaId/delete", studentMediaHandler.DeleteAvatar)
			api.POST("/media/avatar-imports/analyze", studentMediaHandler.AnalyzeAvatarImport)
			api.GET("/media/avatar-imports/:batchId", studentMediaHandler.GetAvatarImportBatch)
			api.POST("/media/avatar-imports/:batchId/confirm", studentMediaHandler.ConfirmAvatarImport)
		}
	}

	return engine
}

func corsMiddleware(allowedOrigin string) gin.HandlerFunc {
	allowedOrigins := parseAllowedOrigins(allowedOrigin)
	allowAll := len(allowedOrigins) == 1 && allowedOrigins[0] == "*"

	return func(c *gin.Context) {
		requestOrigin := c.GetHeader("Origin")
		if requestOrigin != "" {
			if allowAll {
				c.Header("Access-Control-Allow-Origin", "*")
			} else if isAllowedOrigin(requestOrigin, allowedOrigins) {
				c.Header("Access-Control-Allow-Origin", requestOrigin)
				c.Header("Vary", "Origin")
			}
		}
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if !allowAll {
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func parseAllowedOrigins(value string) []string {
	parts := strings.Split(value, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		origin := strings.TrimSpace(part)
		if origin == "" {
			continue
		}
		origins = append(origins, origin)
	}

	if len(origins) == 0 {
		return []string{"*"}
	}

	return origins
}

func isAllowedOrigin(origin string, allowedOrigins []string) bool {
	for _, allowed := range allowedOrigins {
		if allowed == origin {
			return true
		}
	}
	return false
}
