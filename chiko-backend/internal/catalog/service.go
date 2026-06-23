package catalog

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Service contains all catalog business logic.
// It talks to the DB via pgxpool, respecting RLS (uses the caller's JWT role
// via the pool's connection that was set up with the user's JWT, OR service_role
// for ops that bypass RLS).
type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

// ──────────────────────────── CATEGORIES ────────────────────────────────────

// ListCategories returns all categories for the producer.
func (s *Service) ListCategories(ctx context.Context, producerID uuid.UUID) ([]Category, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, producer_id, name, created_at
		FROM   categories
		WHERE  producer_id = $1
		ORDER  BY name
	`, producerID)
	if err != nil {
		return nil, fmt.Errorf("catalog.ListCategories: %w", err)
	}
	defer rows.Close()

	var cats []Category
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.ProducerID, &c.Name, &c.CreatedAt); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	return cats, rows.Err()
}

// CreateCategory creates a single category for the producer.
func (s *Service) CreateCategory(ctx context.Context, producerID uuid.UUID, name string) (Category, error) {
	if name == "" {
		return Category{}, errValidation("category name is required")
	}
	var c Category
	err := s.db.QueryRow(ctx, `
		INSERT INTO categories (producer_id, name)
		VALUES ($1, $2)
		RETURNING id, producer_id, name, created_at
	`, producerID, name).Scan(&c.ID, &c.ProducerID, &c.Name, &c.CreatedAt)
	if err != nil {
		return Category{}, fmt.Errorf("catalog.CreateCategory: %w", err)
	}
	return c, nil
}

// EnsureDefaultCategories creates the 9 default categories for a producer
// if they don't exist yet. Idempotent — safe to call multiple times.
// Requires migration 007 (UNIQUE (producer_id, name) on categories).
func (s *Service) EnsureDefaultCategories(ctx context.Context, producerID uuid.UUID) error {
	for _, name := range DefaultCategoryNames {
		_, err := s.db.Exec(ctx, `
			INSERT INTO categories (producer_id, name)
			VALUES ($1, $2)
			ON CONFLICT (producer_id, name) DO NOTHING
		`, producerID, name)
		if err != nil {
			return fmt.Errorf("catalog.EnsureDefaultCategories %q: %w", name, err)
		}
	}
	return nil
}

// ensureOtherCategory returns (or creates) the "Другое" category for a producer.
// Uses ON CONFLICT on the unique constraint from migration 007.
func (s *Service) ensureOtherCategory(ctx context.Context, producerID uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.db.QueryRow(ctx, `
		INSERT INTO categories (producer_id, name)
		VALUES ($1, 'Другое')
		ON CONFLICT (producer_id, name) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`, producerID).Scan(&id)
	return id, err
}

// ──────────────────────────── PRODUCTS ──────────────────────────────────────

// ListProducts returns products for a producer with optional fuzzy search and category filter.
func (s *Service) ListProducts(ctx context.Context, producerID uuid.UUID, p SearchParams) ([]Product, error) {
	if p.Limit <= 0 || p.Limit > 200 {
		p.Limit = 50
	}

	var (
		rows pgx.Rows
		err  error
	)

	if p.Query != "" && p.CategoryID != nil {
		rows, err = s.db.Query(ctx, `
			SELECT id, producer_id, category_id, name, unit, price, stock_qty, is_active, created_at, updated_at
			FROM   products
			WHERE  producer_id  = $1
			  AND  category_id  = $2
			  AND  is_active    = TRUE
			  AND  similarity(name, $3) > 0.3
			ORDER  BY similarity(name, $3) DESC
			LIMIT  $4 OFFSET $5
		`, producerID, *p.CategoryID, p.Query, p.Limit, p.Offset)
	} else if p.Query != "" {
		rows, err = s.db.Query(ctx, `
			SELECT id, producer_id, category_id, name, unit, price, stock_qty, is_active, created_at, updated_at
			FROM   products
			WHERE  producer_id = $1
			  AND  is_active   = TRUE
			  AND  similarity(name, $2) > 0.3
			ORDER  BY similarity(name, $2) DESC
			LIMIT  $3 OFFSET $4
		`, producerID, p.Query, p.Limit, p.Offset)
	} else if p.CategoryID != nil {
		rows, err = s.db.Query(ctx, `
			SELECT id, producer_id, category_id, name, unit, price, stock_qty, is_active, created_at, updated_at
			FROM   products
			WHERE  producer_id = $1
			  AND  category_id = $2
			  AND  is_active   = TRUE
			ORDER  BY name
			LIMIT  $3 OFFSET $4
		`, producerID, *p.CategoryID, p.Limit, p.Offset)
	} else {
		rows, err = s.db.Query(ctx, `
			SELECT id, producer_id, category_id, name, unit, price, stock_qty, is_active, created_at, updated_at
			FROM   products
			WHERE  producer_id = $1
			  AND  is_active   = TRUE
			ORDER  BY name
			LIMIT  $2 OFFSET $3
		`, producerID, p.Limit, p.Offset)
	}

	if err != nil {
		return nil, fmt.Errorf("catalog.ListProducts: %w", err)
	}
	defer rows.Close()

	var products []Product
	for rows.Next() {
		var pr Product
		if err := rows.Scan(
			&pr.ID, &pr.ProducerID, &pr.CategoryID,
			&pr.Name, &pr.Unit, &pr.Price, &pr.StockQty,
			&pr.IsActive, &pr.CreatedAt, &pr.UpdatedAt,
		); err != nil {
			return nil, err
		}
		products = append(products, pr)
	}
	return products, rows.Err()
}

// CreateProduct adds a product to the catalog.
func (s *Service) CreateProduct(ctx context.Context, producerID uuid.UUID, in CreateProductInput) (Product, error) {
	if err := in.Valid(); err != nil {
		return Product{}, err
	}
	var pr Product
	err := s.db.QueryRow(ctx, `
		INSERT INTO products (producer_id, category_id, name, unit, price, stock_qty)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, producer_id, category_id, name, unit, price, stock_qty, is_active, created_at, updated_at
	`, producerID, in.CategoryID, in.Name, in.Unit, in.Price, in.StockQty).Scan(
		&pr.ID, &pr.ProducerID, &pr.CategoryID,
		&pr.Name, &pr.Unit, &pr.Price, &pr.StockQty,
		&pr.IsActive, &pr.CreatedAt, &pr.UpdatedAt,
	)
	if err != nil {
		return Product{}, fmt.Errorf("catalog.CreateProduct: %w", err)
	}
	return pr, nil
}

// UpdateProduct performs a partial update on a product owned by producerID.
func (s *Service) UpdateProduct(ctx context.Context, producerID, productID uuid.UUID, in UpdateProductInput) (Product, error) {
	// Build dynamic SET clause only for provided fields.
	sets := []string{"updated_at = NOW()"}
	args := []any{producerID, productID}
	argN := 3

	if in.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", argN))
		args = append(args, *in.Name)
		argN++
	}
	if in.Unit != nil {
		sets = append(sets, fmt.Sprintf("unit = $%d", argN))
		args = append(args, *in.Unit)
		argN++
	}
	if in.Price != nil {
		if *in.Price < 0 {
			return Product{}, errValidation("price must be ≥ 0")
		}
		sets = append(sets, fmt.Sprintf("price = $%d", argN))
		args = append(args, *in.Price)
		argN++
	}
	if in.StockQty != nil {
		sets = append(sets, fmt.Sprintf("stock_qty = $%d", argN))
		args = append(args, *in.StockQty)
		argN++
	}
	if in.IsActive != nil {
		sets = append(sets, fmt.Sprintf("is_active = $%d", argN))
		args = append(args, *in.IsActive)
		argN++
	}
	if in.CategoryID != nil {
		sets = append(sets, fmt.Sprintf("category_id = $%d", argN))
		args = append(args, *in.CategoryID)
		argN++
	}

	q := fmt.Sprintf(`
		UPDATE products
		SET    %s
		WHERE  producer_id = $1 AND id = $2
		RETURNING id, producer_id, category_id, name, unit, price, stock_qty, is_active, created_at, updated_at
	`, joinSets(sets))

	var pr Product
	err := s.db.QueryRow(ctx, q, args...).Scan(
		&pr.ID, &pr.ProducerID, &pr.CategoryID,
		&pr.Name, &pr.Unit, &pr.Price, &pr.StockQty,
		&pr.IsActive, &pr.CreatedAt, &pr.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return Product{}, errValidation("product not found or access denied")
	}
	if err != nil {
		return Product{}, fmt.Errorf("catalog.UpdateProduct: %w", err)
	}
	return pr, nil
}

// DeleteProduct soft-deletes (is_active=false) a product.
func (s *Service) DeleteProduct(ctx context.Context, producerID, productID uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE products SET is_active = FALSE, updated_at = NOW()
		WHERE producer_id = $1 AND id = $2
	`, producerID, productID)
	if err != nil {
		return fmt.Errorf("catalog.DeleteProduct: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errValidation("product not found or access denied")
	}
	return nil
}

