package http

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type Hub struct {
	mu    sync.Mutex
	conns map[*client]struct{}
}

type client struct {
	conn *websocket.Conn
	wmu  sync.Mutex
}

func newHub() *Hub {
	return &Hub{conns: map[*client]struct{}{}}
}

func (h *Hub) add(c *client) {
	h.mu.Lock()
	h.conns[c] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) remove(c *client) {
	h.mu.Lock()
	delete(h.conns, c)
	h.mu.Unlock()
}

func (h *Hub) broadcast(b []byte) {
	h.mu.Lock()
	clients := make([]*client, 0, len(h.conns))
	for c := range h.conns {
		clients = append(clients, c)
	}
	h.mu.Unlock()
	for _, c := range clients {
		c.wmu.Lock()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := c.conn.Write(ctx, websocket.MessageText, b)
		cancel()
		c.wmu.Unlock()
		if err != nil {
			c.conn.Close(websocket.StatusNormalClosure, "")
		}
	}
}

func encodeMsg(type_ string, payload any) []byte {
	var raw json.RawMessage
	if payload != nil {
		if r, err := json.Marshal(payload); err == nil {
			raw = r
		}
	}
	b, _ := json.Marshal(struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload,omitempty"`
	}{type_, raw})
	return b
}
