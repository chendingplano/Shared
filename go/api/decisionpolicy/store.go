package decisionpolicy

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

const (
	defaultSchema    = "shared"
	defaultListLimit = 100
	maxListLimit     = 1000
	maxNameLen       = 128
)

var (
	namePattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
	schemaPattern = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)
)

// Store manages policies in Postgres. It is safe for concurrent use.
type Store struct {
	db       *sql.DB
	schema   string
	logger   ApiTypes.JimoLogger
	policies string // qualified table names
	versions string
}

// Option configures a Store.
type Option func(*Store)

// WithSchema sets the Postgres schema holding the tables (default "shared").
// It is meant for tests that use a throwaway schema. NewStore panics on an
// invalid schema name, since the name is interpolated into SQL.
func WithSchema(schema string) Option {
	return func(s *Store) { s.schema = schema }
}

// NewStore returns a Store over db, normally ApiTypes.SharedDBHandle. A nil
// logger is replaced by a default one.
func NewStore(db *sql.DB, logger ApiTypes.JimoLogger, opts ...Option) *Store {
	s := &Store{db: db, schema: defaultSchema, logger: logger}
	for _, opt := range opts {
		opt(s)
	}
	if !schemaPattern.MatchString(s.schema) {
		panic(fmt.Sprintf("decisionpolicy: invalid schema name %q", s.schema))
	}
	if s.logger == nil {
		s.logger = loggerutil.CreateDefaultLogger("20261005-482")
	}
	s.policies = s.schema + ".decision_policies"
	s.versions = s.schema + ".decision_policy_versions"
	return s
}

const policyColumns = "id, name, description, current_version, latest_version, status, created_by, updated_by, created_at, updated_at, deleted_at"

// ---- mutations ----

// Create creates a policy with version 1 as its current version.
func (s *Store) Create(ctx context.Context, in CreateInput) (*Policy, error) {
	const op = "create"
	if err := validateName(in.Name); err != nil {
		return nil, s.fail(op, err, "name", in.Name, "actor", in.Actor)
	}
	if err := validateContent(in.Content); err != nil {
		return nil, s.fail(op, err, "name", in.Name, "actor", in.Actor)
	}
	var p *Policy
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		var err error
		p, err = scanPolicy(tx.QueryRowContext(ctx,
			`INSERT INTO `+s.policies+` (name, description, current_version, latest_version, created_by, updated_by)
			 VALUES ($1, $2, 1, 1, $3, $3) RETURNING `+policyColumns,
			in.Name, in.Description, in.Actor))
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO `+s.versions+` (policy_id, version, content, note, created_by) VALUES ($1, 1, $2, $3, $4)`,
			p.ID, in.Content, in.Note, in.Actor)
		return err
	})
	if err != nil {
		return nil, s.fail(op, err, "name", in.Name, "actor", in.Actor)
	}
	s.logger.Info("decisionpolicy: created", "policy_id", p.ID, "name", p.Name, "version", 1, "actor", in.Actor)
	return p, nil
}

// Update edits a policy's name and/or description. It never creates a version.
func (s *Store) Update(ctx context.Context, id int64, in UpdateInput) (*Policy, error) {
	const op = "update"
	if in.Name == nil && in.Description == nil {
		return nil, s.fail(op, fmt.Errorf("%w: nothing to update", ErrInvalidInput), "policy_id", id, "actor", in.Actor)
	}
	if in.Name != nil {
		if err := validateName(*in.Name); err != nil {
			return nil, s.fail(op, err, "policy_id", id, "actor", in.Actor)
		}
	}
	var p *Policy
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		cur, err := s.lockActive(ctx, tx, id)
		if err != nil {
			return err
		}
		name, desc := cur.Name, cur.Description
		if in.Name != nil {
			name = *in.Name
		}
		if in.Description != nil {
			desc = *in.Description
		}
		p, err = scanPolicy(tx.QueryRowContext(ctx,
			`UPDATE `+s.policies+` SET name = $2, description = $3, updated_by = $4, updated_at = NOW()
			 WHERE id = $1 RETURNING `+policyColumns,
			id, name, desc, in.Actor))
		return err
	})
	if err != nil {
		return nil, s.fail(op, err, "policy_id", id, "actor", in.Actor)
	}
	s.logger.Info("decisionpolicy: updated", "policy_id", id, "name", p.Name, "actor", in.Actor)
	return p, nil
}

// CreateVersion appends version latest_version+1. It becomes current unless
// in.MakeCurrent is false.
func (s *Store) CreateVersion(ctx context.Context, id int64, in VersionInput) (*PolicyVersion, error) {
	const op = "create_version"
	if err := validateContent(in.Content); err != nil {
		return nil, s.fail(op, err, "policy_id", id, "actor", in.Actor)
	}
	makeCurrent := in.MakeCurrent == nil || *in.MakeCurrent
	var v *PolicyVersion
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		cur, err := s.lockActive(ctx, tx, id)
		if err != nil {
			return err
		}
		next := cur.LatestVersion + 1
		v, err = scanVersion(tx.QueryRowContext(ctx,
			`INSERT INTO `+s.versions+` (policy_id, version, content, note, created_by) VALUES ($1, $2, $3, $4, $5)
			 RETURNING id, policy_id, $6::text, version, content, note, created_by, created_at`,
			id, next, in.Content, in.Note, in.Actor, cur.Name))
		if err != nil {
			return err
		}
		current := cur.CurrentVersion
		if makeCurrent {
			current = next
		}
		_, err = tx.ExecContext(ctx,
			`UPDATE `+s.policies+` SET latest_version = $2, current_version = $3, updated_by = $4, updated_at = NOW() WHERE id = $1`,
			id, next, current, in.Actor)
		return err
	})
	if err != nil {
		return nil, s.fail(op, err, "policy_id", id, "actor", in.Actor)
	}
	s.logger.Info("decisionpolicy: version created", "policy_id", id, "version", v.Version, "current", makeCurrent, "actor", in.Actor)
	return v, nil
}

// SetCurrentVersion makes an existing version current (e.g. a rollback).
func (s *Store) SetCurrentVersion(ctx context.Context, id int64, version int, actor string) (*Policy, error) {
	const op = "set_current_version"
	var p *Policy
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := s.lockActive(ctx, tx, id); err != nil {
			return err
		}
		var one int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM `+s.versions+` WHERE policy_id = $1 AND version = $2`, id, version).Scan(&one)
		if err != nil {
			return err // sql.ErrNoRows → ErrNotFound
		}
		p, err = scanPolicy(tx.QueryRowContext(ctx,
			`UPDATE `+s.policies+` SET current_version = $2, updated_by = $3, updated_at = NOW() WHERE id = $1 RETURNING `+policyColumns,
			id, version, actor))
		return err
	})
	if err != nil {
		return nil, s.fail(op, err, "policy_id", id, "version", version, "actor", actor)
	}
	s.logger.Info("decisionpolicy: current version set", "policy_id", id, "version", version, "actor", actor)
	return p, nil
}

