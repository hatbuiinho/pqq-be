package sync

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type client struct {
	connectionID string
	conn         *websocket.Conn
	send         chan []byte
}

type Hub struct {
	mu      sync.RWMutex
	clients map[*client]struct{}
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[*client]struct{}),
	}
}

func (h *Hub) Register(conn *websocket.Conn) {
	c := &client{
		connectionID: generateConnectionID(),
		conn:         conn,
		send:         make(chan []byte, 16),
	}

	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()

	connectedPayload, _ := json.Marshal(RealtimeEvent{
		Type:         "connected",
		ConnectionID: c.connectionID,
		ServerTime:   time.Now().UTC().Format(time.RFC3339Nano),
	})
	c.send <- connectedPayload

	go h.writeLoop(c)
	go h.readLoop(c)
}

func (h *Hub) BroadcastChange(entityNames []EntityName, recordIDs []string) {
	payload, err := json.Marshal(RealtimeEvent{
		Type:        "sync.changed",
		ServerTime:  time.Now().UTC().Format(time.RFC3339Nano),
		EntityNames: entityNames,
		RecordIDs:   recordIDs,
	})
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for c := range h.clients {
		select {
		case c.send <- payload:
		default:
			go h.unregister(c)
		}
	}
}

func (h *Hub) writeLoop(c *client) {
	ticker := time.NewTicker(25 * time.Second)
	defer func() {
		ticker.Stop()
		h.unregister(c)
	}()

	for {
		select {
		case payload, ok := <-c.send:
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				return
			}
		case <-ticker.C:
			payload, _ := json.Marshal(RealtimeEvent{
				Type:       "ping",
				ServerTime: time.Now().UTC().Format(time.RFC3339Nano),
			})
			if err := c.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				return
			}
		}
	}
}

func (h *Hub) readLoop(c *client) {
	defer h.unregister(c)

	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (h *Hub) unregister(c *client) {
	h.mu.Lock()
	if _, exists := h.clients[c]; !exists {
		h.mu.Unlock()
		return
	}
	delete(h.clients, c)
	close(c.send)
	h.mu.Unlock()

	if err := c.conn.Close(); err != nil {
		log.Printf("close websocket: %v", err)
	}
}

func generateConnectionID() string {
	return time.Now().UTC().Format("20060102150405.000000000")
}
