// Package push delivers Firebase Cloud Messaging notifications (ТЗ раздел 5).
//
// Critical events  → full-screen Android / Time Sensitive iOS (no Critical Alert in MVP).
// Normal events    → standard priority.
//
// Architecture: push_token comes from PUT /api/users/push-token (Step 1.5).
// If FCM returns UNREGISTERED → set push_enabled=false in producers.
package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// EventType mirrors the Sprints v4 push event list.
type EventType string

const (
	// Critical — full-screen push (ТЗ раздел 5).
	EventOrderConfirmed EventType = "order.confirmed"
	EventOrderReady     EventType = "order.ready"

	// Normal priority.
	EventReturnRequested  EventType = "return.requested"
	EventReturnEscalated  EventType = "return.escalated"
	EventLimitApproaching EventType = "limit.approaching"
	EventConflictOverwrite EventType = "conflict.overwritten"
)

// Payload is the generic push payload.
type Payload struct {
	Type    EventType      `json:"type"`
	Title   string         `json:"title"`
	Body    string         `json:"body"`
	Data    map[string]any `json:"data,omitempty"`
	UserID  uuid.UUID      `json:"-"` // recipient
}

// Service sends push notifications via FCM v1 HTTP API.
type Service struct {
	fcmKey string
	db     *pgxpool.Pool
	client *http.Client
}

func NewService(fcmKey string, db *pgxpool.Pool) *Service {
	return &Service{
		fcmKey: fcmKey,
		db:     db,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Send delivers a push notification to the given user.
// Looks up push_token from producers table.
func (s *Service) Send(ctx context.Context, p Payload) {
	if s.fcmKey == "" {
		log.Debug().Str("type", string(p.Type)).Msg("push: FCM key not configured, skip")
		return
	}

	token, err := s.getToken(ctx, p.UserID)
	if err != nil || token == "" {
		return // no token — user hasn't registered device yet
	}

	isCritical := p.Type == EventOrderConfirmed || p.Type == EventOrderReady

	fcmBody := s.buildFCMBody(token, p, isCritical)
	if err := s.sendToFCM(ctx, fcmBody, p.UserID, token); err != nil {
		log.Error().Err(err).Str("user", p.UserID.String()).Msg("push: FCM send failed")
	}
}

// SendToOpposite sends to the OTHER side of the chat (ТЗ Step 4.1).
// Used for order.confirmed: if caller=Client → push to Producer, and vice versa.
func (s *Service) SendToOpposite(ctx context.Context, chatID, callerID uuid.UUID, p Payload) {
	var recipientID uuid.UUID
	err := s.db.QueryRow(ctx, `
		SELECT CASE WHEN producer_id = $2 THEN client_id ELSE producer_id END
		FROM   chats WHERE id = $1 AND (producer_id = $2 OR client_id = $2)
	`, chatID, callerID).Scan(&recipientID)
	if err != nil || recipientID == uuid.Nil {
		return
	}
	p.UserID = recipientID
	s.Send(ctx, p)
}

func (s *Service) getToken(ctx context.Context, userID uuid.UUID) (string, error) {
	var token string
	var enabled bool
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(push_token,''), COALESCE(push_enabled, true)
		FROM producers WHERE id = $1
	`, userID).Scan(&token, &enabled)
	if err != nil || !enabled {
		return "", nil
	}
	return token, nil
}

// buildFCMBody creates the FCM v1 message JSON.
// Critical events use Android fullScreenIntent + iOS interruptionLevel=timeSensitive.
func (s *Service) buildFCMBody(token string, p Payload, critical bool) map[string]any {
	dataMap := map[string]string{"type": string(p.Type)}
	for k, v := range p.Data {
		dataMap[k] = fmt.Sprintf("%v", v)
	}

	msg := map[string]any{
		"token": token,
		"notification": map[string]string{
			"title": p.Title,
			"body":  p.Body,
		},
		"data": dataMap,
	}

	if critical {
		// Android: full-screen intent (flutter_local_notifications handles this client-side).
		msg["android"] = map[string]any{
			"priority": "high",
			"notification": map[string]any{
				"channel_id": "chiko_critical",
			},
		}
		// iOS: Time Sensitive category (no Critical Alert — осознанно вне MVP).
		msg["apns"] = map[string]any{
			"headers": map[string]string{
				"apns-push-type":     "alert",
				"apns-priority":      "10",
				"apns-expiration":    "0",
			},
			"payload": map[string]any{
				"aps": map[string]any{
					"alert":              map[string]string{"title": p.Title, "body": p.Body},
					"interruption-level": "time-sensitive",
					"sound":              "chiko_call.caf",
				},
			},
		}
	}

	return map[string]any{"message": msg}
}

func (s *Service) sendToFCM(ctx context.Context, body map[string]any, userID uuid.UUID, token string) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}

	// FCM v1 endpoint. FCM_KEY should be a valid OAuth2 bearer token or legacy server key.
	// In production: use firebase-admin-go for token exchange; in MVP legacy key is acceptable.
	url := "https://fcm.googleapis.com/fcm/send" // legacy v1; switch to v1 endpoint when using OAuth2
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "key="+s.fcmKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("FCM HTTP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("FCM HTTP %d", resp.StatusCode)
	}

	// Parse FCM response to detect UNREGISTERED token (ТЗ раздел 5 — metric).
	var fcmResp struct {
		Results []struct {
			Error string `json:"error"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&fcmResp); err == nil {
		for _, r := range fcmResp.Results {
			if r.Error == "NotRegistered" || r.Error == "InvalidRegistration" {
				// Token is stale → disable push for this user (ТЗ раздел 5).
				s.db.Exec(ctx, `
					UPDATE producers SET push_enabled = FALSE
					WHERE id = $1 AND push_token = $2
				`, userID, token)
				log.Warn().
					Str("user", userID.String()).
					Msg("push: token unregistered, push_enabled=false")
			}
		}
	}

	return nil
}
