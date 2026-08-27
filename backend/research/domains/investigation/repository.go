package investigation

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgreSQLStore struct{ pool *pgxpool.Pool }

func NewPostgreSQLStore(ctx context.Context, databaseURL string) (*PostgreSQLStore, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	store := &PostgreSQLStore{pool: pool}
	_, err = pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS research_plans (
		id TEXT PRIMARY KEY, evaluation_id TEXT NOT NULL, target_country TEXT NOT NULL, target_domain TEXT NOT NULL,
		tasks JSONB NOT NULL, limits JSONB NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
	CREATE INDEX IF NOT EXISTS research_plan_evaluation_idx ON research_plans(evaluation_id, created_at);
	CREATE TABLE IF NOT EXISTS research_reports (
		id TEXT PRIMARY KEY, plan_id TEXT NOT NULL REFERENCES research_plans(id), evaluation_id TEXT NOT NULL,
		status TEXT NOT NULL, report JSONB NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
	CREATE INDEX IF NOT EXISTS research_report_evaluation_idx ON research_reports(evaluation_id, created_at);`)
	if err != nil {
		pool.Close()
		return nil, err
	}
	return store, nil
}

func (s *PostgreSQLStore) Close() { s.pool.Close() }

func (s *PostgreSQLStore) SavePlan(ctx context.Context, plan Plan) (Plan, error) {
	if plan.ID == "" || plan.EvaluationID == "" || len(plan.Tasks) == 0 {
		return Plan{}, ErrInvalid
	}
	tasks, err := json.Marshal(plan.Tasks)
	if err != nil {
		return Plan{}, err
	}
	limits, err := json.Marshal(plan.Limit)
	if err != nil {
		return Plan{}, err
	}
	err = s.pool.QueryRow(ctx, `INSERT INTO research_plans(id,evaluation_id,target_country,target_domain,tasks,limits) VALUES($1,$2,$3,$4,$5,$6) RETURNING created_at`, plan.ID, plan.EvaluationID, plan.TargetCountry, plan.TargetDomain, tasks, limits).Scan(&plan.CreatedAt)
	return plan, err
}

func (s *PostgreSQLStore) SaveReport(ctx context.Context, report Report) (Report, error) {
	if report.ID == "" || report.PlanID == "" || report.Status == "" {
		return Report{}, ErrInvalid
	}
	payload, err := json.Marshal(report)
	if err != nil {
		return Report{}, err
	}
	err = s.pool.QueryRow(ctx, `INSERT INTO research_reports(id,plan_id,evaluation_id,status,report) VALUES($1,$2,$3,$4,$5) RETURNING created_at`, report.ID, report.PlanID, report.EvaluationID, report.Status, payload).Scan(&report.CreatedAt)
	return report, err
}

func (s *PostgreSQLStore) GetReport(ctx context.Context, id string) (Report, error) {
	var payload []byte
	if err := s.pool.QueryRow(ctx, `SELECT report FROM research_reports WHERE id=$1`, id).Scan(&payload); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Report{}, ErrNotFound
		}
		return Report{}, err
	}
	var report Report
	if err := json.Unmarshal(payload, &report); err != nil {
		return Report{}, err
	}
	return report, nil
}

type MemoryStore struct {
	plans   map[string]Plan
	reports map[string]Report
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{plans: map[string]Plan{}, reports: map[string]Report{}}
}
func (s *MemoryStore) SavePlan(_ context.Context, p Plan) (Plan, error) {
	p.CreatedAt = time.Now()
	s.plans[p.ID] = p
	return p, nil
}
func (s *MemoryStore) SaveReport(_ context.Context, r Report) (Report, error) {
	r.CreatedAt = time.Now()
	s.reports[r.ID] = r
	return r, nil
}
func (s *MemoryStore) GetReport(_ context.Context, id string) (Report, error) {
	r, ok := s.reports[id]
	if !ok {
		return Report{}, ErrNotFound
	}
	return r, nil
}
