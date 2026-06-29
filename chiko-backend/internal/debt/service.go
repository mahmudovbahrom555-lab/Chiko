package debt

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/chiko/backend/internal/ws"
)

// Service implements all debt / return business logic.
// All writes use INSERT only — no UPDATE/DELETE on debt_transactions
// (enforced by DB trigger fn_debt_immutable_fields, migration 003).
type Service struct {
	db  *pgxpool.Pool
	hub *ws.Hub
}

func NewService(db *pgxpool.Pool, hub *ws.Hub) *Service {
	return &Service{db: db, hub: hub}
}

// ──────────────────────────── BALANCE ────────────────────────────────────────

// GetBalance computes the current debt for a chat.
// Formula (ТЗ раздел 6.2, КРИТИЧНО — тесты обязательны):
//   current_debt = SUM(amount * sign) WHERE status IN ('pending', 'confirmed')
//   Disputed записи НЕ входят.
//   Отрицательный результат = предоплата (Credit).
func (s *Service) GetBalance(ctx context.Context, chatID, callerID uuid.UUID) (Balance, error) {
	if err := s.validateChatParticipant(ctx, chatID, callerID); err != nil {
		return Balance{}, err
	}
	var b Balance
	b.ChatID = chatID

	err := s.db.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(amount * sign) FILTER (WHERE status IN ('pending','confirmed')), 0),
			COALESCE(SUM(amount * sign) FILTER (WHERE status = 'pending'), 0) != 0,
			COALESCE(p.catalog_currency, 'UZS')
		FROM  debt_transactions dt
		JOIN  chats c ON c.id = dt.chat_id
		JOIN  producers p ON p.id = c.producer_id
		WHERE dt.chat_id = $1
		GROUP BY p.catalog_currency
	`, chatID).Scan(&b.Balance, &b.HasPending, &b.Currency)

	if err == pgx.ErrNoRows {
		// No transactions yet — zero balance.
		// Still need currency from producer.
		s.db.QueryRow(ctx, `
			SELECT COALESCE(p.catalog_currency, 'UZS')
			FROM   chats c
			JOIN   producers p ON p.id = c.producer_id
			WHERE  c.id = $1
		`, chatID).Scan(&b.Currency)
		return b, nil
	}
	if err != nil {
		return Balance{}, fmt.Errorf("debt.GetBalance: %w", err)
	}
	return b, nil
}

// ListHistory returns all debt_transactions for a chat in chronological order.
func (s *Service) ListHistory(ctx context.Context, chatID, callerID uuid.UUID) ([]Tx, error) {
	if err := s.validateChatParticipant(ctx, chatID, callerID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, chat_id, type, amount, sign, initiator_id,
		       confirmed_by_id, confirmed_at, status, comment, created_at
		FROM   debt_transactions
		WHERE  chat_id = $1
		ORDER  BY created_at
	`, chatID)
	if err != nil {
		return nil, fmt.Errorf("debt.ListHistory: %w", err)
	}
	defer rows.Close()

	var txs []Tx
	for rows.Next() {
		var t Tx
		if err := rows.Scan(
			&t.ID, &t.ChatID, &t.Type, &t.Amount, &t.Sign,
			&t.InitiatorID, &t.ConfirmedByID, &t.ConfirmedAt,
			&t.Status, &t.Comment, &t.CreatedAt,
		); err != nil {
			return nil, err
		}
		txs = append(txs, t)
	}
	return txs, rows.Err()
}

// ──────────────────────────── CREATE ─────────────────────────────────────────

// CreateDelivery — producer records a goods delivery (sign=+1, debt grows).
// Starts as Pending; client confirms reception.
func (s *Service) CreateDelivery(ctx context.Context, in CreateDeliveryInput, initiatorID uuid.UUID) (Tx, error) {
	if err := s.validateChatParticipant(ctx, in.ChatID, initiatorID); err != nil {
		return Tx{}, err
	}
	if in.Amount <= 0 {
		return Tx{}, errValidation("amount must be > 0")
	}
	// Delivery must be initiated by the producer.
	if err := s.requireProducer(ctx, in.ChatID, initiatorID); err != nil {
		return Tx{}, err
	}
	t, err := s.insertTx(ctx, in.ChatID, TypeDelivery, in.Amount, 1, initiatorID, StatusPending, in.Comment)
	if err != nil {
		return Tx{}, err
	}
	s.broadcastDebt(t, ws.EventDebtCreated)
	return t, nil
}

