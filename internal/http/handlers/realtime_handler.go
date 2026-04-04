package handlers

import (
	"net/http"
	"strings"

	"pqq/be/internal/auth"
	"pqq/be/internal/sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type RealtimeHandler struct {
	authService *auth.Service
	hub         *sync.Hub
	upgrader    websocket.Upgrader
}

func NewRealtimeHandler(authService *auth.Service, hub *sync.Hub) *RealtimeHandler {
	return &RealtimeHandler{
		authService: authService,
		hub:         hub,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (h *RealtimeHandler) ServeWS(c *gin.Context) {
	accessToken := strings.TrimSpace(c.Query("access_token"))
	if accessToken == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing access token"})
		return
	}

	if _, err := h.authService.ParseToken(accessToken); err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	h.hub.Register(conn)
}
