## Why

Decision-model requests (`jev_compatible` and `jev_emulated` in `shared/go/api/llm`) judge a state against typed questions. Task-specific instructions — a **policy**, e.g. Red Hat's prompt-injection definition — go into the state so that every question shares them and they stay in the cacheable prompt prefix. A policy's wording drives accuracy and gets tuned over time (Red Hat measured +17.8 / −3.7 point swings from policy rewording alone), so apps need a versioned system of record for policies instead of hard-coding text. Per the workspace's storage principle, that record is a database table with versions, not files in Git.

## What Changes

- New Go package `shared/go/api/decisionpolicy`: a Postgres-backed store with which an app can create a policy, edit its metadata, query policies and versions, delete a policy, and create a new version.
- Policy versions are immutable. Changing a policy's text always creates a new version, so any past decision can be traced to the exact text it used. A policy has one *current* version, which can be moved to any of its versions (e.g. for a rollback).
- Delete is a soft delete. A deleted policy and its versions stay readable by ID for traceability, and its name becomes free for reuse.
- Two new tables in the `shared` schema, `shared.decision_policies` and `shared.decision_policy_versions`, created by a goose migration in ChenWeb's `shared_migrations/`.
- No change to the `llm` package or to the decision-model request/response shape. Apps load a policy version and place its content into the state themselves.

## Capabilities

### New Capabilities
- `decision-policy-store`: versioned storage of decision-model policies — policy identity and metadata, immutable numbered versions, current-version selection, soft delete, and the Go API that apps call for these operations.

### Modified Capabilities
<!-- none — shared/openspec/specs/ is empty; this is shared's first OpenSpec change -->

## Impact

- **Code:** new package `shared/go/api/decisionpolicy` (store, types, errors, tests). Nothing else in `shared/go` changes.
- **Database:** new tables `shared.decision_policies` and `shared.decision_policy_versions`, Postgres only. Migration file added to `ChenWeb/shared_migrations/`. Other apps (e.g. `tax`) add the same migration file only when they adopt the store.
- **Dependents:** additive only; ChenWeb and tax must still build. No HTTP endpoints or UI in this change; an app exposes the store through its own handlers when it needs to.
- **Docs:** new devdoc for the store. The `jev_emulated` devdoc gains a pointer explaining where policies come from.
- **Repo:** creates `shared/openspec/` (first OpenSpec use in `shared`).
