package analytics

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Dashboard is the producer analytics summary (ТЗ раздел 9).
type Dashboard struct {
	ProducerID      uuid.UUID    `json:"producer_id"`
	Period          string       `json:"period"` // "30d"
	OrderCount      int          `json:"order_count"`
	OrderTotal      float64      `json:"order_total"`
	ActiveChats     int          `json:"active_chats"`
	TotalReceivables float64     `json:"total_receivables"`
	AvgOrderValue   float64      `json:"avg_order_value"`
	TopClients      []TopClient  `json:"top_clients"`
	GeneratedAt     time.Time    `json:"generated_at"`
}

type TopClient struct {
	ChatID         uuid.UUID `json:"chat_id"`
	OrderCount     int       `json:"order_count"`
	OrderTotal     float64   `json:"order_total"`
	TotalReceivables float64 `json:"total_receivables"`
}

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service { return &Service{db: db} }

// GetDashboard returns a 30-day analytics snapshot for the given producer.
func (s *Service) GetDashboard(ctx context.Context, producerID uuid.UUID) (*Dashboard, error) {
	d := &Dashboard{
		ProducerID:  producerID,
		Period:      "30d",
		GeneratedAt: time.Now().UTC(),
	}

	// Aggregate orders confirmed in the last 30 days.
	err := s.db.QueryRow(ctx, `
		SELECT
			COUNT(*)::INT,
			COALESCE(SUM(o.total), 0),
			COALESCE(AVG(o.total), 0)
		FROM orders o
		JOIN chats c ON c.id = o.chat_id
		WHERE c.producer_id = $1
		  AND o.status = 'confirmed'
		  AND o.confirmed_at >= NOW() - INTERVAL '30 days'
	`, producerID).Scan(&d.OrderCount, &d.OrderTotal, &d.AvgOrderValue)
	if err != nil {
		return nil, err
	}

	// Count active chats (non-archived).
	err = s.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM chats WHERE producer_id = $1
	`, producerID).Scan(&d.ActiveChats)
	if err != nil {
		return nil, err
	}

	// Total receivables = sum of all pending/confirmed debt with positive balance.
	err = s.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(cm.total_receivables), 0)
		FROM client_metrics cm
		JOIN chats c ON c.id = cm.chat_id
		WHERE c.producer_id = $1
	`, producerID).Scan(&d.TotalReceivables)
	if err != nil {
		return nil, err
	}

	// Top 5 clients by order total (last 30 days).
	rows, err := s.db.Query(ctx, `
		SELECT
			o.chat_id,
			COUNT(*)::INT       AS order_count,
			COALESCE(SUM(o.total), 0) AS order_total,
			COALESCE(cm.total_receivables, 0)
		FROM orders o
		JOIN chats c ON c.id = o.chat_id
		LEFT JOIN client_metrics cm ON cm.chat_id = o.chat_id
		WHERE c.producer_id = $1
		  AND o.status = 'confirmed'
		  AND o.confirmed_at >= NOW() - INTERVAL '30 days'
		GROUP BY o.chat_id, cm.total_receivables
		ORDER BY order_total DESC
		LIMIT 5
	`, producerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var tc TopClient
		if err := rows.Scan(&tc.ChatID, &tc.OrderCount, &tc.OrderTotal, &tc.TotalReceivables); err != nil {
			return nil, err
		}
		d.TopClients = append(d.TopClients, tc)
	}
	if d.TopClients == nil {
		d.TopClients = []TopClient{}
	}
	return d, rows.Err()
}
