# Test Report: goose_pg

**Project:** shared  
**Package:** `github.com/chendingplano/shared/go/api/goose`  
**Generated:** 2026-03-24 12:12:24 UTC  

## Summary

| Total | Pass | Fail | Pass Rate |
|------:|-----:|-----:|----------:|
| 57 | 57 | 0 | 100.0% |

## Test Cases

| TCID | Purpose | Statement | Result | Error | Time (ms) |
|-----:|---------|-----------|:------:|-------|----------:|
| 1 | dialectFor maps PgName to gooselib.DialectPostgres | `dialectFor(ApiTypes.PgName)` | ✅ PASS | — | 0 |
| 2 | dialectFor maps MysqlName to gooselib.DialectMySQL | `dialectFor(ApiTypes.MysqlName)` | ✅ PASS | — | 0 |
| 3 | dialectFor rejects unsupported db type 'sqlite3' | `dialectFor("sqlite3")` | ✅ PASS | — | 0 |
| 4 | dialectFor rejects empty-string db type | `dialectFor("")` | ✅ PASS | — | 0 |
| 5 | applyDefaults sets MigrationsDir to 'migrations' when not specified | `applyDefaults(ApiTypes.MigrationConfig{})` | ✅ PASS | — | 0 |
| 6 | applyDefaults sets TableName to 'db_migrations' when not specified | `applyDefaults(ApiTypes.MigrationConfig{})` | ✅ PASS | — | 0 |
| 7 | applyDefaults sets Verbose=true when Verbose field is empty | `applyDefaults(ApiTypes.MigrationConfig{})` | ✅ PASS | — | 0 |
| 8 | applyDefaults sets AllowOutOfOrder=true when field is empty | `applyDefaults(ApiTypes.MigrationConfig{})` | ✅ PASS | — | 0 |
| 9 | applyDefaults sets Verbose=false when Verbose='false' | `applyDefaults(ApiTypes.MigrationConfig{Verbose: "false"})` | ✅ PASS | — | 0 |
| 10 | applyDefaults sets AllowOutOfOrder=false when AllowOutOfOrder='false' | `applyDefaults(ApiTypes.MigrationConfig{AllowOutOfOrder: "false"})` | ✅ PASS | — | 0 |
| 11 | applyDefaults preserves custom TableName | `applyDefaults(ApiTypes.MigrationConfig{TableName: "my_migrations"})` | ✅ PASS | — | 0 |
| 12 | applyDefaults preserves custom MigrationsDir | `applyDefaults(ApiTypes.MigrationConfig{MigrationsDir: "db/migrations"})` | ✅ PASS | — | 0 |
| 13 | hasMigrationFiles returns false for nil FS without error | `hasMigrationFiles(nil)` | ✅ PASS | — | 0 |
| 14 | hasMigrationFiles returns false for empty directory | `hasMigrationFiles(os.DirFS(emptyDir))` | ✅ PASS | — | 0 |
| 15 | hasMigrationFiles returns true when a .sql file is present | `hasMigrationFiles(os.DirFS(dirWithSQL))` | ✅ PASS | — | 0 |
| 16 | hasMigrationFiles returns true when a .go migration file is present | `hasMigrationFiles(os.DirFS(dirWithGo))` | ✅ PASS | — | 0 |
| 17 | hasMigrationFiles ignores non-.sql non-.go files and returns false | `hasMigrationFiles(os.DirFS(dirWithTxt))` | ✅ PASS | — | 0 |
| 18 | hasMigrationFiles propagates ReadDir error to caller | `hasMigrationFiles(pgFailFS{err: boom})` | ✅ PASS | — | 0 |
| 19 | slugify converts mixed-case description to lowercase_underscored | `slugify("Add Tags Column")` | ✅ PASS | — | 0 |
| 20 | slugify collapses consecutive special characters to one underscore | `slugify("  add---tags!!!column  ")` | ✅ PASS | — | 0 |
| 21 | slugify returns 'migration' for whitespace-only input | `slugify("   ")` | ✅ PASS | — | 0 |
| 22 | slugify truncates output to 60 characters | `slugify("xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")` | ✅ PASS | — | 0 |
| 23 | slugify strips leading and trailing underscores from result | `slugify("---add_column---")` | ✅ PASS | — | 0 |
| 24 | slugify preserves numeric characters in description | `slugify("Add 2nd Table v3")` | ✅ PASS | — | 0 |
| 25 | buildMigrationSQL always emits a '-- +goose Up' section | `buildMigrationSQL(upSQL, downSQL)` | ✅ PASS | — | 0 |
| 26 | buildMigrationSQL emits '-- +goose Down' when downSQL is non-empty | `buildMigrationSQL(upSQL, nonEmptyDownSQL)` | ✅ PASS | — | 0 |
| 27 | buildMigrationSQL omits '-- +goose Down' when downSQL is empty/whitespace | `buildMigrationSQL(upSQL, "  ")` | ✅ PASS | — | 0 |
| 28 | buildMigrationSQL trims leading/trailing whitespace from SQL content | `buildMigrationSQL("  SELECT 1;  ", "")` | ✅ PASS | — | 0 |
| 29 | CreateMigration returns error when MigrationsDir is not set | `m.CreateMigration(desc, upSQL, downSQL) with empty cfg.MigrationsDir` | ✅ PASS | — | 0 |
| 30 | CreateMigration returns error when upSQL is empty or whitespace | `m.CreateMigration(desc, "  ", downSQL)` | ✅ PASS | — | 0 |
| 31 | CreateMigration writes a timestamped .sql file with correct goose annotations | `m.CreateMigration("Add Users Table", createSQL, dropSQL)` | ✅ PASS | — | 0 |
| 32 | CreateMigration file has no Down section when downSQL is empty | `m.CreateMigration(desc, upSQL, "")` | ✅ PASS | — | 0 |
| 33 | NewWithDB returns error for unsupported db type | `NewWithDB(nil, "badtype", cfg, logger)` | ✅ PASS | — | 0 |
| 34 | NewWithDB with empty migrations directory returns migrator with nil provider | `NewWithDB(&sql.DB{}, PgName, cfg{emptyDir}, logger)` | ✅ PASS | — | 0 |
| 35 | rebuildProvider keeps provider nil when no migration files exist | `m.rebuildProvider() with empty MigrationsFS` | ✅ PASS | — | 0 |
| 36 | CreateAndApply succeeds when MigrationsFS dir is empty (nil provider path) | `m.CreateAndApply(ctx, desc, upSQL, "")` | ✅ PASS | — | 0 |
| 37 | CreateAndApply returns filename even when provider rebuild fails (nil DB) | `m.CreateAndApply(ctx, desc, upSQL, "") — nil DB, MigrationsFS = writeDir` | ✅ PASS | — | 0 |
| 38 | Up returns nil immediately when provider is nil (no migrations to apply) | `m.Up(ctx) with nil provider` | ✅ PASS | — | 0 |
| 39 | UpByOne returns nil immediately when provider is nil | `m.UpByOne(ctx) with nil provider` | ✅ PASS | — | 0 |
| 40 | UpTo returns nil immediately when provider is nil | `m.UpTo(ctx, 42) with nil provider` | ✅ PASS | — | 0 |
| 41 | Down returns nil immediately when provider is nil | `m.Down(ctx) with nil provider` | ✅ PASS | — | 0 |
| 42 | DownTo returns nil immediately when provider is nil | `m.DownTo(ctx, 0) with nil provider` | ✅ PASS | — | 0 |
| 43 | Status returns (nil, nil) when provider is nil | `m.Status(ctx) with nil provider` | ✅ PASS | — | 0 |
| 44 | GetVersion returns (0, nil) when provider is nil | `m.GetVersion(ctx) with nil provider` | ✅ PASS | — | 0 |
| 45 | HasPending returns (false, nil) when provider is nil | `m.HasPending(ctx) with nil provider` | ✅ PASS | — | 0 |
| 46 | ListSources returns nil when provider is nil | `m.ListSources() with nil provider` | ✅ PASS | — | 0 |
| 47 | RunMigrations returns error when MigrationsDir points to an existing file | `RunMigrations with MigrationsDir = path-to-a-regular-file` | ✅ PASS | — | 0 |
| 48 | RunMigrations succeeds for empty migrations directory with valid DBType | `RunMigrations(ctx, logger, name, cfg{emptyDir}, db)` | ✅ PASS | — | 0 |
| 49 | RunProjectMigrations returns error when DB is nil | `RunProjectMigrations(ctx, logger, cfg, nil)` | ✅ PASS | — | 0 |
| 50 | RunSharedMigrations returns error when DB is nil | `RunSharedMigrations(ctx, logger, cfg, nil)` | ✅ PASS | — | 0 |
| 51 | RunAutoTesterMigrations returns error when DB is nil | `RunAutoTesterMigrations(ctx, logger, cfg, nil)` | ✅ PASS | — | 0 |
| 52 | RunProjectMigrations initialises ProjectMigrator successfully on first call | `RunProjectMigrations(ctx, logger, cfg, &sql.DB{})` | ✅ PASS | — | 0 |
| 53 | RunProjectMigrations returns 'already initialized' error on second call | `RunProjectMigrations(ctx, logger, cfg, &sql.DB{}) — second call` | ✅ PASS | — | 0 |
| 54 | RunSharedMigrations initialises SharedMigrator successfully on first call | `RunSharedMigrations(ctx, logger, cfg, &sql.DB{})` | ✅ PASS | — | 0 |
| 55 | RunSharedMigrations returns 'already initialized' error on second call | `RunSharedMigrations(ctx, logger, cfg, &sql.DB{}) — second call` | ✅ PASS | — | 0 |
| 56 | RunAutoTesterMigrations initialises AutoTesterMigrator successfully on first call | `RunAutoTesterMigrations(ctx, logger, cfg, &sql.DB{})` | ✅ PASS | — | 0 |
| 57 | RunAutoTesterMigrations returns 'already initialized' error on second call | `RunAutoTesterMigrations(ctx, logger, cfg, &sql.DB{}) — second call` | ✅ PASS | — | 0 |

---
*Generated by `goose_pg_test.go` — testname: `goose_pg`*
