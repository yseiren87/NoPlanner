package document

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	Create(context.Context, string, VersionInput) (Version, error)
	AddVersion(context.Context, string, VersionInput) (Version, error)
	GetVersion(context.Context, string, uint32) (Version, error)
	ListVersions(context.Context, string) ([]Version, error)
	ReplaceElements(context.Context, string, uint32, string, []Element) error
	ReplaceRequirements(context.Context, string, uint32, string, []Requirement, []Dependency) error
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
		CREATE TABLE IF NOT EXISTS documents (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS documents_project_idx ON documents(project_id);
		CREATE TABLE IF NOT EXISTS document_versions (
			document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE RESTRICT,
			version INTEGER NOT NULL CHECK (version > 0),
			file_name TEXT NOT NULL,
			media_type TEXT NOT NULL,
			size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
			sha256 TEXT NOT NULL,
			object_key TEXT NOT NULL,
			extracted_text TEXT NOT NULL DEFAULT '',
			blocks JSONB NOT NULL DEFAULT '[]'::jsonb,
			parse_issues JSONB NOT NULL DEFAULT '[]'::jsonb,
			source_url TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (document_id, version)
		);
		CREATE TABLE IF NOT EXISTS document_elements (
			document_id TEXT NOT NULL,
			version INTEGER NOT NULL,
			id TEXT NOT NULL,
			type TEXT NOT NULL,
			content TEXT NOT NULL,
			confidence DOUBLE PRECISION NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
			source_block_ordinal INTEGER NOT NULL,
			source_location JSONB NOT NULL,
			model TEXT NOT NULL,
			PRIMARY KEY (document_id, version, id),
			FOREIGN KEY (document_id, version) REFERENCES document_versions(document_id, version) ON DELETE CASCADE
		);
		CREATE TABLE IF NOT EXISTS document_requirements (document_id TEXT NOT NULL, version INTEGER NOT NULL, id TEXT NOT NULL, type TEXT NOT NULL, content TEXT NOT NULL, confidence DOUBLE PRECISION NOT NULL CHECK(confidence>=0 AND confidence<=1), source_block_ordinal INTEGER NOT NULL, source_location JSONB NOT NULL, model TEXT NOT NULL, PRIMARY KEY(document_id,version,id), FOREIGN KEY(document_id,version) REFERENCES document_versions(document_id,version) ON DELETE CASCADE);
		CREATE TABLE IF NOT EXISTS document_dependencies (document_id TEXT NOT NULL, version INTEGER NOT NULL, id TEXT NOT NULL, type TEXT NOT NULL, content TEXT NOT NULL, confidence DOUBLE PRECISION NOT NULL CHECK(confidence>=0 AND confidence<=1), source_block_ordinal INTEGER NOT NULL, source_location JSONB NOT NULL, requirement_ids JSONB NOT NULL, model TEXT NOT NULL, PRIMARY KEY(document_id,version,id), FOREIGN KEY(document_id,version) REFERENCES document_versions(document_id,version) ON DELETE CASCADE);
	`)
	if err == nil {
		_, err = s.pool.Exec(ctx, `ALTER TABLE document_versions ADD COLUMN IF NOT EXISTS extracted_text TEXT NOT NULL DEFAULT ''; ALTER TABLE document_versions ADD COLUMN IF NOT EXISTS blocks JSONB NOT NULL DEFAULT '[]'::jsonb; ALTER TABLE document_versions ADD COLUMN IF NOT EXISTS parse_issues JSONB NOT NULL DEFAULT '[]'::jsonb; ALTER TABLE document_versions ADD COLUMN IF NOT EXISTS source_url TEXT NOT NULL DEFAULT ''`)
	}
	return err
}

func (s *PostgreSQLStore) ReplaceRequirements(ctx context.Context, documentID string, version uint32, model string, requirements []Requirement, dependencies []Dependency) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM document_dependencies WHERE document_id=$1 AND version=$2`, documentID, version); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM document_requirements WHERE document_id=$1 AND version=$2`, documentID, version); err != nil {
		return err
	}
	for _, value := range requirements {
		location, _ := json.Marshal(value.SourceLocation)
		if _, err := tx.Exec(ctx, `INSERT INTO document_requirements VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, documentID, version, value.ID, value.Type, value.Content, value.Confidence, value.SourceBlockOrdinal, location, model); err != nil {
			return err
		}
	}
	for _, value := range dependencies {
		location, _ := json.Marshal(value.SourceLocation)
		links, _ := json.Marshal(value.RequirementIDs)
		if _, err := tx.Exec(ctx, `INSERT INTO document_dependencies VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, documentID, version, value.ID, value.Type, value.Content, value.Confidence, value.SourceBlockOrdinal, location, links, model); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *PostgreSQLStore) ReplaceElements(ctx context.Context, documentID string, version uint32, model string, elements []Element) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM document_elements WHERE document_id=$1 AND version=$2`, documentID, version); err != nil {
		return err
	}
	for _, element := range elements {
		location, err := json.Marshal(element.SourceLocation)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO document_elements(document_id,version,id,type,content,confidence,source_block_ordinal,source_location,model) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, documentID, version, element.ID, element.Type, element.Content, element.Confidence, element.SourceBlockOrdinal, location, model); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *PostgreSQLStore) Create(ctx context.Context, projectID string, input VersionInput) (Version, error) {
	if !validInput(projectID, input) {
		return Version{}, ErrInvalid
	}
	documentID, err := newID()
	if err != nil {
		return Version{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Version{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO documents (id, project_id) VALUES ($1, $2)`, documentID, projectID); err != nil {
		return Version{}, err
	}
	value, err := insertVersion(ctx, tx, documentID, projectID, 1, input)
	if err != nil {
		return Version{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Version{}, err
	}
	return value, nil
}

func (s *PostgreSQLStore) AddVersion(ctx context.Context, documentID string, input VersionInput) (Version, error) {
	if !validInput(documentID, input) {
		return Version{}, ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Version{}, err
	}
	defer tx.Rollback(ctx)
	var projectID string
	if err := tx.QueryRow(ctx, `SELECT project_id FROM documents WHERE id = $1 FOR UPDATE`, documentID).Scan(&projectID); errors.Is(err, pgx.ErrNoRows) {
		return Version{}, ErrNotFound
	} else if err != nil {
		return Version{}, err
	}
	var number uint32
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(version), 0) + 1 FROM document_versions WHERE document_id = $1`, documentID).Scan(&number); err != nil {
		return Version{}, err
	}
	value, err := insertVersion(ctx, tx, documentID, projectID, number, input)
	if err != nil {
		return Version{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Version{}, err
	}
	return value, nil
}

func (s *PostgreSQLStore) GetVersion(ctx context.Context, documentID string, number uint32) (Version, error) {
	var value Version
	err := s.pool.QueryRow(ctx, `
		SELECT v.document_id, d.project_id, v.version, v.file_name, v.media_type, v.size_bytes, v.sha256, v.object_key, v.created_at, v.extracted_text, v.blocks, v.parse_issues, v.source_url
		FROM document_versions v JOIN documents d ON d.id = v.document_id
		WHERE v.document_id = $1 AND v.version = $2`, documentID, number,
	).Scan(&value.DocumentID, &value.ProjectID, &value.Number, &value.FileName, &value.MediaType, &value.SizeBytes, &value.SHA256, &value.ObjectKey, &value.CreatedAt, &value.Extraction.Text, &value.Extraction.Blocks, &value.Extraction.Issues, &value.SourceURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return Version{}, ErrNotFound
	}
	return value, err
}

func (s *PostgreSQLStore) ListVersions(ctx context.Context, documentID string) ([]Version, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT v.document_id, d.project_id, v.version, v.file_name, v.media_type, v.size_bytes, v.sha256, v.object_key, v.created_at, v.extracted_text, v.blocks, v.parse_issues, v.source_url
		FROM document_versions v JOIN documents d ON d.id = v.document_id
		WHERE v.document_id = $1 ORDER BY v.version`, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]Version, 0)
	for rows.Next() {
		var value Version
		if err := rows.Scan(&value.DocumentID, &value.ProjectID, &value.Number, &value.FileName, &value.MediaType, &value.SizeBytes, &value.SHA256, &value.ObjectKey, &value.CreatedAt, &value.Extraction.Text, &value.Extraction.Blocks, &value.Extraction.Issues, &value.SourceURL); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, ErrNotFound
	}
	return values, nil
}

func insertVersion(ctx context.Context, tx pgx.Tx, documentID, projectID string, number uint32, input VersionInput) (Version, error) {
	var value Version
	blocks, err := json.Marshal(input.Extraction.Blocks)
	if err != nil {
		return Version{}, err
	}
	issues, err := json.Marshal(input.Extraction.Issues)
	if err != nil {
		return Version{}, err
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO document_versions (document_id, version, file_name, media_type, size_bytes, sha256, object_key, extracted_text, blocks, parse_issues, source_url)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING document_id, $12::text, version, file_name, media_type, size_bytes, sha256, object_key, created_at, extracted_text, blocks, parse_issues, source_url`,
		documentID, number, strings.TrimSpace(input.FileName), strings.TrimSpace(input.MediaType), input.SizeBytes, input.SHA256, input.ObjectKey,
		input.Extraction.Text, blocks, issues, input.SourceURL, projectID,
	).Scan(&value.DocumentID, &value.ProjectID, &value.Number, &value.FileName, &value.MediaType, &value.SizeBytes, &value.SHA256, &value.ObjectKey, &value.CreatedAt, &value.Extraction.Text, &value.Extraction.Blocks, &value.Extraction.Issues, &value.SourceURL)
	return value, err
}

func validInput(scope string, input VersionInput) bool {
	return strings.TrimSpace(scope) != "" && strings.TrimSpace(input.FileName) != "" && strings.TrimSpace(input.MediaType) != "" && input.SHA256 != "" && input.ObjectKey != "" && input.SizeBytes > 0 && (strings.TrimSpace(input.Extraction.Text) != "" || len(input.Extraction.Issues) > 0)
}

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}
