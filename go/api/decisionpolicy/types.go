// Package decisionpolicy stores decision-model policies: task-specific
// instruction text (for example, a definition of prompt injection) that apps
// place into the state of a decision request (llm.ProviderJevCompatible or
// llm.ProviderJevEmulated) so that every question shares it.
//
// A policy has a name, a description, and numbered versions. Versions are
// immutable: changing a policy's text always creates a new version, so a
// stored (policy ID, version) pair always resolves to the exact text a
// decision used. One version is current; it defaults to the newest and can be
// moved to any version (rollback). Delete is soft: the policy and its versions
// stay readable by ID and the name becomes reusable.
//
// The tables live in the Postgres "shared" schema and are created by the goose
// migration *_create_shared_decision_policies.sql in each adopting app's
// shared_migrations/ directory. Spec:
// shared/openspec/specs/decision-policy-store/spec.md.
package decisionpolicy

import (
	"errors"
	"time"
)

// Policy status values.
const (
	StatusActive  = "active"
	StatusDeleted = "deleted"
)

// Sentinel errors; test with errors.Is.
var (
	ErrNotFound      = errors.New("decisionpolicy: not found")
	ErrNameTaken     = errors.New("decisionpolicy: name is used by another active policy")
	ErrInvalidInput  = errors.New("decisionpolicy: invalid input")
	ErrDeleted       = errors.New("decisionpolicy: policy is deleted")
	ErrTablesMissing = errors.New("decisionpolicy: tables missing; run the *_create_shared_decision_policies.sql migration")
)

// Policy is a policy's identity and metadata.
type Policy struct {
	ID             int64      `json:"id"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	CurrentVersion int        `json:"current_version"`
	LatestVersion  int        `json:"latest_version"`
	Status         string     `json:"status"`
	CreatedBy      string     `json:"created_by"`
	UpdatedBy      string     `json:"updated_by"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty"`
}

// PolicyVersion is one immutable version of a policy's text.
type PolicyVersion struct {
	ID         int64     `json:"id"`
	PolicyID   int64     `json:"policy_id"`
	PolicyName string    `json:"policy_name"`
	Version    int       `json:"version"`
	Content    string    `json:"content"`
	Note       string    `json:"note"`
	CreatedBy  string    `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
}

// CreateInput creates a policy and its version 1.
type CreateInput struct {
	Name        string
	Description string
	Content     string
	Note        string // why version 1 exists
	Actor       string // user ID of the caller
}

// UpdateInput edits metadata only; nil fields are left unchanged.
type UpdateInput struct {
	Name        *string
	Description *string
	Actor       string
}

// VersionInput appends a version. MakeCurrent defaults to true when nil.
type VersionInput struct {
	Content     string
	Note        string
	Actor       string
	MakeCurrent *bool
}

// ListFilter selects active policies. Limit defaults to 100 and is capped at 1000.
type ListFilter struct {
	NamePrefix string // case-insensitive
	Limit      int
	Offset     int
}
