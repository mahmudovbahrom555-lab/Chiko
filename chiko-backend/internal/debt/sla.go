package debt

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/chiko/backend/internal/push"
)

const defaultSLAHours = 48
const slaJobInterval = 30 * time.Minute

// StartSLAJob runs a background goroutine that escalates overdue return requests
// every 30 minutes (ТЗ раздел 7: SLA эскалация).
// The SLA threshold is read from DB feature_flags on each tick (configurable
// without restart, per ТЗ раздел 13.3).
func StartSLAJob(ctx context.Context, svc *Service, pushSvc *push.Service) {
	go func() {
		ticker := time.NewTicker(slaJobInterval)
		defer ticker.Stop()

		// Run once immediately on startup to catch any backlog.
		runEscalation(ctx, svc, pushSvc)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runEscalation(ctx, svc, pushSvc)
			}
		}
	}()
}

func runEscalation(ctx context.Context, svc *Service, pushSvc *push.Service) {
	slaHours := svc.readSLAHours(ctx)

	escalated, err := svc.EscalateOverdueReturns(ctx, slaHours)
	if err != nil {
		log.Error().Err(err).Msg("sla: escalation job failed")
		return
	}
	if len(escalated) == 0 {
		return
	}

	log.Info().Int("escalated", len(escalated)).Int("sla_hours", slaHours).Msg("sla: return requests escalated")

	// Push to each affected producer (ТЗ раздел 5, шаг 4.1).
	if pushSvc == nil {
		return
	}
	for _, e := range escalated {
		pushSvc.Send(ctx, push.Payload{
			Type:   push.EventReturnEscalated,
			Title:  "Возврат просрочен",
			Body:   "Запрос на возврат превысил SLA — требуется внимание",
			UserID: e.ProducerID,
			Data:   map[string]any{"chat_id": e.ChatID.String()},
		})
	}
}

// readSLAHours reads the SLA threshold from feature_flags.value_numeric (migration 009).
// Falls back to 48h if not configured.
func (s *Service) readSLAHours(ctx context.Context) int {
	var hours float64
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(value_numeric, $1)
		FROM   feature_flags
		WHERE  key = 'sla_return_hours' AND enabled = TRUE
	`, float64(defaultSLAHours)).Scan(&hours)
	if err != nil || hours <= 0 {
		return defaultSLAHours
	}
	return int(hours)
}
