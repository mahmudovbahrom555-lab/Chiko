package ws

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// BroadcastMsg carries an event to all clients in a chat room,
// optionally skipping the client that triggered it (ExcludeID).
type BroadcastMsg struct {
	ChatID    uuid.UUID
	Data      []byte    // pre-encoded JSON
	ExcludeID uuid.UUID // zero value = send to everyone
}

// Hub manages all active WebSocket connections, grouped by chat_id.
// Only the Hub's Run goroutine reads/writes the rooms map — no mutex needed there.
type Hub struct {
	// rooms[chatID] → set of connected clients
	rooms map[uuid.UUID]map[*Client]struct{}

	register   chan *Client
	unregister chan *Client
	broadcast  chan BroadcastMsg

	mu sync.RWMutex // protects nothing in rooms; used only for external Send calls
}

// NewHub creates an uninitialised Hub. Call Run() to start it.
func NewHub() *Hub {
	return &Hub{
		rooms:      make(map[uuid.UUID]map[*Client]struct{}),
		register:   make(chan *Client, 64),
		unregister: make(chan *Client, 64),
		broadcast:  make(chan BroadcastMsg, 256),
	}
}

// Run processes all hub events in a single goroutine.
// Block here — launch with go hub.Run(ctx).
func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			h.closeAll()
			return

		case c := <-h.register:
			if h.rooms[c.chatID] == nil {
				h.rooms[c.chatID] = make(map[*Client]struct{})
			}
			h.rooms[c.chatID][c] = struct{}{}
			log.Debug().
				Str("user", c.userID.String()).
				Str("chat", c.chatID.String()).
				Int("room_size", len(h.rooms[c.chatID])).
				Msg("ws: client registered")

		case c := <-h.unregister:
			if room, ok := h.rooms[c.chatID]; ok {
				if _, exists := room[c]; exists {
					delete(room, c)
					close(c.send)
					if len(room) == 0 {
						delete(h.rooms, c.chatID)
					}
				}
			}
			log.Debug().
				Str("user", c.userID.String()).
				Str("chat", c.chatID.String()).
				Msg("ws: client unregistered")

		case msg := <-h.broadcast:
			room, ok := h.rooms[msg.ChatID]
			if !ok {
				continue
			}
			for c := range room {
				if c.userID == msg.ExcludeID {
					continue
				}
				select {
				case c.send <- msg.Data:
				default:
					// Slow client — drop message, do not block hub.
					log.Warn().
						Str("user", c.userID.String()).
						Msg("ws: dropping message for slow client")
				}
			}
		}
	}
}

// Broadcast sends a pre-encoded message to everyone in the chat except ExcludeID.
// Safe to call from any goroutine.
func (h *Hub) Broadcast(msg BroadcastMsg) {
	select {
	case h.broadcast <- msg:
	default:
		log.Warn().Str("chat", msg.ChatID.String()).Msg("ws: broadcast channel full, message dropped")
	}
}

// OnlineCount returns the number of connected clients in a chat room.
// Primarily for debugging / health checks.
func (h *Hub) OnlineCount(chatID uuid.UUID) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.rooms[chatID])
}

func (h *Hub) closeAll() {
	for _, room := range h.rooms {
		for c := range room {
			close(c.send)
		}
	}
	h.rooms = make(map[uuid.UUID]map[*Client]struct{})
}

// --- Test helpers (used only in *_test.go files) ---

// NewTestClient constructs a minimal Client for unit tests (no real WS conn).
func NewTestClient(hub *Hub, chatID, userID uuid.UUID, send chan []byte) *Client {
	return &Client{hub: hub, chatID: chatID, userID: userID, send: send}
}

// RegisterTest sends a client to the register channel (for tests).
func (h *Hub) RegisterTest(c *Client) { h.register <- c }

// UnregisterTest sends a client to the unregister channel (for tests).
func (h *Hub) UnregisterTest(c *Client) { h.unregister <- c }