// CreatePayment — either side records a payment (sign=-1, debt falls).
// Starts as Pending; the OTHER side confirms.
func (s *Service) CreatePayment(ctx context.Context, in CreatePaymentInput, initiatorID uuid.UUID) (Tx, error) {
	if err := s.validateChatParticipant(ctx, in.ChatID, initiatorID); err != nil {
		return Tx{}, err
	}
	if in.Amount <= 0 {
		return Tx{}, errValidation("amount must be > 0")
	}
	t, err := s.insertTx(ctx, in.ChatID, TypePayment, in.Amount, -1, initiatorID, StatusPending, in.Comment)
	if err != nil {
		return Tx{}, err
	}
	s.broadcastDebt(t, ws.EventDebtCreated)
	return t, nil
}

// ──────────────────────────── STATUS TRANSITIONS ─────────────────────────────

// Confirm transitions a pending transaction → confirmed.
// The confirmer must be the OPPOSITE party (no self-confirm).
func (s *Service) Confirm(ctx context.Context, txID, callerID uuid.UUID) (Tx, error) {
	t, err := s.loadTx(ctx, txID)
	if err != nil {
		return Tx{}, err
	}
	// Verify caller is a participant of this chat BEFORE any status check.
	if err := s.validateChatParticipant(ctx, t.ChatID, callerID); err != nil {
		return Tx{}, err
	}
	if t.Status != StatusPending {
		return Tx{}, errValidation("only pending transactions can be confirmed")
	}
	// Self-confirm prevention (ТЗ раздел 6.3 — подтверждает ДРУГАЯ сторона).
	if t.InitiatorID == callerID {
		return Tx{}, errValidation("cannot confirm your own transaction")
	}

	now := time.Now()
	t, err = s.updateStatus(ctx, txID, StatusConfirmed, callerID, &now)
	if err != nil {
		return Tx{}, err
	}
	s.broadcastDebt(t, ws.EventDebtConfirmed)
	s.updatePaymentDelay(ctx, t)
	return t, nil
}

// Dispute transitions a pending transaction → disputed.
// The disputer must be a participant (but not necessarily the opposite party).
func (s *Service) Dispute(ctx context.Context, txID, callerID uuid.UUID) (Tx, error) {
	t, err := s.loadTx(ctx, txID)
	if err != nil {
		return Tx{}, err
	}
	if err := s.validateChatParticipant(ctx, t.ChatID, callerID); err != nil {
		return Tx{}, err
	}
	if t.Status != StatusPending {
		return Tx{}, errValidation("only pending transactions can be disputed")
	}
	if t.InitiatorID == callerID {
		return Tx{}, errValidation("cannot dispute your own transaction")
	}
	t, err = s.updateStatus(ctx, txID, StatusDisputed, callerID, nil)
	if err != nil {
		return Tx{}, err
	}
	s.broadcastDebt(t, ws.EventDebtDisputed)
	s.incrementDisputeCount(ctx, t.ChatID)
	return t, nil
}

// ──────────────────────────── RETURN FLOW ────────────────────────────────────

// CreateReturnRequest — client reports damaged/wrong goods (ТЗ раздел 7 шаг 1).
func (s *Service) CreateReturnRequest(ctx context.Context, in CreateReturnRequestInput, callerID uuid.UUID) (ReturnRequest, error) {
	if err := s.requireClient(ctx, in.ChatID, callerID); err != nil {
		return ReturnRequest{}, err
	}
	if in.OrderID == uuid.Nil {
		return ReturnRequest{}, errValidation("order_id is required")
	}

	items := in.ItemsJSON
	if len(items) == 0 {
		items = []byte("[]")
	}

	var rr ReturnRequest
	err := s.db.QueryRow(ctx, `
		INSERT INTO return_requests (chat_id, order_id, items_jsonb, photo_urls, created_by)
		VALUES ($1, $2, $3::jsonb, $4, $5)
		RETURNING id, chat_id, order_id, items_jsonb, photo_urls,
		          status, escalated, created_by, resolved_at, created_at
	`, in.ChatID, in.OrderID, items, in.PhotoURLs, callerID).Scan(
		&rr.ID, &rr.ChatID, &rr.OrderID, &rr.ItemsJSON, &rr.PhotoURLs,
		&rr.Status, &rr.Escalated, &rr.CreatedBy, &rr.ResolvedAt, &rr.CreatedAt,
	)
	if err != nil {
		return ReturnRequest{}, fmt.Errorf("debt.CreateReturnRequest: %w", err)
	}
	return rr, nil
}

