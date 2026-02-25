package autotester

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chendingplano/shared/go/api/databaseutil"
)

// LoadSQLFixtures loads and executes SQL fixture files against the database.
// Each file is read and executed as a single SQL statement.
// Files should be named with version prefixes for ordering: "001_users.sql", "002_projects.sql", etc.
func LoadSQLFixtures(ctx context.Context, db *sql.DB, paths ...string) error {
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read fixture %s (MID_260222132420): %w", path, err)
		}

		// Execute the entire file as SQL
		if _, err := db.ExecContext(ctx, string(data)); err != nil {
			return fmt.Errorf("execute fixture %s (MID_260222132421): %w", path, err)
		}
	}
	return nil
}

// LoadSQLFixturesFromDir loads all .sql files from a directory in alphabetical order.
// This is useful for loading a set of related fixtures.
func LoadSQLFixturesFromDir(ctx context.Context, db *sql.DB, dirPath string) error {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("read fixtures directory %s (MID_260222132422): %w", dirPath, err)
	}

	// Collect and sort .sql files
	var sqlFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			sqlFiles = append(sqlFiles, filepath.Join(dirPath, entry.Name()))
		}
	}

	// Files are already sorted alphabetically by ReadDir
	return LoadSQLFixtures(ctx, db, sqlFiles...)
}

// TruncateTable removes all rows from a table without logging individual row deletions.
// Use with caution - this cannot be rolled back in some databases.
func TruncateTable(ctx context.Context, db *sql.DB, tableName string) error {
	// Validate table name to prevent SQL injection
	if !databaseutil.IsValidTableName(tableName) {
		return fmt.Errorf("invalid table name: %s (MID_260222132423)", tableName)
	}

	query := fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", tableName)
	_, err := db.ExecContext(ctx, query)
	return err
}

// DeleteFromTable removes all rows from a table with logging.
// Safer than TRUNCATE but slower for large tables.
func DeleteFromTable(ctx context.Context, db *sql.DB, tableName string) error {
	if !databaseutil.IsValidTableName(tableName) {
		return fmt.Errorf("invalid table name: %s (MID_260222132424)", tableName)
	}

	query := fmt.Sprintf("DELETE FROM %s", tableName)
	_, err := db.ExecContext(ctx, query)
	return err
}

// DeleteTestRows removes rows tagged with a specific test_run_id.
// This is the preferred cleanup method for test data isolation.
func DeleteTestRows(ctx context.Context, db *sql.DB, tableName, runID string) error {
	if !databaseutil.IsValidTableName(tableName) {
		return fmt.Errorf("invalid table name: %s (MID_260222132425)", tableName)
	}

	query := fmt.Sprintf("DELETE FROM %s WHERE test_run_id = $1", tableName)
	_, err := db.ExecContext(ctx, query, runID)
	return err
}

// BeginTestTx starts a transaction for test isolation.
// The transaction should be rolled back in Cleanup to ensure no test data persists.
func BeginTestTx(ctx context.Context, db *sql.DB) (*sql.Tx, error) {
	return db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelReadCommitted,
		ReadOnly:  false,
	})
}

// RollbackTx safely rolls back a transaction, ignoring errors.
// Use this in defer statements for cleanup.
func RollbackTx(tx *sql.Tx) {
	if tx != nil {
		_ = tx.Rollback() // ignore error - we're cleaning up
	}
}

// CommitTx commits a transaction with error handling.
func CommitTx(ctx context.Context, tx *sql.Tx) error {
	if tx == nil {
		return fmt.Errorf("nil transaction (MID_260222132426)")
	}
	return tx.Commit()
}
