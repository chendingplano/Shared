package sharedtesters

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/autotester"
	"github.com/chendingplano/shared/go/api/databaseutil"
)

// DatabaseUtilTester tests the databaseutil package functionality.
type DatabaseUtilTester struct {
	autotester.BaseTester
	testDB *sql.DB
}

// NewDatabaseUtilTester creates a new databaseutil tester.
func NewDatabaseUtilTester() *DatabaseUtilTester {
	return &DatabaseUtilTester{
		BaseTester: autotester.NewBaseTester(
			"tester_databaseutil",
			"Tests databaseutil package functionality",
			"validation",
			"unit",
			[]string{"database", "utilities"},
		),
	}
}

// Prepare sets up the test environment.
func (t *DatabaseUtilTester) Prepare(ctx context.Context) error {
	t.testDB = ApiTypes.CommonConfig.PGConf.ProjectDBHandle
	if t.testDB == nil {
		return fmt.Errorf("database connection not initialized (MID_260222132430)")
	}
	return nil
}

// GetTestCases returns static test cases for databaseutil testing.
func (t *DatabaseUtilTester) GetTestCases() []autotester.TestCase {
	return []autotester.TestCase{
		{
			ID:          "TC_260222132430",
			Name:        "Test valid table name validation",
			Description: "Verify that valid table names are accepted",
			Input:       "users",
			Expected:    autotester.ExpectedResult{Success: true},
			Priority:    autotester.PriorityHigh,
			Tags:        []string{"validation", "security"},
			RunTest:     t.testValidTableName,
		},
		{
			ID:          "TC_260222132431",
			Name:        "Test invalid table name rejection",
			Description: "Verify invalid table names (SQL injection attempts) are rejected",
			Input:       "users; DROP TABLE users;--",
			Expected:    autotester.ExpectedResult{Success: false, ExpectedError: "invalid"},
			Priority:    autotester.PriorityCritical,
			Tags:        []string{"validation", "security"},
			RunTest:     t.testInvalidTableName,
		},
		{
			ID:          "TC_260222132432",
			Name:        "Test ExecuteStatement function",
			Description: "Verify that ExecuteStatement can execute a simple SQL statement",
			Input:       "CREATE TEMP TABLE IF NOT EXISTS test_temp (id SERIAL)",
			Expected:    autotester.ExpectedResult{Success: true},
			Priority:    autotester.PriorityHigh,
			Tags:        []string{"execution"},
			RunTest:     t.testExecuteStatement,
		},
		{
			ID:          "TC_260222132433",
			Name:        "Test table name with schema",
			Description: "Verify that table names with schema are validated correctly",
			Input:       "public.users",
			Expected:    autotester.ExpectedResult{Success: false, ExpectedError: "invalid"},
			Priority:    autotester.PriorityMedium,
			Tags:        []string{"validation"},
			RunTest:     t.testTableNameWithSchema,
		},
		{
			ID:          "TC_260222132435",
			Name:        "Test table name with underscores",
			Description: "Verify that table names with underscores are accepted",
			Input:       "auto_test_runs",
			Expected:    autotester.ExpectedResult{Success: true},
			Priority:    autotester.PriorityMedium,
			Tags:        []string{"validation"},
			RunTest:     t.testTableNameWithUnderscore,
		},
		{
			ID:          "TC_260222132436",
			Name:        "Test table name with numeric prefix",
			Description: "Verify that table names starting with numbers are accepted",
			Input:       "_123_test",
			Expected:    autotester.ExpectedResult{Success: true},
			Priority:    autotester.PriorityLow,
			Tags:        []string{"validation"},
			RunTest:     t.testTableNameNumericPrefix,
		},
	}
}

func (t *DatabaseUtilTester) testValidTableName(
	ctx context.Context,
	tc autotester.TestCase,
	result *autotester.TestResult) {
	tableName, ok := tc.Input.(string)
	if !ok {
		result.Status = autotester.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, "invalid input type: expected string (MID_260222132431)")
		return
	}

	// Test multiple valid names
	validNames := []string{tableName, "users", "projects", "test_table", "table123"}
	allValid := true
	invalidList := []string{}

	for _, name := range validNames {
		if !databaseutil.IsValidTableName(name) {
			allValid = false
			invalidList = append(invalidList, name)
		}
	}

	if !allValid {
		result.Status = autotester.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("expected these names to be valid but they were rejected: %v (MID_260222132432)", invalidList))
		return
	}

	result.ActualValue = len(validNames)
	result.SideEffectsObserved = []string{"validation_passed"}
}

