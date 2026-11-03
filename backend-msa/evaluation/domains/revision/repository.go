package revision

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"time"
)

type PostgreSQLStore struct{ pool *pgxpool.Pool }

func NewPostgreSQLStore(ctx context.Context, databaseURL string) (*PostgreSQLStore, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := EnsureSchema(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return &PostgreSQLStore{pool: pool}, nil
}
func (s *PostgreSQLStore) Close() { s.pool.Close() }
func EnsureSchema(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS document_version_comparisons(id TEXT PRIMARY KEY,document_id TEXT NOT NULL,previous_version INTEGER NOT NULL,current_version INTEGER NOT NULL,comparison JSONB NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()); CREATE INDEX IF NOT EXISTS version_comparison_document_idx ON document_version_comparisons(document_id,current_version); CREATE TABLE IF NOT EXISTS objections(id TEXT PRIMARY KEY,evaluation_id TEXT NOT NULL,finding_id TEXT NOT NULL,status TEXT NOT NULL,objection JSONB NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()); CREATE TABLE IF NOT EXISTS reevaluations(id TEXT PRIMARY KEY,document_id TEXT NOT NULL,previous_evaluation_id TEXT NOT NULL,current_evaluation_id TEXT NOT NULL,previous_verdict TEXT NOT NULL,current_verdict TEXT NOT NULL,criteria_version TEXT NOT NULL,model TEXT NOT NULL,reevaluation JSONB NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()); CREATE INDEX IF NOT EXISTS reevaluation_document_idx ON reevaluations(document_id,created_at);`)
	return err
}
func (s *PostgreSQLStore) SaveComparison(ctx context.Context, value Comparison) (Comparison, error) {
	if value.ID == "" || value.DocumentID == "" {
		return Comparison{}, ErrInvalid
	}
	value.CreatedAt = time.Now().UTC()
	payload, err := json.Marshal(value)
	if err != nil {
		return Comparison{}, err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO document_version_comparisons(id,document_id,previous_version,current_version,comparison,created_at) VALUES($1,$2,$3,$4,$5,$6)`, value.ID, value.DocumentID, value.PreviousVersion, value.CurrentVersion, payload, value.CreatedAt)
	return value, err
}
func (s *PostgreSQLStore) SaveReevaluation(ctx context.Context, value Reevaluation) (Reevaluation, error) {
	if value.ID == "" || value.DocumentID == "" {
		return Reevaluation{}, ErrInvalid
	}
	value.CreatedAt = time.Now().UTC()
	payload, err := json.Marshal(value)
	if err != nil {
		return Reevaluation{}, err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO reevaluations(id,document_id,previous_evaluation_id,current_evaluation_id,previous_verdict,current_verdict,criteria_version,model,reevaluation,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, value.ID, value.DocumentID, value.PreviousEvaluationID, value.CurrentEvaluationID, value.PreviousVerdict, value.CurrentVerdict, value.CriteriaVersion, value.Model, payload, value.CreatedAt)
	if err == nil && value.ObjectionID != "" {
		_, _ = s.pool.Exec(ctx, `UPDATE objections SET status='reevaluated', objection=jsonb_set(jsonb_set(objection,'{Status}','"reevaluated"'),'{ResultingReevaluationID}',to_jsonb($1::text)) WHERE id=$2`, value.ID, value.ObjectionID)
	}
	return value, err
}
func (s *PostgreSQLStore) SaveObjection(ctx context.Context, value Objection) (Objection, error) {
	value.CreatedAt = time.Now().UTC()
	payload, err := json.Marshal(value)
	if err != nil {
		return Objection{}, err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO objections(id,evaluation_id,finding_id,status,objection,created_at) VALUES($1,$2,$3,$4,$5,$6)`, value.ID, value.EvaluationID, value.FindingID, value.Status, payload, value.CreatedAt)
	return value, err
}
func (s *PostgreSQLStore) ListVerdictChanges(ctx context.Context, documentID string) ([]Reevaluation, error) {
	if strings.TrimSpace(documentID) == "" {
		return nil, ErrInvalid
	}
	rows, err := s.pool.Query(ctx, `SELECT reevaluation FROM reevaluations WHERE document_id=$1 AND previous_verdict<>current_verdict ORDER BY created_at`, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []Reevaluation
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var value Reevaluation
		if err := json.Unmarshal(payload, &value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

type MemoryStore struct {
	Comparisons   []Comparison
	Reevaluations []Reevaluation
	Objections    []Objection
}

func (s *MemoryStore) SaveComparison(_ context.Context, v Comparison) (Comparison, error) {
	s.Comparisons = append(s.Comparisons, v)
	return v, nil
}
func (s *MemoryStore) SaveReevaluation(_ context.Context, v Reevaluation) (Reevaluation, error) {
	s.Reevaluations = append(s.Reevaluations, v)
	return v, nil
}
func (s *MemoryStore) SaveObjection(_ context.Context, v Objection) (Objection, error) {
	s.Objections = append(s.Objections, v)
	return v, nil
}
func (s *MemoryStore) ListVerdictChanges(_ context.Context, documentID string) ([]Reevaluation, error) {
	var out []Reevaluation
	for _, v := range s.Reevaluations {
		if v.DocumentID == documentID && v.VerdictChanged {
			out = append(out, v)
		}
	}
	return out, nil
}