// Delete soft-deletes a policy. Its versions stay readable by policy ID and
// its name becomes free for a new policy.
func (s *Store) Delete(ctx context.Context, id int64, actor string) error {
	const op = "delete"
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := s.lockActive(ctx, tx, id); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`UPDATE `+s.policies+` SET status = $2, deleted_at = NOW(), updated_by = $3, updated_at = NOW() WHERE id = $1`,
			id, StatusDeleted, actor)
		return err
	})
	if err != nil {
		return s.fail(op, err, "policy_id", id, "actor", actor)
	}
	s.logger.Info("decisionpolicy: deleted", "policy_id", id, "actor", actor)
	return nil
}

// ---- queries ----

// Get returns a policy by ID, whatever its status.
func (s *Store) Get(ctx context.Context, id int64) (*Policy, error) {
	p, err := scanPolicy(s.db.QueryRowContext(ctx, `SELECT `+policyColumns+` FROM `+s.policies+` WHERE id = $1`, id))
	if err != nil {
		return nil, s.fail("get", err, "policy_id", id)
	}
	s.logger.Debug("decisionpolicy: get", "policy_id", id)
	return p, nil
}

// GetByName returns the active policy with this name (case-insensitive).
func (s *Store) GetByName(ctx context.Context, name string) (*Policy, error) {
	p, err := scanPolicy(s.db.QueryRowContext(ctx,
		`SELECT `+policyColumns+` FROM `+s.policies+` WHERE LOWER(name) = LOWER($1) AND status = $2`, name, StatusActive))
	if err != nil {
		return nil, s.fail("get_by_name", err, "name", name)
	}
	s.logger.Debug("decisionpolicy: get by name", "policy_id", p.ID, "name", name)
	return p, nil
}

// List returns active policies ordered by name.
func (s *Store) List(ctx context.Context, f ListFilter) ([]Policy, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}
	if f.Offset < 0 {
		return nil, s.fail("list", fmt.Errorf("%w: negative offset", ErrInvalidInput))
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+policyColumns+` FROM `+s.policies+`
		 WHERE status = $1 AND LOWER(name) LIKE LOWER($2) ESCAPE '\'
		 ORDER BY LOWER(name) LIMIT $3 OFFSET $4`,
		StatusActive, escapeLike(f.NamePrefix)+"%", limit, f.Offset)
	if err != nil {
		return nil, s.fail("list", err, "prefix", f.NamePrefix)
	}
	defer rows.Close()
	out := []Policy{}
	for rows.Next() {
		p, err := scanPolicy(rows)
		if err != nil {
			return nil, s.fail("list", err, "prefix", f.NamePrefix)
		}
		out = append(out, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, s.fail("list", err, "prefix", f.NamePrefix)
	}
	s.logger.Debug("decisionpolicy: list", "prefix", f.NamePrefix, "count", len(out))
	return out, nil
}

const versionSelect = `SELECT v.id, v.policy_id, p.name, v.version, v.content, v.note, v.created_by, v.created_at FROM `

