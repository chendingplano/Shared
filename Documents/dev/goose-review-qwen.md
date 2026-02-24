# Goose Database Migration Module - Review Recommendations

**Reviewed by:** Qwen Code  
**Date:** February 20, 2026  
**Document reviewed:** `goose.md`

---

## Overall Assessment

Well-structured and comprehensive. The documentation is clear and covers most use cases effectively.

---

## Critical Corrections

### 1. Inconsistent Package Alias

The docs use both `sharedgoose` and `gooselib` as package aliases. Standardize on one:
- Use `sharedgoose` for the wrapper module (`github.com/chendingplano/shared/go/api/goose`)
- Use `goose` for the upstream library (`github.com/pressly/goose/v3`)

**Current issue:** The "Error Types" section uses `gooselib` which is non-standard.

### 2. Missing Error Import

In the "Error Types" section, the code shows `errors.Is()` and `errors.As()` but doesn't show `"errors"` in an import block.

**Fix:** Add the import for clarity:
```go
import (
    "errors"
    gooselib "github.com/pressly/goose/v3"
)
```

### 3. Typo in Error Types Section

The import alias `gooselib` should be `goose` for consistency with standard conventions.

---

## Clarity Improvements

### 1. MigrationsFS vs MigrationsDir

The explanation is good but could benefit from a concrete warning:

> **Warning:** When using `embed.FS`, remember that `embed.FS` has no path on disk. If you plan to use `CreateMigration` or `CreateAndApply`, you must also set `MigrationsDir` to the actual filesystem path where new migration files should be written.

### 2. CreateAndApply Concurrency Warning

The concurrency warning is buried near the end of the "Creating and Applying Migrations Programmatically" section. Move this warning closer to where `CreateAndApply` is first introduced, or add a prominent note in a callout box.

### 3. Empty downSQL Behavior

The docs say "pass an empty string if rolling back is not needed" but don't clarify what happens if someone calls `Down()` on such a migration.

**Clarification needed:** Does it skip silently, error, or panic? Document the exact behavior.

---

## Missing Information

### 1. Transaction Behavior Limitations

The "How It Works" section mentions transactions, but doesn't clarify when transactions are used or their limitations.

**Add:** Some DDL statements cannot run inside transactions (e.g., `CREATE INDEX CONCURRENTLY` in PostgreSQL, `CREATE DATABASE` in most databases). These will fail if goose wraps them in a transaction. Workarounds:
- Use `-- +goose NO TRANSACTION` at the top of the migration file
- Split such operations into separate migration files

### 2. MySQL vs PostgreSQL Differences

The module supports both databases, but there's no guidance on dialect-specific considerations.

**Add a section or note covering:**
- `BIGSERIAL` (PostgreSQL) vs `BIGINT AUTO_INCREMENT` (MySQL)
- `TIMESTAMPTZ` (PostgreSQL) vs `DATETIME` (MySQL)
- `TEXT[]` arrays (PostgreSQL) have no direct MySQL equivalent
- `CONCURRENTLY` keyword for indexes (PostgreSQL only)

### 3. Rollback Safety in Production

No warning about destructive operations (e.g., `DROP COLUMN`, `DROP TABLE`) in production environments.

**Add a "Best Practices" subsection:**
- Test migrations on a staging environment first
- For destructive changes, consider a multi-step approach (e.g., stop writing to column → deploy → migrate data → drop column in a later migration)
- Always test the `Down` migration before deploying the `Up` migration

### 4. Version Type Clarification

The docs show `int64` for versions but also show timestamp strings like `20240215143000`.

**Clarify:** The numeric prefix is parsed as an `int64`. Leading zeros are significant. Timestamps beyond year 2286 will overflow `int64` when using microsecond precision (not a practical concern for millisecond/second precision).

---

## Structural Suggestions

### 1. Add Quick Start Section

Add a "Quick Start" section at the top for users who want the minimal working example immediately:

```go
// 1. Embed migrations
//go:embed migrations
var embedMigrations embed.FS

// 2. Create migrator
migrationsFS, _ := fs.Sub(embedMigrations, "migrations")
migrator, _ := sharedgoose.New(sharedgoose.Config{
    MigrationsFS: migrationsFS,
})

// 3. Run migrations
_ = migrator.Up(context.Background())
```

### 2. Enhance API Reference

The API Reference tables are excellent. Consider adding:
- Return types for each method
- Example error conditions
- Whether the method is safe for concurrent use

### 3. Expand Troubleshooting

Add guidance for:
- "What if I accidentally applied a bad migration?" — manual intervention steps
- "How do I edit an already-applied migration?" — answer: don't; create a new one to fix it
- "The version table is corrupted" — how to inspect and repair manually

---

## Minor Issues

### 1. Link Consistency

Some links use relative paths (`../../go/api/goose/goose.go`), others use full GitHub URLs.

**Recommendation:** Use relative paths for internal project links, full URLs for external resources.

### 2. PostgreSQL-Specific Examples

The SQL examples use `BIGSERIAL` and `TIMESTAMPTZ` (PostgreSQL-specific). If MySQL is supported:
- Use dialect-agnostic examples, OR
- Provide both variants side-by-side, OR
- Add a note that examples are PostgreSQL-focused

### 3. Formatting Consistency

- Some code blocks have language specifiers (`go`, `sql`, `bash`), others don't. Ensure all have them for proper syntax highlighting.
- Table formatting is inconsistent in places.

---

## Summary of Priority Actions

| Priority | Action |
|----------|--------|
| 🔴 High | Fix inconsistent package alias (`gooselib` → `goose`) |
| 🔴 High | Add missing `"errors"` import in Error Types section |
| 🟡 Medium | Add transaction behavior limitations note |
| 🟡 Medium | Add MySQL vs PostgreSQL dialect differences |
| 🟡 Medium | Move concurrency warning for `CreateAndApply` to a more prominent location |
| 🟢 Low | Add Quick Start section |
| 🟢 Low | Expand Troubleshooting section |
| 🟢 Low | Standardize link formats |