// CreateReturnCorrection — producer resolves a return request (ТЗ раздел 7 шаг 2).
// Creates a debt_transaction with type=return_correction, status=confirmed IMMEDIATELY
// (DB constraint debt_return_correction_confirmed enforces this).
// The return_request is marked as resolved.
func (s *Service) CreateReturnCorrection(ctx context.Context, in CreateReturnCorrectionInput, producerID uuid.UUID) (Tx, error) {
	if in.Amount <= 0 {
		return Tx{}, errValidation("amount must be > 0")
	}

	// Load the return_request to get chat_id and validate it belongs to this producer.
	var rr ReturnRequest
	err := s.db.QueryRow(ctx, `
		SELECT rr.id, rr.chat_id, rr.status
		FROM   return_requests rr
		JOIN   chats c ON c.id = rr.chat_id
		WHERE  rr.id = $1 AND c.producer_id = $2
	`, in.ReturnRequestID, producerID).Scan(&rr.ID, &rr.ChatID, &rr.Status)
	if err == pgx.ErrNoRows {
		return Tx{}, errValidation("return request not found or access denied")
	}
	if err != nil {
		return Tx{}, fmt.Errorf("debt.CreateReturnCorrection load: %w", err)
	}
	if rr.Status == ReturnResolved {
		return Tx{}, errValidation("return request is already resolved")
	}

	// Wrap INSERT + UPDATE in a transaction: partial failure would create a
	// debt_transaction without resolving the return_request → duplicate corrections.
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Tx{}, fmt.Errorf("debt.CreateReturnCorrection begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Insert return_correction as CONFIRMED immediately (ТЗ раздел 7).
	now := time.Now()
	t, err := s.insertTxWithStatusTx(ctx, tx, rr.ChatID, TypeReturnCorrection, in.Amount, -1,
		producerID, StatusConfirmed, in.Comment, &producerID, &now)
	if err != nil {
		return Tx{}, err
	}

	// Mark return_request resolved and link to the created transaction.
	if _, err := tx.Exec(ctx, `
		UPDATE return_requests
		SET    status = 'resolved',
		       resolved_at = NOW(),
		       resulting_transaction_id = $2
		WHERE  id = $1
	`, in.ReturnRequestID, t.ID); err != nil {
		return Tx{}, fmt.Errorf("debt.CreateReturnCorrection resolve: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Tx{}, fmt.Errorf("debt.CreateReturnCorrection commit: %w", err)
	}

	s.broadcastDebt(t, ws.EventDebtConfirmed)
	return t, nil
}

// DisputeCorrection — client disagrees with the producer's return correction.
// Transitions the debt_transaction to disputed AND the return_request to disputed.
func (s *Service) DisputeCorrection(ctx context.Context, txID, callerID uuid.UUID) (Tx, error) {
	t, err := s.loadTx(ctx, txID)
	if err != nil {
		return Tx{}, err
	}
	if t.Type != TypeReturnCorrection {
		return Tx{}, errValidation("only return_correction transactions can be disputed this way")
	}
	if t.Status == StatusDisputed {
		return Tx{}, errValidation("already disputed")
	}
	// Client must be participant.
	if err := s.requireClient(ctx, t.ChatID, callerID); err != nil {
		return Tx{}, err
	}

	t, err = s.updateStatus(ctx, txID, StatusDisputed, callerID, nil)
	if err != nil {
		return Tx{}, err
	}

	// Mark the corresponding return_request as disputed via resulting_transaction_id
	// (direct FK link, not time-based heuristic — avoids matching wrong return_request
	// when two corrections happen within the same minute in one chat).
	if _, err := s.db.Exec(ctx, `
		UPDATE return_requests SET status='disputed'
		WHERE  resulting_transaction_id = $1
	`, txID); err != nil {
		log.Error().Err(err).Str("tx", txID.String()).
			Msg("debt: failed to mark return_request disputed")
	}

	s.broadcastDebt(t, ws.EventDebtDisputed)
	s.incrementDisputeCount(ctx, t.ChatID)
	return t, nil
}

// ──────────────────────────── SLA ────────────────────────────────────────────

// EscalatedReturn carries the IDs needed by the SLA job to send push notifications.
type EscalatedReturn struct {
	ChatID     uuid.UUID
	ProducerID uuid.UUID
}

// EscalateOverdueReturns marks overdue pending return_requests as 'attention'
// and returns info needed for push notifications.
func (s *Service) EscalateOverdueReturns(ctx context.Context, slaHours int) ([]EscalatedReturn, error) {
	if slaHours <= 0 {
		slaHours = 48
	}
	rows, err := s.db.Query(ctx, `
		UPDATE return_requests rr
		SET    status    = 'attention',
		       escalated = TRUE
		FROM   chats c
		WHERE  rr.chat_id   = c.id
		  AND  rr.status    = 'pending'
		  AND  rr.escalated = FALSE
		  AND  rr.created_at < NOW() - ($1 || ' hours')::interval
		RETURNING rr.chat_id, c.producer_id
	`, slaHours)
	if err != nil {
		return nil, fmt.Errorf("debt.EscalateOverdueReturns: %w", err)
	}
	defer rows.Close()

	var result []EscalatedReturn
	for rows.Next() {
		var e EscalatedReturn
		if err := rows.Scan(&e.ChatID, &e.ProducerID); err != nil {
			return result, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

// ──────────────────────────── client_metrics helpers ─────────────────────────

// updatePaymentDelay recalculates avg payment delay after a payment is confirmed.
// payment_delay_avg_days = avg(confirmed_at - created_at) for confirmed payments.
func (s *Service) updatePaymentDelay(ctx context.Context, t Tx) {
	if t.Type != TypePayment || t.ConfirmedAt == nil {
		return
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO client_metrics (chat_id, payment_delay_avg_days, updated_at)
		VALUES ($1,
			(SELECT AVG(EXTRACT(EPOCH FROM (confirmed_at - created_at))/86400)
			 FROM   debt_transactions
			 WHERE  chat_id=$1 AND type='payment' AND status='confirmed'
			        AND confirmed_at IS NOT NULL),
			NOW()
		)
		ON CONFLICT (chat_id) DO UPDATE
		  SET payment_delay_avg_days = EXCLUDED.payment_delay_avg_days,
		      updated_at             = NOW()
	`, t.ChatID); err != nil {
		log.Error().Err(err).Str("chat", t.ChatID.String()).Msg("debt: updatePaymentDelay failed")
	}
}

func (s *Service) incrementDisputeCount(ctx context.Context, chatID uuid.UUID) {
	if _, err := s.db.Exec(ctx, `
		INSERT INTO client_metrics (chat_id, dispute_count, updated_at)
		VALUES ($1, 1, NOW())
		ON CONFLICT (chat_id) DO UPDATE
		  SET dispute_count = client_metrics.dispute_count + 1,
		      updated_at    = NOW()
	`, chatID); err != nil {
		log.Error().Err(err).Str("chat", chatID.String()).Msg("debt: incrementDisputeCount failed")
	}
}

// ──────────────────────────── internal helpers ────────────────────────────────

func (s *Service) insertTx(ctx context.Context, chatID uuid.UUID, txType string,
	amount float64, sign int, initiatorID uuid.UUID, status, comment string,
) (Tx, error) {
	return s.insertTxWithStatus(ctx, chatID, txType, amount, sign, initiatorID, status, comment, nil, nil)
}

func (s *Service) insertTxWithStatus(ctx context.Context, chatID uuid.UUID, txType string,
	amount float64, sign int, initiatorID uuid.UUID, status, comment string,
	confirmedBy *uuid.UUID, confirmedAt *time.Time,
) (Tx, error) {
	return insertTxSQL(ctx, s.db, chatID, txType, amount, sign, initiatorID, status, comment, confirmedBy, confirmedAt)
}

// insertTxWithStatusTx is the transaction-aware variant used when multiple
// writes must be atomic (e.g. CreateReturnCorrection).
func (s *Service) insertTxWithStatusTx(ctx context.Context, tx pgx.Tx, chatID uuid.UUID, txType string,
	amount float64, sign int, initiatorID uuid.UUID, status, comment string,
	confirmedBy *uuid.UUID, confirmedAt *time.Time,
) (Tx, error) {
	return insertTxSQL(ctx, tx, chatID, txType, amount, sign, initiatorID, status, comment, confirmedBy, confirmedAt)
}

// dbQuerier is the minimal interface shared by *pgxpool.Pool and pgx.Tx.
type dbQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func insertTxSQL(ctx context.Context, q dbQuerier, chatID uuid.UUID, txType string,
	amount float64, sign int, initiatorID uuid.UUID, status, comment string,
	confirmedBy *uuid.UUID, confirmedAt *time.Time,
) (Tx, error) {
	var t Tx
	err := q.QueryRow(ctx, `
		INSERT INTO debt_transactions
			(chat_id, type, amount, sign, initiator_id,
			 confirmed_by_id, confirmed_at, status, comment)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id, chat_id, type, amount, sign, initiator_id,
		          confirmed_by_id, confirmed_at, status, comment, created_at
	`, chatID, txType, amount, sign, initiatorID,
		confirmedBy, confirmedAt, status, comment,
	).Scan(
		&t.ID, &t.ChatID, &t.Type, &t.Amount, &t.Sign,
		&t.InitiatorID, &t.ConfirmedByID, &t.ConfirmedAt,
		&t.Status, &t.Comment, &t.CreatedAt,
	)
	if err != nil {
		return Tx{}, fmt.Errorf("debt.insertTx (%s): %w", txType, err)
	}
	return t, nil
}

// updateStatus modifies status, confirmed_by_id, confirmed_at on a debt_transaction.
// This is the ONLY allowed "update" — financial fields are protected by DB trigger.
func (s *Service) updateStatus(ctx context.Context, txID uuid.UUID, status string, callerID uuid.UUID, confirmedAt *time.Time) (Tx, error) {
	var t Tx
	err := s.db.QueryRow(ctx, `
		UPDATE debt_transactions
		SET    status          = $2,
		       confirmed_by_id = $3,
		       confirmed_at    = $4
		WHERE  id = $1
		RETURNING id, chat_id, type, amount, sign, initiator_id,
		          confirmed_by_id, confirmed_at, status, comment, created_at
	`, txID, status, callerID, confirmedAt).Scan(
		&t.ID, &t.ChatID, &t.Type, &t.Amount, &t.Sign,
		&t.InitiatorID, &t.ConfirmedByID, &t.ConfirmedAt,
		&t.Status, &t.Comment, &t.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return Tx{}, errValidation("transaction not found")
	}
	if err != nil {
		return Tx{}, fmt.Errorf("debt.updateStatus: %w", err)
	}
	return t, nil
}

func (s *Service) loadTx(ctx context.Context, txID uuid.UUID) (Tx, error) {
	var t Tx
	err := s.db.QueryRow(ctx, `
		SELECT id, chat_id, type, amount, sign, initiator_id,
		       confirmed_by_id, confirmed_at, status, comment, created_at
		FROM   debt_transactions WHERE id = $1
	`, txID).Scan(
		&t.ID, &t.ChatID, &t.Type, &t.Amount, &t.Sign,
		&t.InitiatorID, &t.ConfirmedByID, &t.ConfirmedAt,
		&t.Status, &t.Comment, &t.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return Tx{}, errValidation("transaction not found")
	}
	return t, err
}

// validateChatParticipant returns error if callerID is not a participant of the chat.
// DB errors are logged and treated as "not a participant" (safe fail-closed).
func (s *Service) validateChatParticipant(ctx context.Context, chatID, callerID uuid.UUID) error {
	var exists bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM chats WHERE id=$1 AND (producer_id=$2 OR client_id=$2))
	`, chatID, callerID).Scan(&exists)
	if err != nil {
		log.Warn().Err(err).Str("chat", chatID.String()).Str("caller", callerID.String()).
			Msg("debt: validateChatParticipant DB error")
		return errValidation("not a participant of this chat")
	}
	if !exists {
		return errValidation("not a participant of this chat")
	}
	return nil
}

// requireProducer returns error if callerID is not the producer of the chat.
func (s *Service) requireProducer(ctx context.Context, chatID, callerID uuid.UUID) error {
	var exists bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chats WHERE id=$1 AND producer_id=$2)`, chatID, callerID).Scan(&exists)
	if err != nil {
		log.Warn().Err(err).Str("chat", chatID.String()).Msg("debt: requireProducer DB error")
		return errValidation("only the producer can initiate delivery")
	}
	if !exists {
		return errValidation("only the producer can initiate delivery")
	}
	return nil
}

// requireClient returns error if callerID is not the client of the chat.
func (s *Service) requireClient(ctx context.Context, chatID, callerID uuid.UUID) error {
	var exists bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chats WHERE id=$1 AND client_id=$2)`, chatID, callerID).Scan(&exists)
	if err != nil {
		log.Warn().Err(err).Str("chat", chatID.String()).Msg("debt: requireClient DB error")
		return errValidation("only the client can perform this action")
	}
	if !exists {
		return errValidation("only the client can perform this action")
	}
	return nil
}

func (s *Service) broadcastDebt(t Tx, eventType ws.EventType) {
	payload := ws.DebtPayload{
		TxID:   t.ID,
		ChatID: t.ChatID,
		Type:   t.Type,
		Amount: t.Amount,
		Sign:   t.Sign,
	}
	ev, err := ws.NewEvent(eventType, payload)
	if err != nil {
		return
	}
	encoded, _ := ev.Encode()
	s.hub.Broadcast(ws.BroadcastMsg{ChatID: t.ChatID, Data: encoded})
}
