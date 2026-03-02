// Package tester_migration provides automated testing for the goose database migration system.
//
// # Change Log
//
// ## 2026-02-24: Removed unused helper functions
//
// ### Analysis
//
// The following functions were defined but never called anywhere in the codebase:
//
// - `getAppliedMigrations(...)` - Redundant; `syncState()` already queries applied migrations
//   and stores them in `t.state.Applied`
// - `getCurrentVersion(...)` - Redundant; `syncState()` already queries current version
//   and stores it in `t.state.CurrentVersion`
// - `hasPendingMigrations(...)` - Redundant; handlers call `migrator.HasPending()` directly
// - `getMigrationStatus(...)` - Redundant; handlers call `migrator.Status()` directly
// - `listTables(...)` - Redundant; `syncState()` already queries tables inline
// - `listMigrationFiles(...)` - Redundant; `syncState()` already scans migration files inline,
//   and helper methods like `getInitialPoolFiles()` provide similar functionality
//
// These functions were likely created during an earlier design iteration that favored
// more granular helper functions. The final implementation consolidated this logic
// into `syncState()` and the handler functions in `tester_migration_handlers.go`,
// rendering these standalone functions obsolete.
//
// ### Fix
//
// Removed all six unused functions to reduce code clutter and maintenance burden.
// No functional changes; all existing tests and operations continue to work as before.

package tester_migration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	autotester "github.com/chendingplano/shared/go/api/autotester"
)

// syncState queries the DUT to update the internal state tracking.
func (t *MigrationTester) syncState(ctx context.Context) error {
	state := MigrationSUTState{
		Applied:        make([]MigrationRecord, 0),
		FilesInDir:     make([]MigrationFile, 0),
		Tables:         make(map[string]bool),
		CurrentVersion: 0,
	}

	// 1. Query current version from tracking table
	versionQuery := fmt.Sprintf(`
		SELECT COALESCE(MAX(version_id), 0) FROM %s
	`, autotester.AutotesterConfig.MigrationConfig.TableName)
	var currentVersion int64
	if err := autotester.AutotesterConfig.MigrationDBHandle.QueryRowContext(ctx, versionQuery).Scan(&currentVersion); err != nil {
		// Table might not exist yet, that's ok
		currentVersion = 0
	}
	state.CurrentVersion = currentVersion

	// 2. Query applied migrations from tracking table
	appliedQuery := fmt.Sprintf(`
		SELECT version_id, migration_type 
		FROM %s 
		ORDER BY version_id ASC
	`, autotester.AutotesterConfig.MigrationConfig.TableName)
	rows, err := autotester.AutotesterConfig.MigrationDBHandle.QueryContext(ctx, appliedQuery)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var version int64
			var migrationType string
			if err := rows.Scan(&version, &migrationType); err == nil {
				state.Applied = append(state.Applied, MigrationRecord{
					Version:  version,
					Filename: fmt.Sprintf("%d_%s.sql", version, migrationType),
					Applied:  true,
				})
			}
		}
	}

	// 3. Scan migrations directory for all .sql files
	entries, err := os.ReadDir(autotester.AutotesterConfig.MigrationConfig.MigrationsDir)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
				version := extractVersionFromFilename(t.logger, entry.Name())
				isApplied := false
				for _, record := range state.Applied {
					if record.Version == version {
						isApplied = true
						break
					}
				}

				state.FilesInDir = append(state.FilesInDir, MigrationFile{
					Version:   version,
					Filename:  entry.Name(),
					IsApplied: isApplied,
				})
			}
		}
	}

	// 4. Query for all testonly_ tables in DUT
	tablesQuery := `
		SELECT table_name FROM information_schema.tables 
		WHERE table_schema = 'public' AND table_name LIKE 'testonly_%'
	`
	rows, err = autotester.AutotesterConfig.MigrationDBHandle.QueryContext(ctx, tablesQuery)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var tableName string
			if err := rows.Scan(&tableName); err == nil {
				state.Tables[tableName] = true
			}
		}
	}

	t.state = state
	return nil
}

