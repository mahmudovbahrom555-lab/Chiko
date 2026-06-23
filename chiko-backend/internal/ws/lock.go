package ws

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

const softLockDuration = 3 * time.Second

// ReleaseLocks clears every soft lock held by userID across all active orders.
// Called when a WebSocket client disconnects (ТЗ раздел 4.4, раздел 12.4).
// Best-effort: uses its own context with a short timeout.
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
	PrevLockedBy    uuid.UUID  // zero if row was unlocked or lock was expired
	PrevLockedUntil *time.Time // nil if row was unlocked
	Conflict        bool       // true when an active lock belonged to a DIFFERENT user
}

// AcquireLock atomically reads the previous lock state and sets the new one.
//
// Uses a CTE to capture the old values BEFORE the UPDATE (standard PostgreSQL
// CTE snapshot behaviour — all CTEs see the pre-modification state).
// This fixes the earlier bug where RETURNING a sub-SELECT returned the NEW value.
//
// Last-write-wins: we always update, even if the row is locked by someone else.
// The caller is responsible for broadcasting conflict.overwritten to the loser.
func AcquireLock(ctx context.Context, db *pgxpool.Pool, itemID, userID uuid.UUID) (AcquireLockResult, error) {
	var (
		prevLockedBy    *uuid.UUID
		prevLockedUntil *time.Time
	)

	err := db.QueryRow(ctx, `
		WITH prev AS (
			SELECT locked_by, locked_until
			FROM   order_items
			WHERE  id = $1
		)
		UPDATE order_items
		SET    locked_by    = $2,
		       locked_until = NOW() + $3::interval
		WHERE  id = $1
		RETURNING
			(SELECT locked_by    FROM prev),
			(SELECT locked_until FROM prev)
	`, itemID, userID, softLockDuration.String()).Scan(&prevLockedBy, &prevLockedUntil)

	if err == pgx.ErrNoRows {
		return AcquireLockResult{}, fmt.Errorf("order_item %s not found", itemID)
	}
	if err != nil {
		return AcquireLockResult{}, fmt.Errorf("ws.AcquireLock: %w", err)
	}

	var res AcquireLockResult
	if prevLockedBy != nil && *prevLockedBy != userID {
		// Previous lock belongs to a different user.
		// Check it hasn't expired — if it has, it's not a real conflict.
		if prevLockedUntil != nil && prevLockedUntil.After(time.Now()) {
			res.PrevLockedBy = *prevLockedBy
			res.PrevLockedUntil = prevLockedUntil
			res.Conflict = true
		}
	}
	return res, nil
}
