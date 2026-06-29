package guest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

// CatalogProduct is the public view of a product for guest browsing.
// Fields match exactly what the products table provides (no description/image_url — not in schema).
// CategoryID is *uuid.UUID (nullable) because products can have NULL category_id.
type CatalogProduct struct {
	ID           uuid.UUID  `json:"id"`
	Name         string     `json:"name"`
	Price        float64    `json:"price"`
	Unit         string     `json:"unit"`
	CategoryID   *uuid.UUID `json:"category_id"` // nullable — product may have no category
	CategoryName string     `json:"category_name"`
	StockQty     float64   `json:"stock_qty"`
}

// GuestCatalog is the full catalog view for a guest.
type GuestCatalog struct {
	ProducerID   uuid.UUID        `json:"producer_id"`
	ProducerName string           `json:"producer_name"`
	Currency     string           `json:"currency"`
	Products     []CatalogProduct `json:"products"`
}

// CartItem is a single line in a guest cart.
type CartItem struct {
	ProductID uuid.UUID `json:"product_id"`
	Name      string    `json:"name"`
	Price     float64   `json:"price"`
	Qty       float64   `json:"qty"`
}

// GuestSession represents a guest cart stored in guest_sessions.
type GuestSession struct {
	ID         uuid.UUID  `json:"id"`
	ProducerID uuid.UUID  `json:"producer_id"`
	Items      []CartItem `json:"items"`
	ExpiresAt  time.Time  `json:"expires_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service { return &Service{db: db} }

// GetCatalog returns all active products for a producer identified by guest_token.
// Uses service-role DB access (no RLS) since the endpoint is public.
func (s *Service) GetCatalog(ctx context.Context, guestToken string) (*GuestCatalog, error) {
	var producerID uuid.UUID
	var name, currency string
	err := s.db.QueryRow(ctx, `
		SELECT id, COALESCE(name,''), catalog_currency
		FROM producers WHERE guest_token = $1
	`, guestToken).Scan(&producerID, &name, &currency)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("guest.GetCatalog lookup: %w", err)
	}

	rows, err := s.db.Query(ctx, `
		SELECT p.id, p.name, p.price,
		       COALESCE(p.unit,'шт'), p.category_id,
		       COALESCE(c.name,''),
		       COALESCE(p.stock_qty, 0)
		FROM products p
		LEFT JOIN categories c ON c.id = p.category_id
		WHERE p.producer_id = $1 AND p.is_active = TRUE
		ORDER BY c.name, p.name
	`, producerID)
	if err != nil {
		return nil, fmt.Errorf("guest.GetCatalog query: %w", err)
	}
	defer rows.Close()

	catalog := &GuestCatalog{
		ProducerID:   producerID,
		ProducerName: name,
		Currency:     currency,
		Products:     []CatalogProduct{},
	}
	for rows.Next() {
		var p CatalogProduct
		// CategoryID scanned as *uuid.UUID to handle NULL (products without category).
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Price,
			&p.Unit, &p.CategoryID, &p.CategoryName,
			&p.StockQty,
		); err != nil {
			return nil, err
		}
		catalog.Products = append(catalog.Products, p)
	}
	return catalog, rows.Err()
}

// AddToCart creates a new guest session or updates an existing one.
// If sessionID is nil, a new session is created.
// Returns the updated session.
func (s *Service) AddToCart(ctx context.Context, guestToken string, sessionID *uuid.UUID, item CartItem) (*GuestSession, error) {
	// Resolve producer.
	var producerID uuid.UUID
	err := s.db.QueryRow(ctx, `
		SELECT id FROM producers WHERE guest_token = $1
	`, guestToken).Scan(&producerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	// Fetch or create session inside a transaction to prevent concurrent overwrites.
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var sess GuestSession
	var cartRaw []byte

	if sessionID != nil {
		// SELECT FOR UPDATE locks the row for the duration of this transaction.
		err = tx.QueryRow(ctx, `
			SELECT id, producer_id, cart_jsonb, expires_at, created_at
			FROM guest_sessions
			WHERE id = $1 AND producer_id = $2 AND expires_at > NOW()
			FOR UPDATE
		`, sessionID, producerID).Scan(
			&sess.ID, &sess.ProducerID, &cartRaw, &sess.ExpiresAt, &sess.CreatedAt,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			sessionID = nil
		} else if err != nil {
			return nil, err
		}
	}

	if sessionID == nil {
		sess.ID = uuid.New()
		sess.ProducerID = producerID
		sess.Items = []CartItem{}
		cartRaw, _ = json.Marshal(sess.Items)
		err = tx.QueryRow(ctx, `
			INSERT INTO guest_sessions (id, producer_id, cart_jsonb)
			VALUES ($1, $2, $3)
			RETURNING expires_at, created_at
		`, sess.ID, producerID, cartRaw).Scan(&sess.ExpiresAt, &sess.CreatedAt)
		if err != nil {
			return nil, err
		}
	} else {
		if err := json.Unmarshal(cartRaw, &sess.Items); err != nil {
			sess.Items = []CartItem{}
		}
	}

	// Validate price and name from DB catalog — never trust client-supplied values.
	// Client-supplied price could be manipulated (e.g. price: 0 or price: -1).
	if item.ProductID != uuid.Nil && item.Qty > 0 {
		var realPrice float64
		var realName string
		err := tx.QueryRow(ctx, `
			SELECT price, name FROM products
			WHERE id=$1 AND producer_id=$2 AND is_active=TRUE
		`, item.ProductID, producerID).Scan(&realPrice, &realName)
		if err == nil {
			item.Price = realPrice
			item.Name = realName
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("guest.AddToCart validate product: %w", err)
		}
	}

	// Upsert item (merge by product_id).
	found := false
	for i, ci := range sess.Items {
		if ci.ProductID == item.ProductID {
			sess.Items[i].Qty = item.Qty
			sess.Items[i].Price = item.Price
			sess.Items[i].Name = item.Name
			if item.Qty <= 0 {
				sess.Items = append(sess.Items[:i], sess.Items[i+1:]...)
			}
			found = true
			break
		}
	}
	if !found && item.Qty > 0 {
		sess.Items = append(sess.Items, item)
	}

	updatedCart, _ := json.Marshal(sess.Items)
	if _, err = tx.Exec(ctx, `
		UPDATE guest_sessions SET cart_jsonb = $1 WHERE id = $2
	`, updatedCart, sess.ID); err != nil {
		return nil, err
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &sess, nil
}

// GetCart returns an existing guest session by ID.
func (s *Service) GetCart(ctx context.Context, sessionID uuid.UUID) (*GuestSession, error) {
	var sess GuestSession
	var cartRaw []byte
	err := s.db.QueryRow(ctx, `
		SELECT id, producer_id, cart_jsonb, expires_at, created_at
		FROM guest_sessions WHERE id = $1 AND expires_at > NOW()
	`, sessionID).Scan(
		&sess.ID, &sess.ProducerID, &cartRaw, &sess.ExpiresAt, &sess.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(cartRaw, &sess.Items); err != nil {
		sess.Items = []CartItem{}
	}
	return &sess, nil
}
