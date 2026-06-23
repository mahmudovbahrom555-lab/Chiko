package ws

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

const softLockDuration = 3 * time.Second

// ReleaseLocks clears every soft lock held by userID across all active orders.
// Called when a WebSocket client disconnects (ТЗ раздел 4.4, раздел 12.4).
// Uses a short context because disconnect cleanup is best-effort.
func ReleaseLocks(db *pgxpool.Pool, userID uuid.UUID) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tag, err := db.Exec(ctx, `
		UPDATE order_items
		SET    locked_by    = NULL,
		       locked_until = NULL
		WHERE  locked_by    = $1
	`, userID)
	if err != nil {
		log.Error().Err(err).Str("user", userID.String()).Msg("ws: failed to release locks on disconnect")
		return
	}
	if tag.RowsAffected() > 0 {
		log.Debug().
			Str("user", userID.String()).
			Int64("rows", tag.RowsAffected()).
			Msg("ws: locks released on disconnect")
	}
}

// AcquireLockResult holds the outcome of a soft-lock attempt.
type AcquireLockResult struct {
	PrevLockedBy uuid.UUID // zero if row was unlocked
	Conflict     bool      // true when prev lock belonged to a DIFFERENT user
}

// AcquireLock sets locked_by/locked_until on an order_item.
// Returns info about any previous lock so the caller can broadcast
// conflict.overwritten to the losing side.
// Last-write-wins: we always update, even if the row is locked by someone else.
func AcquireLock(ctx context.Context, db *pgxpool.Pool, itemID, userID uuid.UUID) (AcquireLockResult, error) {
	var prevLockedBy *uuid.UUID
	err := db.QueryRow(ctx, `
		UPDATE order_items
		SET    locked_by    = $2,
		       locked_until = NOW() + $3::interval
		WHERE  id = $1
		RETURNING (SELECT locked_by FROM order_items WHERE id = $1)
	`, itemID, userID, softLockDuration.String()).Scan(&prevLockedBy)
	if err != nil {
		return AcquireLockResult{}, err
	}

	var res AcquireLockResult
	if prevLockedBy != nil && *prevLockedBy != userID {
		// Row was locked by a different user — they lose (last write wins).
		res.PrevLockedBy = *prevLockedBy
		res.Conflict = true
	}
	return res, nil
}
