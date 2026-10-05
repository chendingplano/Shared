## Context

`shared/go/api/llm` has two decision-model providers: `jev_compatible`, which calls the real Jev API, and `jev_emulated`, which runs on chat-model logprobs. Both take a state and typed questions. We settled on these points:

- A **policy** is task-specific instruction text, such as a definition of prompt injection. Apps put it in the state, e.g. `{"text": "<message>", "policy": "<policy text>"}`. This shares it across questions and keeps it in the cached prompt prefix.
- Policies are stored in the database and versioned. Apps need create, edit, query, delete and create-new-version operations.
- Stances such as "err on the side of blocking" belong to the app (thresholds or policy wording), not to the decision model.
- No prompt-frame "profiles" for now.

Workspace conventions that constrain the design:

- Tables owned by shared code live in the Postgres `shared` schema, and their goose migrations sit in each app's `shared_migrations/` directory. The precedent is `ChenWeb/shared_migrations/20260411095601_create_shared_nats_subjects_table.sql`.
- Data lives in the database with a version column, not in Git.
- Every operation is logged through `ApiTypes.JimoLogger`.
- Changes to `shared/go` must be additive and must not break `ChenWeb` or `tax`.
- Never point tests at the `miner` database through `TEST_DATABASE_URL`; some tests wipe rows there.

## Goals / Non-Goals

**Goals:**
- A small Go API that apps call to manage policies and fetch the text to put into a state.
- Every decision can be traced to the exact policy text it used, as a policy ID plus version number.
- Safe concurrent version creation: two writers never get the same version number.

**Non-Goals:**
- HTTP endpoints, UI, or authorization rules. Apps wrap the store in their own handlers and decide who may edit.
- Composing the state. Apps decide the state's field names and where the policy goes.
- Prompt-frame profiles, evaluation harnesses, and binding policies to particular models.
- MySQL support. The `shared` schema convention is Postgres-only.
- Changes to the `llm` package.

## Decisions

### D1. Package `shared/go/api/decisionpolicy`, independent of `llm`
Policy storage has nothing to do with how a model is called, and a policy could feed any decision provider. A separate package keeps `llm` free of database code.
*Alternative:* a subpackage of `llm`. Rejected because `llm` is the provider layer, and its only database contact is through the `UsageCaptureSink` interface.

### D2. Two tables: identity and immutable versions
```
shared.decision_policies
  id               BIGSERIAL PK
  name             VARCHAR(128) NOT NULL        -- unique among non-deleted policies
  description      TEXT NOT NULL DEFAULT ''
  current_version  INT NOT NULL                 -- version number of the current version
  latest_version   INT NOT NULL                 -- highest version number allocated
  status           VARCHAR(16) NOT NULL DEFAULT 'active'  -- 'active' | 'deleted'
  created_by, updated_by  VARCHAR(255) NOT NULL DEFAULT ''
  created_at, updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
  deleted_at       TIMESTAMPTZ NULL
  UNIQUE INDEX ON (LOWER(name)) WHERE status = 'active'

shared.decision_policy_versions
  id          BIGSERIAL PK
  policy_id   BIGINT NOT NULL REFERENCES shared.decision_policies(id)
  version     INT NOT NULL                     -- 1, 2, 3 … per policy
  content     TEXT NOT NULL                    -- non-empty, checked
  note        TEXT NOT NULL DEFAULT ''         -- why this version exists
  created_by  VARCHAR(255) NOT NULL DEFAULT ''
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
  UNIQUE (policy_id, version)
```
*Alternative:* one table with a `version` column and a row per version. Rejected because metadata such as name and description would then repeat on every row, and "current version" has no single natural home.

### D3. Versions are immutable; "edit" changes metadata only
Changing a policy's content always appends a new version. Editing covers name, description and which version is current. This is what makes traceability (goal 2) hold: a stored `(policy_id, version)` always resolves to the text that was used.
*Alternative:* allow editing the content of the current version in place, e.g. for typo fixes. Rejected because one in-place edit silently changes the meaning of every past record that cites that version, and a typo fix costs one version number.

### D4. Current version is a pointer, defaulting to the newest
`CreateVersion` makes the new version current unless the caller passes `MakeCurrent: false`, which lets a candidate wording be stored and evaluated before it goes live. `SetCurrentVersion` moves the pointer to any existing version, which is how a rollback works. `latest_version` is kept separately so that version numbers never repeat, even after a rollback.

### D5. Soft delete; the name becomes reusable
`Delete` sets `status = 'deleted'` and `deleted_at`. The partial unique index covers only active rows, so the same name can be created again. Name lookups (`GetByName`, `List`) see only active policies. Lookups by ID (`Get`, `GetVersion`) still return deleted policies and their versions, so old decisions stay traceable. There is no hard delete in the API; a hard purge, if ever needed, is an operator task.
*Alternative:* hard delete with cascade. Rejected because it breaks traceability.

