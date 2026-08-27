package implementation

import (
	"context"
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
	_, err = pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS implementation_history(id TEXT PRIMARY KEY,project_id TEXT NOT NULL,kind TEXT NOT NULL,source_id TEXT NOT NULL,result_id TEXT NOT NULL,revision TEXT NOT NULL,actor_id TEXT NOT NULL,target TEXT NOT NULL,status TEXT NOT NULL,payload_hash TEXT NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()); CREATE INDEX IF NOT EXISTS implementation_history_project_idx ON implementation_history(project_id,created_at);`)
	if err != nil {
		pool.Close()
		return nil, err
	}
	return store, nil
}
func (s *PostgreSQLStore) Close() { s.pool.Close() }
func (s *PostgreSQLStore) Append(ctx context.Context, item HistoryItem) (HistoryItem, error) {
	if item.ID == "" || item.ProjectID == "" {
		return HistoryItem{}, ErrInvalid
	}
	err := s.pool.QueryRow(ctx, `INSERT INTO implementation_history(id,project_id,kind,source_id,result_id,revision,actor_id,target,status,payload_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING created_at`, item.ID, item.ProjectID, item.Kind, item.SourceID, item.ResultID, item.Revision, item.ActorID, item.Target, item.Status, item.PayloadHash).Scan(&item.CreatedAt)
	return item, err
}
func (s *PostgreSQLStore) History(ctx context.Context, projectID string) ([]HistoryItem, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,project_id,kind,source_id,result_id,revision,actor_id,target,status,payload_hash,created_at FROM implementation_history WHERE project_id=$1 ORDER BY created_at,id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HistoryItem
	for rows.Next() {
		var item HistoryItem
		if err := rows.Scan(&item.ID, &item.ProjectID, &item.Kind, &item.SourceID, &item.ResultID, &item.Revision, &item.ActorID, &item.Target, &item.Status, &item.PayloadHash, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	return out, nil
}

type MemoryStore struct {
	mu    sync.RWMutex
	items []HistoryItem
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{} }
func (s *MemoryStore) Append(_ context.Context, item HistoryItem) (HistoryItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if item.ID == "" || item.ProjectID == "" {
		return HistoryItem{}, ErrInvalid
	}
	item.CreatedAt = time.Now().UTC()
	s.items = append(s.items, item)
	return item, nil
}
func (s *MemoryStore) History(_ context.Context, projectID string) ([]HistoryItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []HistoryItem
	for _, item := range s.items {
		if item.ProjectID == projectID {
			out = append(out, item)
		}
	}
	return out, nil
}