// GetVersion returns one version of a policy, whatever the policy's status.
func (s *Store) GetVersion(ctx context.Context, id int64, version int) (*PolicyVersion, error) {
	v, err := scanVersion(s.db.QueryRowContext(ctx,
		versionSelect+s.versions+` v JOIN `+s.policies+` p ON p.id = v.policy_id WHERE v.policy_id = $1 AND v.version = $2`,
		id, version))
	if err != nil {
		return nil, s.fail("get_version", err, "policy_id", id, "version", version)
	}
	s.logger.Debug("decisionpolicy: get version", "policy_id", id, "version", version)
	return v, nil
}

// GetCurrent returns the current version of the active policy with this name.
// Its Content is the text to place into a decision request's state; record
// PolicyID and Version in the request metadata for traceability.
func (s *Store) GetCurrent(ctx context.Context, name string) (*PolicyVersion, error) {
	v, err := scanVersion(s.db.QueryRowContext(ctx,
		versionSelect+s.versions+` v JOIN `+s.policies+` p ON p.id = v.policy_id AND v.version = p.current_version
		 WHERE LOWER(p.name) = LOWER($1) AND p.status = $2`,
		name, StatusActive))
	if err != nil {
		return nil, s.fail("get_current", err, "name", name)
	}
	s.logger.Debug("decisionpolicy: get current", "policy_id", v.PolicyID, "name", name, "version", v.Version)
	return v, nil
}

// ListVersions returns all versions of a policy, newest first.
func (s *Store) ListVersions(ctx context.Context, id int64) ([]PolicyVersion, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		versionSelect+s.versions+` v JOIN `+s.policies+` p ON p.id = v.policy_id WHERE v.policy_id = $1 ORDER BY v.version DESC`, id)
	if err != nil {
		return nil, s.fail("list_versions", err, "policy_id", id)
	}
	defer rows.Close()
	out := []PolicyVersion{}
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, s.fail("list_versions", err, "policy_id", id)
		}
		out = append(out, *v)
	}
	if err := rows.Err(); err != nil {
		return nil, s.fail("list_versions", err, "policy_id", id)
	}
	s.logger.Debug("decisionpolicy: list versions", "policy_id", id, "count", len(out))
	return out, nil
}

// ---- helpers ----

// lockActive locks the policy row for the rest of tx and rejects deleted policies.
func (s *Store) lockActive(ctx context.Context, tx *sql.Tx, id int64) (*Policy, error) {
	p, err := scanPolicy(tx.QueryRowContext(ctx, `SELECT `+policyColumns+` FROM `+s.policies+` WHERE id = $1 FOR UPDATE`, id))
	if err != nil {
		return nil, err
	}
	if p.Status != StatusActive {
		return nil, ErrDeleted
	}
	return p, nil
}

func (s *Store) inTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// fail maps a database error to the package's sentinel errors and logs it.
func (s *Store) fail(op string, err error, kv ...any) error {
	err = mapError(err, s.schema)
	args := append([]any{"op", op, "error", err}, kv...)
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrNameTaken), errors.Is(err, ErrInvalidInput), errors.Is(err, ErrDeleted):
		s.logger.Warn("decisionpolicy: operation rejected", args...)
	default:
		s.logger.Error("decisionpolicy: operation failed", args...)
	}
	return err
}

func mapError(err error, schema string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	var state interface{ SQLState() string } // implemented by lib/pq and pgx errors
	if errors.As(err, &state) {
		switch state.SQLState() {
		case "23505": // unique_violation: the active-name index
			return fmt.Errorf("%w: %v", ErrNameTaken, err)
		case "42P01": // undefined_table
			return fmt.Errorf("%w (%s.decision_policies, %s.decision_policy_versions): %v", ErrTablesMissing, schema, schema, err)
		}
	}
	return err
}

func validateName(name string) error {
	if len(name) == 0 || len(name) > maxNameLen || !namePattern.MatchString(name) {
		return fmt.Errorf("%w: name must be 1-%d characters of letters, digits, '_', '.', '-', starting with a letter or digit; got %q",
			ErrInvalidInput, maxNameLen, name)
	}
	return nil
}

func validateContent(content string) error {
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("%w: content must not be empty", ErrInvalidInput)
	}
	return nil
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

type rowScanner interface{ Scan(dest ...any) error }

func scanPolicy(r rowScanner) (*Policy, error) {
	var p Policy
	var deleted sql.NullTime
	if err := r.Scan(&p.ID, &p.Name, &p.Description, &p.CurrentVersion, &p.LatestVersion, &p.Status,
		&p.CreatedBy, &p.UpdatedBy, &p.CreatedAt, &p.UpdatedAt, &deleted); err != nil {
		return nil, err
	}
	if deleted.Valid {
		t := deleted.Time
		p.DeletedAt = &t
	}
	return &p, nil
}

func scanVersion(r rowScanner) (*PolicyVersion, error) {
	var v PolicyVersion
	if err := r.Scan(&v.ID, &v.PolicyID, &v.PolicyName, &v.Version, &v.Content, &v.Note, &v.CreatedBy, &v.CreatedAt); err != nil {
		return nil, err
	}
	return &v, nil
}
