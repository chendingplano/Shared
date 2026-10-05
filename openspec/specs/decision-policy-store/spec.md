# decision-policy-store Specification

## Purpose
Versioned storage of decision-model policies: task-specific instruction text that apps place into a decision request's state. Covers policy identity and metadata, immutable numbered versions, current-version selection, soft delete, and the Go API in `shared/go/api/decisionpolicy`.
## Requirements
### Requirement: Create a policy
The store SHALL create a policy from a name, an optional description, non-empty content, an optional note and an actor. In one transaction it SHALL create the policy and its version 1, and make version 1 current.

#### Scenario: Successful create
- **WHEN** an app creates policy `prompt_injection` with content "Your task is to determine…"
- **THEN** the store returns a policy with `current_version` = 1 and `latest_version` = 1, and version 1 holds that exact content

#### Scenario: Name already used by an active policy
- **WHEN** an app creates a policy whose name matches an active policy's name case-insensitively
- **THEN** the store returns `ErrNameTaken` and creates nothing

#### Scenario: Invalid input
- **WHEN** the name is empty, longer than 128 characters, or contains characters outside `[A-Za-z0-9_.-]` (or starts with `_`, `.` or `-`), or the content is empty after trimming whitespace
- **THEN** the store returns `ErrInvalidInput` and creates nothing

### Requirement: Versions are immutable
The store SHALL NOT offer any operation that changes a version's content or note after the version is created.

#### Scenario: Reading an old version after newer ones exist
- **WHEN** a policy has versions 1–3 and an app reads version 1
- **THEN** the content returned is byte-identical to what was stored when version 1 was created

### Requirement: Create a new version
The store SHALL append a version numbered `latest_version + 1` with the given content, note and actor. By default it SHALL make the new version current; when the caller sets `MakeCurrent` to false, the current version SHALL stay unchanged. Version numbers SHALL never repeat within a policy.

#### Scenario: New version becomes current
- **WHEN** an app creates a new version of a policy whose latest version is 2
- **THEN** the new version is number 3 and the policy's `current_version` is 3

#### Scenario: Staged version
- **WHEN** an app creates a new version with `MakeCurrent` = false on a policy whose current version is 2
- **THEN** the new version is number 3 and `current_version` stays 2

#### Scenario: Concurrent writers
- **WHEN** two callers create a new version of the same policy at the same time
- **THEN** both succeed with distinct consecutive version numbers

#### Scenario: Numbers do not repeat after a rollback
- **WHEN** a policy has versions 1–3, its current version is set back to 1, and a new version is then created
- **THEN** the new version is number 4

### Requirement: Edit policy metadata
The store SHALL let an app change a policy's name and description, and SHALL record the actor and update time. Editing SHALL NOT create a version.

#### Scenario: Rename
- **WHEN** an app renames an active policy to a name no other active policy uses
- **THEN** the policy has the new name and its versions are unchanged

#### Scenario: Rename to a taken name
- **WHEN** an app renames a policy to a name used by another active policy (case-insensitive)
- **THEN** the store returns `ErrNameTaken` and the policy keeps its old name

### Requirement: Select the current version
The store SHALL let an app make any existing version of a policy current.

#### Scenario: Rollback
- **WHEN** an app sets the current version of a policy with versions 1–3 to 1
- **THEN** `GetCurrent` by the policy's name returns version 1's content

#### Scenario: Unknown version
- **WHEN** an app sets the current version to a number that does not exist for that policy
- **THEN** the store returns `ErrNotFound` and the current version is unchanged

### Requirement: Soft delete
Deleting a policy SHALL mark it deleted, record the actor and time, and keep the policy and all its versions in the database. A deleted policy's name SHALL become available to a new policy. Mutations on a deleted policy SHALL return `ErrDeleted`.

#### Scenario: Deleted policy hidden from name lookups
- **WHEN** an app deletes policy `prompt_injection`
- **THEN** `GetByName`, `GetCurrent` and `List` no longer return it

#### Scenario: Deleted policy still traceable by ID
- **WHEN** an app reads a deleted policy, or one of its versions, by policy ID
- **THEN** the store returns it, with status `deleted`

#### Scenario: Name reuse after delete
- **WHEN** policy `prompt_injection` was deleted and an app creates a new policy with that name
- **THEN** the create succeeds with a new policy ID, starting at version 1

#### Scenario: Mutating a deleted policy
- **WHEN** an app updates, creates a version for, sets the current version of, or deletes an already deleted policy
- **THEN** the store returns `ErrDeleted` and changes nothing

### Requirement: Query policies and versions
The store SHALL provide lookups by policy ID (any status) and by name (active only, case-insensitive), a list of active policies ordered by name with an optional name prefix and paging, a single version by policy ID and number, all versions of a policy (newest first), and the current version's content by policy name. Lookups that match nothing SHALL return `ErrNotFound`.

#### Scenario: Fetch policy text for a decision request
- **WHEN** an app calls `GetCurrent("prompt_injection")`
- **THEN** it receives the policy ID, version number and content of the current version, which it can place into the decision-model state and record in request metadata

#### Scenario: Missing policy
- **WHEN** an app looks up a name or ID that does not exist
- **THEN** the store returns `ErrNotFound`

### Requirement: Logging
Every store operation SHALL be logged through the `JimoLogger` passed to the store, including the operation, policy ID or name, version (when relevant), actor (for mutations) and any error.

#### Scenario: Failed mutation is logged
- **WHEN** a create fails with `ErrNameTaken`
- **THEN** a log entry records the operation, the name and the error

### Requirement: Storage schema
The store SHALL use the tables `decision_policies` and `decision_policy_versions` in a configurable Postgres schema that defaults to `shared`. A goose migration in each adopting app's `shared_migrations/` directory SHALL create them.

#### Scenario: Missing tables
- **WHEN** the store is used against a database where the migration has not run
- **THEN** the returned error names the missing table