// ──────────────────────────── CURRENCY ──────────────────────────────────────

// SetCurrency updates the producer's catalog currency.
// Validation is done at DB level via FK to currencies table.
func (s *Service) SetCurrency(ctx context.Context, producerID uuid.UUID, code string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE producers SET catalog_currency = $2 WHERE id = $1
	`, producerID, code)
	if err != nil {
		return fmt.Errorf("catalog.SetCurrency: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errValidation("producer not found")
	}
	return nil
}

// SearchCurrencies returns currencies matching the query (for autocomplete).
func (s *Service) SearchCurrencies(ctx context.Context, q string) ([]map[string]string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT code, name_ru, name_en
		FROM   currencies
		WHERE  code    ILIKE $1
		    OR name_en ILIKE $1
		    OR name_ru ILIKE $1
		ORDER  BY code
		LIMIT  20
	`, "%"+q+"%")
	if err != nil {
		return nil, fmt.Errorf("catalog.SearchCurrencies: %w", err)
	}
	defer rows.Close()

	var result []map[string]string
	for rows.Next() {
		var code, nameRu, nameEn string
		if err := rows.Scan(&code, &nameRu, &nameEn); err != nil {
			return nil, err
		}
		result = append(result, map[string]string{
			"code":    code,
			"name_ru": nameRu,
			"name_en": nameEn,
		})
	}
	return result, rows.Err()
}

