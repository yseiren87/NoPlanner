package planning

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"sync"
)

var ErrNotFound = errors.New("autonomous plan not found")

type Store interface {
	Save(context.Context, AutonomousPlan) (AutonomousPlan, error)
	Get(context.Context, string) (AutonomousPlan, error)
}
type PostgreSQLStore struct{ pool *pgxpool.Pool }

func NewPostgreSQLStore(ctx context.Context, url string) (*PostgreSQLStore, error) {
	pool, e := pgxpool.New(ctx, url)
	if e != nil {
		return nil, e
	}
	_, e = pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS autonomous_plans(id TEXT PRIMARY KEY, plan JSONB NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`)
	if e != nil {
		pool.Close()
		return nil, e
	}
	return &PostgreSQLStore{pool}, nil
}
func (s *PostgreSQLStore) Close() { s.pool.Close() }
func (s *PostgreSQLStore) Save(ctx context.Context, v AutonomousPlan) (AutonomousPlan, error) {
	raw, e := json.Marshal(v)
	if e != nil {
		return v, e
	}
	_, e = s.pool.Exec(ctx, `INSERT INTO autonomous_plans(id,plan,created_at) VALUES($1,$2,$3) ON CONFLICT(id) DO UPDATE SET plan=EXCLUDED.plan`, v.ID, raw, v.CreatedAt)
	return v, e
}
func (s *PostgreSQLStore) Get(ctx context.Context, id string) (AutonomousPlan, error) {
	var raw []byte
	if e := s.pool.QueryRow(ctx, `SELECT plan FROM autonomous_plans WHERE id=$1`, id).Scan(&raw); e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return AutonomousPlan{}, ErrNotFound
		}
		return AutonomousPlan{}, e
	}
	var v AutonomousPlan
	e := json.Unmarshal(raw, &v)
	return v, e
}

type MemoryStore struct {
	mu    sync.RWMutex
	items map[string]AutonomousPlan
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{items: map[string]AutonomousPlan{}} }
func (s *MemoryStore) Save(_ context.Context, v AutonomousPlan) (AutonomousPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[v.ID] = v
	return v, nil
}
func (s *MemoryStore) Get(_ context.Context, id string) (AutonomousPlan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.items[id]
	if !ok {
		return AutonomousPlan{}, ErrNotFound
	}
	return v, nil
}
