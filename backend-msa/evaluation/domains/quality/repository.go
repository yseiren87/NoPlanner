package quality

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"sync"
	"time"
)

type PostgreSQLStore struct{ pool *pgxpool.Pool }

func NewPostgreSQLStore(ctx context.Context, databaseURL string) (*PostgreSQLStore, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	store := &PostgreSQLStore{pool: pool}
	_, err = pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS quality_runs(id TEXT PRIMARY KEY,dataset_version TEXT NOT NULL,model TEXT NOT NULL,prompt_version TEXT NOT NULL,criteria_version TEXT NOT NULL,release_gate_passed BOOLEAN NOT NULL,payload JSONB NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()); CREATE INDEX IF NOT EXISTS quality_runs_versions_idx ON quality_runs(dataset_version,criteria_version,created_at);`)
	if err != nil {
		pool.Close()
		return nil, err
	}
	return store, nil
}

type MemoryStore struct {
	mu    sync.RWMutex
	items map[string]Run
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{items: map[string]Run{}} }
func (s *MemoryStore) Save(_ context.Context, run Run) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run.CreatedAt = time.Now().UTC()
	s.items[run.ID] = run
	return run, nil
}
func (s *MemoryStore) Get(_ context.Context, id string) (Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, ok := s.items[id]
	if !ok {
		return Run{}, ErrNotFound
	}
	return run, nil
}
func (s *PostgreSQLStore) Close() { s.pool.Close() }
func (s *PostgreSQLStore) Save(ctx context.Context, run Run) (Run, error) {
	payload, err := json.Marshal(run)
	if err != nil {
		return Run{}, err
	}
	err = s.pool.QueryRow(ctx, `INSERT INTO quality_runs(id,dataset_version,model,prompt_version,criteria_version,release_gate_passed,payload) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING created_at`, run.ID, run.DatasetVersion, run.Model, run.PromptVersion, run.CriteriaVersion, run.ReleaseGatePassed, payload).Scan(&run.CreatedAt)
	return run, err
}
func (s *PostgreSQLStore) Get(ctx context.Context, id string) (Run, error) {
	var payload []byte
	var createdAt time.Time
	err := s.pool.QueryRow(ctx, `SELECT payload,created_at FROM quality_runs WHERE id=$1`, id).Scan(&payload, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	if err != nil {
		return Run{}, err
	}
	var run Run
	if err := json.Unmarshal(payload, &run); err != nil {
		return Run{}, err
	}
	run.CreatedAt = createdAt
	return run, nil
}
