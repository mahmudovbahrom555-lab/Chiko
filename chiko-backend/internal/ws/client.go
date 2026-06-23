package ws

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second
	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second
	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = 30 * time.Second
	// Maximum message size allowed from peer.
	maxMessageSize = 4096
	// Send buffer per client.
	sendBufSize = 256
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Auth is verified before the upgrade in the HTTP handler — here we just accept.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Client represents one authenticated WebSocket connection.
type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	db     *pgxpool.Pool
	userID uuid.UUID
	chatID uuid.UUID
}

// readPump pumps messages from the WebSocket connection to the hub.
// One goroutine per Client.
func (c *Client) readPump() {
	defer func() {
		// Cleanup: release DB locks and unregister from hub.
		go ReleaseLocks(c.db, c.userID)
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Warn().Err(err).Str("user", c.userID.String()).Msg("ws: unexpected close")
			}
			break
		}
		c.handleIncoming(msg)
	}
}

// writePump pumps messages from the hub to the WebSocket connection.
// One goroutine per Client.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				log.Warn().Err(err).Str("user", c.userID.String()).Msg("ws: write error")
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleIncoming processes a raw message from the WebSocket client.
// Currently clients can send lock-release hints or ACKs.
// Actual data mutations go through the REST API.
func (c *Client) handleIncoming(raw []byte) {
	var ev Event
	if err := json.Unmarshal(raw, &ev); err != nil {
		log.Debug().Str("user", c.userID.String()).Msg("ws: received non-JSON message, ignoring")
		return
	}
	// Future: route ev.Type to handlers (e.g. "lock.request").
	// MVP: only server-initiated events; client messages are informational.
	log.Debug().
		Str("user", c.userID.String()).
		Str("type", string(ev.Type)).
		Msg("ws: received client event")
}
