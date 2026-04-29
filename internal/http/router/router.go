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
	authHandler *handlers.AuthHandler,
	authMiddleware gin.HandlerFunc,
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
		api.POST("/auth/login", authHandler.Login)
		api.GET("/auth/club-invites/:token", authHandler.GetClubInvitePreview)
		api.POST("/auth/club-invites/:token/accept", authHandler.AcceptClubInvite)
		api.GET("/students/public-profile/:studentCode", syncHandler.GetStudentPublicProfile)
		api.GET("/sync/ws", realtimeHandler.ServeWS)

		authenticated := api.Group("")
		authenticated.Use(authMiddleware)
		authenticated.GET("/auth/me", authHandler.Me)
		authenticated.GET("/auth/memberships", authHandler.Memberships)
		authenticated.GET("/auth/clubs/:clubId/permissions", authHandler.ClubPermissions)
		authenticated.GET("/auth/audit-logs", authHandler.ListAuditLogs)
		authenticated.GET("/auth/users", authHandler.ListUsers)
		authenticated.POST("/auth/users", authHandler.CreateUser)
		authenticated.POST("/auth/users/:userId/status", authHandler.UpdateUserStatus)
		authenticated.POST("/auth/users/:userId/reset-password", authHandler.ResetUserPassword)
		authenticated.GET("/auth/users/:userId/memberships", authHandler.GetUserMemberships)
		authenticated.POST("/auth/users/:userId/memberships", authHandler.AddMembership)
		authenticated.POST("/auth/memberships/:membershipId/delete", authHandler.RemoveMembership)
		authenticated.GET("/auth/club-invites", authHandler.ListClubInvites)
		authenticated.POST("/auth/club-invites", authHandler.CreateClubInvite)
		authenticated.POST("/auth/club-invites/by-id/:inviteId/revoke", authHandler.RevokeClubInvite)
		authenticated.POST("/attendance/sessions", syncHandler.CreateAttendanceSession)
		authenticated.POST("/sync/push", syncHandler.Push)
		authenticated.POST("/sync/attendance-actions", syncHandler.PushAttendanceActions)
		authenticated.GET("/sync/pull", syncHandler.Pull)
		authenticated.GET("/sync/rebase", syncHandler.Rebase)
		authenticated.POST("/clubs/import", syncHandler.ImportClubs)
		authenticated.POST("/belt-ranks/import", syncHandler.ImportBeltRanks)
		authenticated.POST("/students/import", syncHandler.ImportStudents)
		authenticated.GET("/students/import-template", syncHandler.DownloadStudentImportTemplate)
		if studentMediaHandler != nil {
			authenticated.GET("/students/:studentId/avatars", studentMediaHandler.ListStudentAvatars)
			authenticated.POST("/students/:studentId/avatars", studentMediaHandler.UploadStudentAvatar)
			authenticated.POST("/students/:studentId/avatars/:mediaId/primary", studentMediaHandler.SetPrimaryAvatar)
			authenticated.POST("/students/:studentId/avatars/:mediaId/delete", studentMediaHandler.DeleteAvatar)
			authenticated.POST("/media/avatar-imports/analyze", studentMediaHandler.AnalyzeAvatarImport)
			authenticated.GET("/media/avatar-imports/:batchId", studentMediaHandler.GetAvatarImportBatch)
			authenticated.POST("/media/avatar-imports/:batchId/confirm", studentMediaHandler.ConfirmAvatarImport)
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