### D6. Concurrency: lock the policy row while allocating a version
`CreateVersion` runs in one transaction. It locks the policy row with `SELECT … FOR UPDATE`, sets `version = latest_version + 1`, inserts the version, and updates `latest_version` (and `current_version`) on the policy. The `UNIQUE (policy_id, version)` constraint is a backstop. `Create` inserts the policy and version 1 in one transaction.

### D7. API shape
```go
type Store struct{ /* db *sql.DB, schema string, logger */ }
func NewStore(db *sql.DB, logger ApiTypes.JimoLogger, opts ...Option) *Store // WithSchema(s) for tests; default "shared"

func (s *Store) Create(ctx, CreateInput{Name, Description, Content, Note, Actor}) (*Policy, error)        // version 1, current
func (s *Store) Update(ctx, id, UpdateInput{Name *string, Description *string, Actor}) (*Policy, error)  // metadata only
func (s *Store) CreateVersion(ctx, id, VersionInput{Content, Note, Actor, MakeCurrent *bool}) (*PolicyVersion, error)
func (s *Store) SetCurrentVersion(ctx, id, version int, actor string) (*Policy, error)
func (s *Store) Delete(ctx, id, actor string) error

func (s *Store) Get(ctx, id) (*Policy, error)                                // any status
func (s *Store) GetByName(ctx, name) (*Policy, error)                        // active only
func (s *Store) List(ctx, ListFilter{NamePrefix, Limit, Offset}) ([]Policy, error) // active only, ordered by name
func (s *Store) GetVersion(ctx, id, version int) (*PolicyVersion, error)     // any status
func (s *Store) GetCurrent(ctx, name) (*PolicyVersion, error)                // active; the text to put in state
func (s *Store) ListVersions(ctx, id) ([]PolicyVersion, error)               // newest first, includes content
```
`Policy` carries the policy columns. `PolicyVersion` carries the version columns plus `PolicyName`. Errors are sentinel values that callers test with `errors.Is`: `ErrNotFound`, `ErrNameTaken`, `ErrInvalidInput`, `ErrDeleted` (when mutating a deleted policy). A mutation on a deleted policy fails; reads by ID succeed.

### D8. Validation
- Name: 1–128 characters matching `^[A-Za-z0-9][A-Za-z0-9_.-]*$`. Uniqueness is case-insensitive.
- Content: must not be empty after trimming whitespace. It is stored verbatim; whitespace is not normalised. There is no size cap beyond Postgres TEXT, but callers should keep policies prompt-sized.
- Actor: free text, stored as given; the app supplies the user ID.

### D9. Traceability is the caller's job, made easy
The store does not record which decisions used a policy. Apps put `policy_id` and `version` into `llm.Request.Metadata`, which usage capture persists on the `llm_usage_event` row. The devdoc shows the pattern.

### D10. Tests
- Unit tests with `go-sqlmock` (already in `go.mod`) cover validation, SQL flow, transaction rollback on error, and error mapping (e.g. unique violation `23505` → `ErrNameTaken`).
- One integration test runs only when `DECISIONPOLICY_TEST_DSN` is set. It creates a throwaway schema, applies the migration's DDL into it via `WithSchema`, exercises the full lifecycle (including concurrent `CreateVersion`), and drops the schema. It never reads `TEST_DATABASE_URL`.

## Risks / Trade-offs

- **The migration must be copied into every app that adopts the store.** If an app forgets, every call fails. → Mitigation: the store returns a clear error naming the missing table, and the devdoc lists the step.
- **Version numbers grow with every typo fix (D3).** → Accepted; numbers are cheap, and traceability is the point.
- **Names can be reused after a delete, so a name alone is ambiguous over time.** → Mitigation: traceability uses `policy_id` + `version`, never the name.
- **No authorization in the store.** → Apps must gate mutations in their handlers; stated in the devdoc.
- **Postgres only.** → Matches the `shared` schema convention; MySQL is legacy.

## Migration Plan

1. Add `ChenWeb/shared_migrations/<timestamp>_create_shared_decision_policies.sql` (Up creates the schema if missing, both tables and the indexes; Down drops them). It applies automatically through ChenWeb's normal goose run (`mise dev` / server start).
2. Ship the package. There are no existing callers, so nothing breaks.
3. Rollback: goose Down drops both tables. Do this only before any app stores real policies.

## Open Questions

- Should a policy have an optional `content_type` (`text` / `json`) so apps can store structured policies and validate them as JSON? Deferred: everything is TEXT for now, and JSON can be stored as text.
- Does `tax` need the store now? It is assumed not; it adopts the migration when it does.
