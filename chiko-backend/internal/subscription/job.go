package subscription

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

const trialCheckInterval = time.Hour

// StartTrialExpiryJob runs a background goroutine that transitions
// expired Trial subscriptions to Free (ТЗ раздел 12.7, раздел 3.1).
// No payment integration — Trial→Free happens automatically.
// Trial→Pro remains a manual admin operation in MVP.
func StartTrialExpiryJob(ctx context.Context, db *pgxpool.Pool) {
	go func() {
		// Check immediately on startup to handle any backlog.
		expireTrials(ctx, db)

		ticker := time.NewTicker(trialCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				expireTrials(ctx, db)
			}
		}
	}()
}

func expireTrials(ctx context.Context, db *pgxpool.Pool) {
	tag, err := db.Exec(ctx, `
		UPDATE subscriptions s
		SET    plan_id    = (SELECT id FROM plans WHERE name = 'Free' AND active = TRUE LIMIT 1),
		       status     = 'active'
		WHERE  s.status       = 'trial'
		  AND  s.trial_ends_at < NOW()
		  AND  EXISTS (SELECT 1 FROM plans WHERE name = 'Free' AND active = TRUE)
	`)
	if err != nil {
		log.Error().Err(err).Msg("subscription: trial expiry job failed")
		return
	}
	if tag.RowsAffected() > 0 {
		log.Info().Int64("count", tag.RowsAffected()).Msg("subscription: Trial→Free transitions completed")
	}
}