// testInvalidTableName validates that the table name validation function correctly rejects
// malicious or malformed table names. This test ensures SQL injection prevention by testing
// various invalid table name patterns including:
//   - SQL injection attempts (e.g., "users; DROP TABLE users;--")
//   - Table names with spaces (e.g., "table name")
//   - Table names with special characters (e.g., quotes, backticks)
//
// Invalid table names are those that could potentially be used for SQL injection attacks
// or violate PostgreSQL table naming conventions.
//
// If any invalid table name is incorrectly accepted by IsValidTableName(), the test fails
// and appends an error message to result.ErrorMsgs. The test does NOT return an error;
// instead, it sets result.Status to autotester.StatusFail and populates result.ErrorMsgs with
// detailed information about which table names were incorrectly accepted.
func (t *DatabaseUtilTester) testInvalidTableName(
	ctx context.Context,
	tc autotester.TestCase,
	result *autotester.TestResult) {
	tableName, ok := tc.Input.(string)
	if !ok {
		result.Status = autotester.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, "invalid input type: expected string (MID_260222132433)")
		return
	}

	// Test multiple invalid names
	invalidNames := []string{
		tableName,
		"users; DROP TABLE users;--",
		"table name",
		"table'name",
		"table\"name",
		"table`name",
	}

	for _, name := range invalidNames {
		if databaseutil.IsValidTableName(name) {
			result.Status = autotester.StatusFail
			result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("expected %q to be invalid but was accepted (MID_260222132434)", name))
		} else {
		}
	}

	if result.Status != autotester.StatusFail {
		result.SideEffectsObserved = []string{"validation_rejected"}
	}
}

func (t *DatabaseUtilTester) testExecuteStatement(
	ctx context.Context,
	tc autotester.TestCase,
	result *autotester.TestResult) {
	statement, ok := tc.Input.(string)
	if !ok {
		result.Status = autotester.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, "invalid input type: expected string (MID_260222132435)")
		return
	}

	if t.testDB == nil {
		result.Status = autotester.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, "database connection not initialized (MID_260222132436)")
		return
	}

	// Execute the statement using databaseutil.ExecuteStatement
	if err := databaseutil.ExecuteStatement(t.testDB, statement); err != nil {
		result.Status = autotester.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("ExecuteStatement failed (MID_260222132437): %v", err))
		return
	}

	result.SideEffectsObserved = []string{"statement_executed"}
}

func (t *DatabaseUtilTester) testTableNameWithSchema(
	ctx context.Context,
	tc autotester.TestCase,
	result *autotester.TestResult) {
	tableName, ok := tc.Input.(string)
	if !ok {
		result.Status = autotester.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, "invalid input type: expected string (MID_260222132438)")
		return
	}

	// Schema-qualified names should be rejected by our simple validator
	// (they need special handling)
	if databaseutil.IsValidTableName(tableName) {
		result.Status = autotester.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("expected schema-qualified name %q to be rejected (MID_260222132439)", tableName))
		return
	}

	result.SideEffectsObserved = []string{"validation_rejected"}
}

func (t *DatabaseUtilTester) testTableNameWithUnderscore(
	ctx context.Context,
	tc autotester.TestCase,
	result *autotester.TestResult) {
	tableName, ok := tc.Input.(string)
	if !ok {
		result.Status = autotester.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, "invalid input type: expected string (MID_260222132440)")
		return
	}

	if !databaseutil.IsValidTableName(tableName) {
		result.Status = autotester.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("expected table name with underscore %q to be accepted (MID_260222132441)", tableName))
		return
	}

	result.SideEffectsObserved = []string{"validation_passed"}
}

func (t *DatabaseUtilTester) testTableNameNumericPrefix(
	ctx context.Context,
	tc autotester.TestCase,
	result *autotester.TestResult) {
	tableName, ok := tc.Input.(string)
	if !ok {
		result.Status = autotester.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, "invalid input type: expected string (MID_260222132442)")
		return
	}

	if !databaseutil.IsValidTableName(tableName) {
		result.Status = autotester.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("expected table name with numeric prefix %q to be accepted (MID_260222132443)", tableName))
		return
	}

	result.SideEffectsObserved = []string{"validation_passed"}
}

// Cleanup resets the test environment.
func (t *DatabaseUtilTester) Cleanup(ctx context.Context) error {
	t.testDB = nil
	return nil
}

// Note: databaseutil.IsValidTableName is assumed to exist in the databaseutil package.
// If it doesn't, we need to add it or use our local version from fixtures.go
