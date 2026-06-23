package debt

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

const defaultSLAHours = 48
const slaJobInterval = 30 * time.Minute

// StartSLAJob runs a background goroutine that escalates overdue return requests
// every 30 minutes (ТЗ раздел 7: SLA эскалация).
// The SLA threshold is read from DB feature_flags on each tick (configurable
// without restart, per ТЗ раздел 13.3).
func StartSLAJob(ctx context.Context, svc *Service) {
	go func() {
		ticker := time.NewTicker(slaJobInterval)
		defer ticker.Stop()

		// Run once immediately on startup to catch any backlog.
		runEscalation(ctx, svc)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runEscalation(ctx, svc)
			}
		}
	}()
}

func runEscalation(ctx context.Context, svc *Service) {
	slaHours := svc.readSLAHours(ctx)

	n, err := svc.EscalateOverdueReturns(ctx, slaHours)
	if err != nil {
		log.Error().Err(err).Msg("sla: escalation job failed")
		return
	}
	if n > 0 {
		log.Info().Int("escalated", n).Int("sla_hours", slaHours).Msg("sla: return requests escalated")
	}
}

// readSLAHours reads the SLA threshold from feature_flags.
// Falls back to 48h if not set.
func (s *Service) readSLAHours(ctx context.Context) int {
	// feature_flags table stores key/enabled/scope.
	// For numeric params we store the value in a separate config or encode in key name.
	// For MVP: read from a numeric feature_flag "sla_return_hours" stored as scope value.
	// If not found → use default.
	var scope string
	err := s.db.QueryRow(ctx, `
		SELECT scope FROM feature_flags WHERE key = 'sla_return_hours' AND enabled = TRUE
	`).Scan(&scope)
	if err != nil || scope == "" {
		return defaultSLAHours
	}

	// Scope field re-purposed to store the hour count as a string.
	var hours int
	if _, err := fmt.Sscanf(scope, "%d", &hours); err != nil || hours <= 0 {
		return defaultSLAHours
	}
	return hours
}
