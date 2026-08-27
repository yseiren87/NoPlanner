package assessment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInvalid = errors.New("invalid assessment")

type Store interface {
	Create(context.Context, Analysis) (Analysis, error)
	CreateValidationPlan(context.Context, ValidationPlan) (ValidationPlan, error)
	CreateValidationResultSummary(context.Context, ValidationResultSummary) (ValidationResultSummary, error)
	CreatePurposeAlignmentEvaluation(context.Context, PurposeAlignmentEvaluation) (PurposeAlignmentEvaluation, error)
	CreateContradictionEvaluation(context.Context, ContradictionEvaluation) (ContradictionEvaluation, error)
	CreateRequirementCompletenessEvaluation(context.Context, RequirementCompletenessEvaluation) (RequirementCompletenessEvaluation, error)
	CreateDocumentWorkQualityEvaluation(context.Context, DocumentWorkQualityEvaluation) (DocumentWorkQualityEvaluation, error)
}

type PostgreSQLStore struct{ pool *pgxpool.Pool }

func NewPostgreSQLStore(ctx context.Context, databaseURL string) (*PostgreSQLStore, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	store := &PostgreSQLStore{pool: pool}
	_, err = pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS assumption_risk_analyses (
		id TEXT PRIMARY KEY, document_id TEXT NOT NULL, document_version INTEGER NOT NULL CHECK(document_version > 0),
		assumptions JSONB NOT NULL, risks JSONB NOT NULL, model TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	); CREATE INDEX IF NOT EXISTS assumption_risk_document_idx ON assumption_risk_analyses(document_id, document_version, created_at);
	CREATE TABLE IF NOT EXISTS validation_plans (
		id TEXT PRIMARY KEY, document_id TEXT NOT NULL, document_version INTEGER NOT NULL CHECK(document_version > 0),
		purpose TEXT NOT NULL, country TEXT NOT NULL, domain TEXT NOT NULL, items JSONB NOT NULL, model TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	); CREATE INDEX IF NOT EXISTS validation_plan_document_idx ON validation_plans(document_id, document_version, created_at);
	CREATE TABLE IF NOT EXISTS validation_result_summaries (
		validation_plan_id TEXT PRIMARY KEY REFERENCES validation_plans(id) ON DELETE CASCADE,
		results JSONB NOT NULL, defect_count INTEGER NOT NULL, scored_count INTEGER NOT NULL, excluded_count INTEGER NOT NULL,
		satisfaction_rate DOUBLE PRECISION NOT NULL CHECK(satisfaction_rate >= 0 AND satisfaction_rate <= 1), created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE TABLE IF NOT EXISTS purpose_alignment_evaluations (
		id TEXT PRIMARY KEY, document_id TEXT NOT NULL, document_version INTEGER NOT NULL CHECK(document_version > 0),
		status TEXT NOT NULL, findings JSONB NOT NULL, model TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	); CREATE INDEX IF NOT EXISTS purpose_alignment_document_idx ON purpose_alignment_evaluations(document_id,document_version,created_at);
	CREATE TABLE IF NOT EXISTS contradiction_evaluations (
		id TEXT PRIMARY KEY, document_id TEXT NOT NULL, document_version INTEGER NOT NULL CHECK(document_version > 0), status TEXT NOT NULL,
		contradictions JSONB NOT NULL, model TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	); CREATE INDEX IF NOT EXISTS contradiction_document_idx ON contradiction_evaluations(document_id,document_version,created_at);
	CREATE TABLE IF NOT EXISTS requirement_completeness_evaluations (
		id TEXT PRIMARY KEY, document_id TEXT NOT NULL, document_version INTEGER NOT NULL CHECK(document_version > 0), status TEXT NOT NULL,
		gaps JSONB NOT NULL, model TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	); CREATE INDEX IF NOT EXISTS requirement_completeness_document_idx ON requirement_completeness_evaluations(document_id,document_version,created_at);
	CREATE TABLE IF NOT EXISTS document_work_quality_evaluations (
		id TEXT PRIMARY KEY, document_id TEXT NOT NULL, document_version INTEGER NOT NULL CHECK(document_version > 0),
		results JSONB NOT NULL, model TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	); CREATE INDEX IF NOT EXISTS work_quality_document_idx ON document_work_quality_evaluations(document_id,document_version,created_at);`)
	if err != nil {
		pool.Close()
		return nil, err
	}
	return store, nil
}

func (s *PostgreSQLStore) CreateDocumentWorkQualityEvaluation(ctx context.Context, input DocumentWorkQualityEvaluation) (DocumentWorkQualityEvaluation, error) {
	if strings.TrimSpace(input.DocumentID) == "" || input.DocumentVersion == 0 || input.Model == "" || len(input.Results) != 3 {
		return DocumentWorkQualityEvaluation{}, ErrInvalid
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return DocumentWorkQualityEvaluation{}, err
	}
	input.ID = hex.EncodeToString(raw[:])
	values, err := json.Marshal(input.Results)
	if err != nil {
		return DocumentWorkQualityEvaluation{}, err
	}
	err = s.pool.QueryRow(ctx, `INSERT INTO document_work_quality_evaluations(id,document_id,document_version,results,model) VALUES($1,$2,$3,$4,$5) RETURNING created_at`, input.ID, input.DocumentID, input.DocumentVersion, values, input.Model).Scan(&input.CreatedAt)
	return input, err
}

func (s *PostgreSQLStore) CreateRequirementCompletenessEvaluation(ctx context.Context, input RequirementCompletenessEvaluation) (RequirementCompletenessEvaluation, error) {
	if strings.TrimSpace(input.DocumentID) == "" || input.DocumentVersion == 0 || input.Status == "" || input.Model == "" {
		return RequirementCompletenessEvaluation{}, ErrInvalid
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return RequirementCompletenessEvaluation{}, err
	}
	input.ID = hex.EncodeToString(raw[:])
	values, err := json.Marshal(input.Gaps)
	if err != nil {
		return RequirementCompletenessEvaluation{}, err
	}
	err = s.pool.QueryRow(ctx, `INSERT INTO requirement_completeness_evaluations(id,document_id,document_version,status,gaps,model) VALUES($1,$2,$3,$4,$5,$6) RETURNING created_at`, input.ID, input.DocumentID, input.DocumentVersion, input.Status, values, input.Model).Scan(&input.CreatedAt)
	return input, err
}

func (s *PostgreSQLStore) CreateContradictionEvaluation(ctx context.Context, input ContradictionEvaluation) (ContradictionEvaluation, error) {
	if strings.TrimSpace(input.DocumentID) == "" || input.DocumentVersion == 0 || input.Status == "" || input.Model == "" {
		return ContradictionEvaluation{}, ErrInvalid
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return ContradictionEvaluation{}, err
	}
	input.ID = hex.EncodeToString(raw[:])
	values, err := json.Marshal(input.Contradictions)
	if err != nil {
		return ContradictionEvaluation{}, err
	}
	err = s.pool.QueryRow(ctx, `INSERT INTO contradiction_evaluations(id,document_id,document_version,status,contradictions,model) VALUES($1,$2,$3,$4,$5,$6) RETURNING created_at`, input.ID, input.DocumentID, input.DocumentVersion, input.Status, values, input.Model).Scan(&input.CreatedAt)
	return input, err
}

func (s *PostgreSQLStore) CreatePurposeAlignmentEvaluation(ctx context.Context, input PurposeAlignmentEvaluation) (PurposeAlignmentEvaluation, error) {
	if strings.TrimSpace(input.DocumentID) == "" || input.DocumentVersion == 0 || strings.TrimSpace(input.Status) == "" || strings.TrimSpace(input.Model) == "" {
		return PurposeAlignmentEvaluation{}, ErrInvalid
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return PurposeAlignmentEvaluation{}, err
	}
	input.ID = hex.EncodeToString(raw[:])
	findings, err := json.Marshal(input.Findings)
	if err != nil {
		return PurposeAlignmentEvaluation{}, err
	}
	err = s.pool.QueryRow(ctx, `INSERT INTO purpose_alignment_evaluations(id,document_id,document_version,status,findings,model) VALUES($1,$2,$3,$4,$5,$6) RETURNING created_at`, input.ID, input.DocumentID, input.DocumentVersion, input.Status, findings, input.Model).Scan(&input.CreatedAt)
	return input, err
}

func (s *PostgreSQLStore) CreateValidationResultSummary(ctx context.Context, input ValidationResultSummary) (ValidationResultSummary, error) {
	if strings.TrimSpace(input.ValidationPlanID) == "" || len(input.Results) == 0 {
		return ValidationResultSummary{}, ErrInvalid
	}
	results, err := json.Marshal(input.Results)
	if err != nil {
		return ValidationResultSummary{}, err
	}
	err = s.pool.QueryRow(ctx, `INSERT INTO validation_result_summaries(validation_plan_id,results,defect_count,scored_count,excluded_count,satisfaction_rate) VALUES($1,$2,$3,$4,$5,$6) RETURNING created_at`, input.ValidationPlanID, results, input.DefectCount, input.ScoredCount, input.ExcludedCount, input.SatisfactionRate).Scan(&input.CreatedAt)
	return input, err
}

func (s *PostgreSQLStore) CreateValidationPlan(ctx context.Context, input ValidationPlan) (ValidationPlan, error) {
	if strings.TrimSpace(input.DocumentID) == "" || input.DocumentVersion == 0 || strings.TrimSpace(input.Purpose) == "" || strings.TrimSpace(input.Model) == "" || len(input.Items) == 0 {
		return ValidationPlan{}, ErrInvalid
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return ValidationPlan{}, err
	}
	input.ID = hex.EncodeToString(raw[:])
	items, err := json.Marshal(input.Items)
	if err != nil {
		return ValidationPlan{}, err
	}
	err = s.pool.QueryRow(ctx, `INSERT INTO validation_plans(id,document_id,document_version,purpose,country,domain,items,model) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING created_at`, input.ID, input.DocumentID, input.DocumentVersion, input.Purpose, input.Country, input.Domain, items, input.Model).Scan(&input.CreatedAt)
	return input, err
}

func (s *PostgreSQLStore) Close() { s.pool.Close() }

func (s *PostgreSQLStore) Create(ctx context.Context, input Analysis) (Analysis, error) {
	if strings.TrimSpace(input.DocumentID) == "" || input.DocumentVersion == 0 || strings.TrimSpace(input.Model) == "" {
		return Analysis{}, ErrInvalid
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return Analysis{}, err
	}
	input.ID = hex.EncodeToString(raw[:])
	assumptions, err := json.Marshal(input.Assumptions)
	if err != nil {
		return Analysis{}, err
	}
	risks, err := json.Marshal(input.Risks)
	if err != nil {
		return Analysis{}, err
	}
	err = s.pool.QueryRow(ctx, `INSERT INTO assumption_risk_analyses(id,document_id,document_version,assumptions,risks,model) VALUES($1,$2,$3,$4,$5,$6) RETURNING created_at`, input.ID, input.DocumentID, input.DocumentVersion, assumptions, risks, input.Model).Scan(&input.CreatedAt)
	return input, err
}
