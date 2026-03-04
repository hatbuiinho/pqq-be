package router

import (
	"net/http"

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
	return func(c *gin.Context) {
		if allowedOrigin != "" {
			c.Header("Access-Control-Allow-Origin", allowedOrigin)
		}
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Credentials", "true")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
