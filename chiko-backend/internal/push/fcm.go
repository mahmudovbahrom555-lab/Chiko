// Package push delivers Firebase Cloud Messaging notifications (ТЗ раздел 5).
//
// Critical events  → full-screen Android / Time Sensitive iOS.
// Normal events    → standard priority.
//
// Token lifetime: firebase-admin-go refreshes OAuth2 credentials automatically
// using the service account key (FCM_SERVICE_ACCOUNT_JSON).
// No manual token rotation needed.
package push

import (
	"context"
	"fmt"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
	"google.golang.org/api/option"
)

// EventType mirrors the Sprints v4 push event list.
type EventType string

const (
	// Critical — full-screen push (ТЗ раздел 5).
	EventOrderConfirmed EventType = "order.confirmed"
	EventOrderReady     EventType = "order.ready"

	// Normal priority.
	EventReturnRequested   EventType = "return.requested"
	EventReturnEscalated   EventType = "return.escalated"
	EventLimitApproaching  EventType = "limit.approaching"
	EventConflictOverwrite EventType = "conflict.overwritten"
)

// Payload is the generic push payload.
type Payload struct {
	Type   EventType
	Title  string
	Body   string
	Data   map[string]any
	UserID uuid.UUID // recipient
}

// Service sends push notifications via Firebase Admin SDK (FCM HTTP v1 API).
// Credentials are loaded from a service account JSON file; the SDK refreshes
// the OAuth2 token automatically — no manual rotation needed.
type Service struct {
	msgClient *messaging.Client // nil when FCM not configured
	db        *pgxpool.Pool
}

// NewService initialises push with a Firebase service account key.
// serviceAccountJSON is the path to the downloaded key file from Firebase Console.
// If empty, push is silently disabled (useful in local dev without Firebase).
func NewService(serviceAccountJSON string, db *pgxpool.Pool) *Service {
	if serviceAccountJSON == "" {
		log.Info().Msg("push: FCM_SERVICE_ACCOUNT_JSON not set — push disabled")
		return &Service{db: db}
	}

	app, err := firebase.NewApp(context.Background(), nil,
		option.WithCredentialsFile(serviceAccountJSON))
	if err != nil {
		log.Error().Err(err).Msg("push: Firebase app init failed — push disabled")
		return &Service{db: db}
	}

	client, err := app.Messaging(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("push: messaging client init failed — push disabled")
		return &Service{db: db}
	}

	log.Info().Msg("push: Firebase messaging client ready")
	return &Service{msgClient: client, db: db}
}

// Send delivers a push notification to the given user.
// Looks up push_token from producers table.
func (s *Service) Send(ctx context.Context, p Payload) {
	if s.msgClient == nil {
		return
	}

	token, err := s.getToken(ctx, p.UserID)
	if err != nil || token == "" {
		return
	}

	isCritical := p.Type == EventOrderConfirmed || p.Type == EventOrderReady
	msg := s.buildMessage(token, p, isCritical)

	if _, err := s.msgClient.Send(ctx, msg); err != nil {
		if messaging.IsRegistrationTokenNotRegistered(err) {
			s.disableToken(ctx, p.UserID, token)
			return
		}
		log.Error().Err(err).Str("user", p.UserID.String()).Str("type", string(p.Type)).
			Msg("push: FCM send failed")
	}
}

// SendToOpposite sends to the OTHER side of the chat (ТЗ Step 4.1).
// If caller=Client → push to Producer, and vice versa.
func (s *Service) SendToOpposite(ctx context.Context, chatID, callerID uuid.UUID, p Payload) {
	if s.msgClient == nil {
		return
	}
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
	if err != nil {
		log.Debug().Err(err).Str("user", userID.String()).Msg("push: getToken DB error")
		return "", nil
	}
	if !enabled || token == "" {
		return "", nil
	}
	return token, nil
}

func (s *Service) disableToken(ctx context.Context, userID uuid.UUID, token string) {
	s.db.Exec(ctx, `
		UPDATE producers SET push_enabled = FALSE
		WHERE id = $1 AND push_token = $2
	`, userID, token)
	log.Warn().Str("user", userID.String()).Msg("push: token unregistered, push_enabled=false")
}

// buildMessage constructs the FCM Message using typed firebase-admin-go structs.
// Critical events use Android high-priority + iOS time-sensitive interruption level.
func (s *Service) buildMessage(token string, p Payload, critical bool) *messaging.Message {
	dataMap := map[string]string{"type": string(p.Type)}
	for k, v := range p.Data {
		dataMap[k] = fmt.Sprintf("%v", v)
	}

	msg := &messaging.Message{
		Token: token,
		Notification: &messaging.Notification{
			Title: p.Title,
			Body:  p.Body,
		},
		Data: dataMap,
	}

	if critical {
		msg.Android = &messaging.AndroidConfig{
			Priority: "high",
			Notification: &messaging.AndroidNotification{
				ChannelID: "chiko_critical",
			},
		}
		// iOS: Time Sensitive interruption level via CustomData
		// (Aps.InterruptionLevel not yet in firebase-admin-go v4.20.0).
		msg.APNS = &messaging.APNSConfig{
			Headers: map[string]string{
				"apns-push-type":  "alert",
				"apns-priority":   "10",
				"apns-expiration": "0",
			},
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					Alert: &messaging.ApsAlert{
						Title: p.Title,
						Body:  p.Body,
					},
					Sound: "chiko_call.caf",
					CustomData: map[string]interface{}{
						"interruption-level": "time-sensitive",
					},
				},
			},
		}
	}

	return msg
}
