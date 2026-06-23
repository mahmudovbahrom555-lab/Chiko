package ws

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/chiko/backend/internal/middleware"
)

// Handler returns the HTTP handler that upgrades a connection to WebSocket.
// chatID is extracted from the query parameter "chat_id".
// userID is taken from the auth context (JWT already validated by middleware).
func Handler(hub *Hub, db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := middleware.UserIDFromCtx(r.Context())
		if !ok {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		rawChatID := r.URL.Query().Get("chat_id")
		chatID, err := uuid.Parse(rawChatID)
		if err != nil {
			http.Error(w, `{"error":"invalid_chat_id"}`, http.StatusBadRequest)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Error().Err(err).Str("user", userID.String()).Msg("ws: upgrade failed")
			return
		}

		client := &Client{
			hub:    hub,
			conn:   conn,
			send:   make(chan []byte, sendBufSize),
			db:     db,
			userID: userID,
			chatID: chatID,
		}

		hub.register <- client

		// Each client runs two goroutines.
		go client.writePump()
		go client.readPump()
	}
}
