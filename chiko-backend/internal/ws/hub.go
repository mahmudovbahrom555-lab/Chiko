package ws

import (
	"context"

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

// statsReq is an internal request to count clients in a room.
// Using a channel ensures the read happens inside Hub.Run — no data race.
type statsReq struct {
	chatID uuid.UUID
	resp   chan int
}

// Hub manages all active WebSocket connections, grouped by chat_id.
// INVARIANT: only Hub.Run's goroutine reads/writes the rooms map.
// All public methods communicate via channels — no mutexes needed.
type Hub struct {
	rooms      map[uuid.UUID]map[*Client]struct{}
	register   chan *Client
	unregister chan *Client
	broadcast  chan BroadcastMsg
	stats      chan statsReq
}

// NewHub creates an uninitialised Hub. Call Run() to start it.
func NewHub() *Hub {
	return &Hub{
		rooms:      make(map[uuid.UUID]map[*Client]struct{}),
		register:   make(chan *Client, 64),
		unregister: make(chan *Client, 64),
		broadcast:  make(chan BroadcastMsg, 256),
		stats:      make(chan statsReq, 16),
	}
}

// Run processes all hub events in a single goroutine.
// Launch with: go hub.Run(ctx)
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
					log.Warn().
						Str("user", c.userID.String()).
						Msg("ws: dropping message for slow client")
				}
			}

		case req := <-h.stats:
			// Read inside Hub goroutine — no race.
			req.resp <- len(h.rooms[req.chatID])
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
// Routes through Hub.Run to avoid data races on the rooms map.
func (h *Hub) OnlineCount(chatID uuid.UUID) int {
	req := statsReq{chatID: chatID, resp: make(chan int, 1)}
	select {
	case h.stats <- req:
		return <-req.resp
	default:
		// Hub is busy or stopped — return 0 rather than block.
		return 0
	}
}

func (h *Hub) closeAll() {
	for _, room := range h.rooms {
		for c := range room {
			close(c.send)
		}
	}
	h.rooms = make(map[uuid.UUID]map[*Client]struct{})
}

// ── Test helpers ──────────────────────────────────────────────────────────────

// NewTestClient constructs a minimal Client for unit tests (no real WS conn or DB).
func NewTestClient(hub *Hub, chatID, userID uuid.UUID, send chan []byte) *Client {
	return &Client{hub: hub, chatID: chatID, userID: userID, send: send}
}

// RegisterTest sends a client to the register channel (for tests).
func (h *Hub) RegisterTest(c *Client) { h.register <- c }

// UnregisterTest sends a client to the unregister channel (for tests).
func (h *Hub) UnregisterTest(c *Client) { h.unregister <- c }
