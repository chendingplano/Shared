## 1. Schema

- [x] 1.1 Write `ChenWeb/shared_migrations/<timestamp>_create_shared_decision_policies.sql` (schema `shared`, both tables, partial unique index on `LOWER(name)` for active rows, `UNIQUE (policy_id, version)`, FK, content non-empty CHECK; Down drops all) using the db-migration skill
- [x] 1.2 Apply it to the dev database and verify the tables and indexes with `\d`

## 2. Package skeleton

- [x] 2.1 Create `shared/go/api/decisionpolicy` with `Policy`, `PolicyVersion`, input types, `ListFilter`, and sentinel errors (`ErrNotFound`, `ErrNameTaken`, `ErrInvalidInput`, `ErrDeleted`)
- [x] 2.2 Implement `NewStore(db, logger, opts...)` with `WithSchema` (validated identifier, default `shared`) and the name/content validation from design D8

## 3. Mutations

- [x] 3.1 `Create`: one transaction inserting the policy and version 1; map unique violation 23505 → `ErrNameTaken`
- [x] 3.2 `Update`: metadata-only edit of name/description; reject deleted policies with `ErrDeleted`
- [x] 3.3 `CreateVersion`: transaction with `SELECT … FOR UPDATE`, `latest_version + 1`, optional `MakeCurrent`
- [x] 3.4 `SetCurrentVersion`: verify the version exists, then move the pointer
- [x] 3.5 `Delete`: soft delete setting `status`, `deleted_at` and `updated_by`
- [x] 3.6 Map "relation does not exist" (42P01) to an error naming the missing table

## 4. Queries

- [x] 4.1 `Get` (any status), `GetByName` (active, case-insensitive), `List` (active, prefix, limit/offset, ordered by name)
- [x] 4.2 `GetVersion`, `ListVersions` (newest first), `GetCurrent(name)` returning a `PolicyVersion` with `PolicyName`

## 5. Logging

- [x] 5.1 Log every operation (operation, ID/name, version, actor, error) through the `JimoLogger`

## 6. Tests

- [x] 6.1 sqlmock unit tests: validation, each operation's SQL flow, rollback on error, error mapping (23505, 42P01, no rows)
- [x] 6.2 Integration test gated by `DECISIONPOLICY_TEST_DSN`: throwaway schema via `WithSchema`, full lifecycle covering every spec scenario including concurrent `CreateVersion`, schema dropped at the end; never reads `TEST_DATABASE_URL`
- [x] 6.3 Run `go vet` and `go test ./api/decisionpolicy/...` in `shared/go`, then `go build ./...` in ChenWeb and tax

## 7. Docs and commit

- [x] 7.1 Write a devdoc for the store under `KnowledgeStore/doc-repo/devdocs/202610/` (create-devdoc skill, OpenSpec template), including the state + metadata usage pattern (design D9)
- [x] 7.2 Add a pointer from the `jev_emulated` devdoc to the policy store
- [x] 7.3 Commit via jj: shared (package + openspec), ChenWeb (migration), KnowledgeStore (devdocs) as separate commits; check `jj log` for linear history
