package project

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	Create(context.Context, string, string) (Project, error)
	List(context.Context, string) ([]Project, error)
	Get(context.Context, string, string) (Project, error)
	Update(context.Context, string, string, string) (Project, error)
	SetMemberRole(context.Context, string, string, string, Role) (Member, error)
	UpdateOutputLanguage(context.Context, string, string, string) (Project, error)
}

func (s *PostgreSQLStore) List(ctx context.Context, subject string) ([]Project, error) {
	if strings.TrimSpace(subject) == "" {
		return nil, ErrInvalid
	}
	rows, err := s.pool.Query(ctx, `SELECT p.id,p.name,p.created_at,p.updated_at,m.role,p.output_language FROM projects p JOIN project_members m ON m.project_id=p.id WHERE m.subject=$1 ORDER BY p.updated_at DESC,p.id`, subject)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []Project
	for rows.Next() {
		var value Project
		if err := rows.Scan(&value.ID, &value.Name, &value.CreatedAt, &value.UpdatedAt, &value.Role, &value.OutputLanguage); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

type PostgreSQLStore struct {
	pool *pgxpool.Pool
}

func NewPostgreSQLStore(ctx context.Context, databaseURL string) (*PostgreSQLStore, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	store := &PostgreSQLStore{pool: pool}
	if err := store.prepare(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return store, nil
}

func (s *PostgreSQLStore) Close() {
	s.pool.Close()
}

func (s *PostgreSQLStore) prepare(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS projects (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
			, output_language TEXT NOT NULL DEFAULT 'ko' CHECK(output_language IN ('ko','en'))
		);
		CREATE TABLE IF NOT EXISTS project_members (
			project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			subject TEXT NOT NULL,
			role TEXT NOT NULL CHECK (role IN ('owner', 'editor', 'viewer')),
			PRIMARY KEY (project_id, subject)
		);
		CREATE INDEX IF NOT EXISTS project_members_subject_idx ON project_members(subject);
	`)
	if err == nil {
		_, err = s.pool.Exec(ctx, `ALTER TABLE projects ADD COLUMN IF NOT EXISTS output_language TEXT NOT NULL DEFAULT 'ko' CHECK(output_language IN ('ko','en'))`)
	}
	return err
}

func (s *PostgreSQLStore) Create(ctx context.Context, subject, name string) (Project, error) {
	name = strings.TrimSpace(name)
	if subject == "" || name == "" {
		return Project{}, ErrInvalid
	}
	id, err := newID()
	if err != nil {
		return Project{}, err
	}
	now := time.Now().UTC()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Project{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO projects (id, name, created_at, updated_at) VALUES ($1, $2, $3, $3)`, id, name, now); err != nil {
		return Project{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO project_members (project_id, subject, role) VALUES ($1, $2, $3)`, id, subject, RoleOwner); err != nil {
		return Project{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Project{}, err
	}
	return Project{ID: id, Name: name, CreatedAt: now, UpdatedAt: now, Role: RoleOwner, OutputLanguage: "ko"}, nil
}

func (s *PostgreSQLStore) Get(ctx context.Context, subject, projectID string) (Project, error) {
	var value Project
	err := s.pool.QueryRow(ctx, `
		SELECT p.id, p.name, p.created_at, p.updated_at, m.role, p.output_language
		FROM projects p JOIN project_members m ON m.project_id = p.id
		WHERE p.id = $1 AND m.subject = $2`, projectID, subject,
	).Scan(&value.ID, &value.Name, &value.CreatedAt, &value.UpdatedAt, &value.Role, &value.OutputLanguage)
	if errors.Is(err, pgx.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	return value, err
}

func (s *PostgreSQLStore) Update(ctx context.Context, subject, projectID, name string) (Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Project{}, ErrInvalid
	}
	var role Role
	if err := s.pool.QueryRow(ctx, `SELECT role FROM project_members WHERE project_id = $1 AND subject = $2`, projectID, subject).Scan(&role); errors.Is(err, pgx.ErrNoRows) {
		return Project{}, ErrForbidden
	} else if err != nil {
		return Project{}, err
	}
	if !role.CanEdit() {
		return Project{}, ErrForbidden
	}
	var value Project
	err := s.pool.QueryRow(ctx, `
		UPDATE projects SET name = $1, updated_at = NOW() WHERE id = $2
		RETURNING id, name, created_at, updated_at, output_language`, name, projectID,
	).Scan(&value.ID, &value.Name, &value.CreatedAt, &value.UpdatedAt, &value.OutputLanguage)
	if errors.Is(err, pgx.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	value.Role = role
	return value, err
}

func (s *PostgreSQLStore) UpdateOutputLanguage(ctx context.Context, subject, projectID, language string) (Project, error) {
	if language != "ko" && language != "en" {
		return Project{}, ErrInvalid
	}
	var role Role
	if err := s.pool.QueryRow(ctx, `SELECT role FROM project_members WHERE project_id=$1 AND subject=$2`, projectID, subject).Scan(&role); errors.Is(err, pgx.ErrNoRows) {
		return Project{}, ErrForbidden
	} else if err != nil {
		return Project{}, err
	}
	if !role.CanEdit() {
		return Project{}, ErrForbidden
	}
	var value Project
	err := s.pool.QueryRow(ctx, `UPDATE projects SET output_language=$1,updated_at=NOW() WHERE id=$2 RETURNING id,name,created_at,updated_at,output_language`, language, projectID).Scan(&value.ID, &value.Name, &value.CreatedAt, &value.UpdatedAt, &value.OutputLanguage)
	value.Role = role
	return value, err
}

func (s *PostgreSQLStore) SetMemberRole(ctx context.Context, actor, projectID, subject string, role Role) (Member, error) {
	if actor == "" || subject == "" || !role.Valid() {
		return Member{}, ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Member{}, err
	}
	defer tx.Rollback(ctx)
	var actorRole Role
	if err := tx.QueryRow(ctx, `SELECT role FROM project_members WHERE project_id = $1 AND subject = $2 FOR UPDATE`, projectID, actor).Scan(&actorRole); errors.Is(err, pgx.ErrNoRows) {
		return Member{}, ErrForbidden
	} else if err != nil {
		return Member{}, err
	}
	if !actorRole.CanManageMembers() {
		return Member{}, ErrForbidden
	}
	if actor == subject && role != RoleOwner {
		var owners int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM project_members WHERE project_id = $1 AND role = 'owner'`, projectID).Scan(&owners); err != nil {
			return Member{}, err
		}
		if owners == 1 {
			return Member{}, ErrLastOwner
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO project_members (project_id, subject, role) VALUES ($1, $2, $3)
		ON CONFLICT (project_id, subject) DO UPDATE SET role = EXCLUDED.role`, projectID, subject, role,
	); err != nil {
		return Member{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Member{}, err
	}
	return Member{ProjectID: projectID, Subject: subject, Role: role}, nil
}

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}