// resetToState brings the DUT to the exact state described by preState.
func (t *MigrationTester) resetToState(ctx context.Context, preState MigrationSUTState) error {
	// 1. Drop tracking table + all testonly_ tables
	if err := t.resetDUT(ctx); err != nil {
		return fmt.Errorf("resetDUT failed (MID_260224100030): %w", err)
	}

	// 2. Clear and repopulate testonly_ dir with files from preState.FilesInDir
	if err := t.clearMigrationsDir(ctx); err != nil {
		return fmt.Errorf("clearMigrationsDir failed (MID_260224100031): %w", err)
	}

	// Write all files from preState
	for _, file := range preState.FilesInDir {
		if err := t.writeMigrationFile(file); err != nil {
			return fmt.Errorf("writeMigrationFile failed (MID_260224100032): %w", err)
		}
	}

	// 3. Rebuild migrator
	migrator := t.buildMigrator(true)
	if migrator == nil {
		return fmt.Errorf("buildMigrator failed (MID_260224100033)")
	}

	// 4. Apply exactly the migrations in preState.Applied using UpByOne
	for _, record := range preState.Applied {
		if err := migrator.UpByOne(ctx); err != nil {
			return fmt.Errorf("apply migration %d (MID_260224100034): %w", record.Version, err)
		}
	}

	// 5. Verify state matches via syncState
	if err := t.syncState(ctx); err != nil {
		return fmt.Errorf("syncState failed (MID_260224100035): %w", err)
	}

	if t.state.CurrentVersion != preState.CurrentVersion {
		return fmt.Errorf("state mismatch (MID_260224100036): expected version %d, got %d",
			preState.CurrentVersion, t.state.CurrentVersion)
	}

	return nil
}

// resetDUT drops the tracking table and all testonly_ tables.
func (t *MigrationTester) resetDUT(ctx context.Context) error {
	// Drop tracking table
	_, err := autotester.AutotesterConfig.MigrationDBHandle.ExecContext(ctx, "DROP TABLE IF EXISTS "+autotester.AutotesterConfig.MigrationConfig.TableName)
	if err != nil {
		return fmt.Errorf("drop tracking table (MID_260224100037): %w", err)
	}

	// Drop all testonly_ tables
	return t.dropTestTables(ctx)
}

// writeMigrationFile writes a migration file to the migrations directory.
func (t *MigrationTester) writeMigrationFile(file MigrationFile) error {
	content := buildMigrationFileContent(file.UpSQL, file.DownSQL)
	fullPath := filepath.Join(autotester.AutotesterConfig.MigrationConfig.MigrationsDir, file.Filename)
	return os.WriteFile(fullPath, []byte(content), 0644)
}

// extractVersionFromFilename parses the version number from a migration filename.
// Expected format: YYYYMMDDHHMMSS_description.sql or version_description.sql
func extractVersionFromFilename(logger ApiTypes.JimoLogger, filename string) int64 {
	// Remove .sql suffix
	name := strings.TrimSuffix(filename, ".sql")

	// Try to extract timestamp-based version (first 14 digits)
	if len(name) >= 14 {
		var versionStr string
		for _, c := range name {
			if c >= '0' && c <= '9' {
				versionStr += string(c)
				if len(versionStr) == 14 {
					break
				}
			} else {
				break
			}
		}
		if len(versionStr) == 14 {
			var year, month, day, hour, min, sec int64
			_, err := fmt.Sscanf(versionStr, "%04d%02d%02d%02d%02d%02d", &year, &month, &day, &hour, &min, &sec)
			if err == nil {
				// Return a simplified version number based on position in sequence
				// For testing purposes, we just use a hash of the timestamp
				return year*10000000000 + month*100000000 + day*1000000 + hour*10000 + min*100 + sec
			}
			logger.Error("failed parsing version", "error", err)
		}
	}

	// Fallback: try to parse leading digits as version
	var version int64
	fmt.Sscanf(name, "%d", &version)
	return version
}