// ──────────────────────────── IMPORT ────────────────────────────────────────

// ImportProducts bulk-creates products from import rows.
// Unknown categories → "Другое". Returns count created.
func (s *Service) ImportProducts(ctx context.Context, producerID uuid.UUID, rows []ImportRow) (int, []string, error) {
	if len(rows) == 0 {
		return 0, nil, nil
	}

	// Build category name→ID map for this producer.
	cats, err := s.ListCategories(ctx, producerID)
	if err != nil {
		return 0, nil, err
	}
	catMap := make(map[string]uuid.UUID, len(cats))
	for _, c := range cats {
		catMap[c.Name] = c.ID
	}

	var (
		created  int
		warnings []string
	)

	for _, row := range rows {
		catID, ok := catMap[row.Category]
		if !ok {
			// Unknown or empty category → Другое
			if row.Category != "" {
				warnings = append(warnings, fmt.Sprintf("категория %q не найдена, назначено «Другое»", row.Category))
			}
			catID, err = s.ensureOtherCategory(ctx, producerID)
			if err != nil {
				return created, warnings, err
			}
			catMap[row.Category] = catID // cache
		}

		_, err := s.CreateProduct(ctx, producerID, CreateProductInput{
			CategoryID: &catID,
			Name:       row.Name,
			Unit:       row.Unit,
			Price:      row.Price,
			StockQty:   row.StockQty,
		})
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("строка %q: %v", row.Name, err))
			continue
		}
		created++
	}
	return created, warnings, nil
}

// ──────────────────────────── helpers ───────────────────────────────────────

func joinSets(sets []string) string {
	out := ""
	for i, s := range sets {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}
