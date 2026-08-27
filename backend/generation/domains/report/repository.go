package report

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
	_, err = pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS evaluation_reports(id TEXT PRIMARY KEY,evaluation_id TEXT NOT NULL,document_name TEXT NOT NULL,document_version INTEGER NOT NULL,status TEXT NOT NULL,verdict TEXT NOT NULL,report JSONB NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()); CREATE INDEX IF NOT EXISTS evaluation_reports_evaluation_idx ON evaluation_reports(evaluation_id,created_at);`)
	if err != nil {
		pool.Close()
		return nil, err
	}
	return store, nil
}
func (s *PostgreSQLStore) Close() { s.pool.Close() }
func (s *PostgreSQLStore) Save(ctx context.Context, value Report) (Report, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return Report{}, err
	}
	err = s.pool.QueryRow(ctx, `INSERT INTO evaluation_reports(id,evaluation_id,document_name,document_version,status,verdict,report) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING created_at`, value.ID, value.EvaluationID, value.DocumentName, value.DocumentVersion, value.EvaluationStatus, value.Verdict, payload).Scan(&value.CreatedAt)
	return value, err
}
func (s *PostgreSQLStore) Get(ctx context.Context, id string) (Report, error) {
	var payload []byte
	if err := s.pool.QueryRow(ctx, `SELECT report FROM evaluation_reports WHERE id=$1`, id).Scan(&payload); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Report{}, ErrNotFound
		}
		return Report{}, err
	}
	var value Report
	if err := json.Unmarshal(payload, &value); err != nil {
		return Report{}, err
	}
	return value, nil
}

type MemoryStore struct {
	mu    sync.RWMutex
	items map[string]Report
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{items: map[string]Report{}} }
func (s *MemoryStore) Save(_ context.Context, value Report) (Report, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value.CreatedAt = time.Now().UTC()
	s.items[value.ID] = value
	return value, nil
}
func (s *MemoryStore) Get(_ context.Context, id string) (Report, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.items[id]
	if !ok {
		return Report{}, ErrNotFound
	}
	return value, nil
}
