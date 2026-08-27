package design

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"sync"
)

var ErrNotFound = errors.New("product design not found")

type Store interface {
	Save(context.Context, ProductDesign) (ProductDesign, error)
	Get(context.Context, string) (ProductDesign, error)
}
type PostgreSQLStore struct{ pool *pgxpool.Pool }

func NewPostgreSQLStore(ctx context.Context, url string) (*PostgreSQLStore, error) {
	pool, e := pgxpool.New(ctx, url)
	if e != nil {
		return nil, e
	}
	_, e = pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS product_designs(id TEXT PRIMARY KEY,plan_id TEXT NOT NULL,design JSONB NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()); CREATE INDEX IF NOT EXISTS product_designs_plan_idx ON product_designs(plan_id)`)
	if e != nil {
		pool.Close()
		return nil, e
	}
	return &PostgreSQLStore{pool}, nil
}
func (s *PostgreSQLStore) Close() { s.pool.Close() }
func (s *PostgreSQLStore) Save(ctx context.Context, v ProductDesign) (ProductDesign, error) {
	raw, e := json.Marshal(v)
	if e != nil {
		return v, e
	}
	_, e = s.pool.Exec(ctx, `INSERT INTO product_designs(id,plan_id,design) VALUES($1,$2,$3) ON CONFLICT(id) DO UPDATE SET design=EXCLUDED.design`, v.ID, v.PlanID, raw)
	return v, e
}
func (s *PostgreSQLStore) Get(ctx context.Context, id string) (ProductDesign, error) {
	var raw []byte
	if e := s.pool.QueryRow(ctx, `SELECT design FROM product_designs WHERE id=$1`, id).Scan(&raw); e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return ProductDesign{}, ErrNotFound
		}
		return ProductDesign{}, e
	}
	var v ProductDesign
	e := json.Unmarshal(raw, &v)
	return v, e
}

type MemoryStore struct {
	mu    sync.RWMutex
	items map[string]ProductDesign
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{items: map[string]ProductDesign{}} }
func (s *MemoryStore) Save(_ context.Context, v ProductDesign) (ProductDesign, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[v.ID] = v
	return v, nil
}
func (s *MemoryStore) Get(_ context.Context, id string) (ProductDesign, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.items[id]
	if !ok {
		return ProductDesign{}, ErrNotFound
	}
	return v, nil
}
