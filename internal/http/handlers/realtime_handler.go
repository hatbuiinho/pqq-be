package handlers

import (
	"net/http"

	"pqq/be/internal/sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type RealtimeHandler struct {
	hub      *sync.Hub
	upgrader websocket.Upgrader
}

func NewRealtimeHandler(hub *sync.Hub) *RealtimeHandler {
	return &RealtimeHandler{
		hub: hub,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (h *RealtimeHandler) ServeWS(c *gin.Context) {
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	h.hub.Register(conn)
}
