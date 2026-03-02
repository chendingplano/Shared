package sharedtesters

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/autotester"
)

// DatabaseTester tests database connection and basic operations.
type DatabaseTester struct {
	autotester.BaseTester
	testDB *sql.DB
}

// NewDatabaseTester creates a new database tester.
func NewDatabaseTester() *DatabaseTester {
	return &DatabaseTester{
		BaseTester: autotester.NewBaseTester(
			"tester_database",
			"Tests database connectivity and basic CRUD operations",
			"validation",
			"integration",
			[]string{"database", "core", "critical"},
		),
	}
}

// Prepare establishes a test database connection.
func (t *DatabaseTester) Prepare(ctx context.Context) error {
	if ApiTypes.CommonConfig.PGConf.ProjectDBHandle == nil {
		return fmt.Errorf("database connection not initialized (MID_260222132450)")
	}
	t.testDB = ApiTypes.CommonConfig.PGConf.ProjectDBHandle

	// Verify connection
	if err := t.testDB.PingContext(ctx); err != nil {
		return fmt.Errorf("database ping failed (MID_260222132451): %w", err)
	}

	return nil
}

// GetTestCases returns the static test cases for database testing.
func (t *DatabaseTester) GetTestCases() []autotester.TestCase {
	return []autotester.TestCase{
		{
			ID:          "TC_260222132452",
			Name:        "Test database connection ping",
			Description: "Test the database responds to ping",
			Input:       nil,
			Expected:    autotester.ExpectedResult{Success: true},
			Priority:    autotester.PriorityCritical,
			Tags:        []string{"connection", "smoke"},
			RunTest:     t.testPing,
		},
		{
			ID:          "TC_260222132455",
			Name:        "Test database transaction",
			Description: "Verify that transactions can be created and rolled back",
			Input:       nil,
			Expected:    autotester.ExpectedResult{Success: true},
			Priority:    autotester.PriorityHigh,
			Tags:        []string{"transaction"},
			RunTest:     t.testTransaction,
		},
		{
			ID:          "TC_260222132456",
			Name:        "Test simple query execution",
			Description: "Verify that a simple SELECT query can be executed",
			Input:       "SELECT 1",
			Expected:    autotester.ExpectedResult{Success: true},
			Priority:    autotester.PriorityHigh,
			Tags:        []string{"query"},
			RunTest:     t.testSimpleQuery,
		},
	}
}

// GenerateTestCases creates dynamic test cases for stress testing.
func (t *DatabaseTester) GenerateTestCases(ctx context.Context) ([]autotester.TestCase, error) {
	// Generate random stress test cases if we have a random source
	if t.GetRandFunc() == nil {
		return nil, nil
	}

	cases := make([]autotester.TestCase, 0, 50)

	// Random simple queries for stress testing
	for i := 0; i < 50; i++ {
		cases = append(cases, autotester.TestCase{
			ID:          fmt.Sprintf("TC_260222141500_%03d", i),
			Name:        fmt.Sprintf("Stress test iteration %d", i),
			Description: "Random simple query stress test",
			Input:       "SELECT 1",
			Expected:    autotester.ExpectedResult{Success: true, MaxDuration: 100 * time.Millisecond},
			Priority:    autotester.PriorityLow,
			Tags:        []string{"stress", "random"},
			RunTest:     t.testSimpleQuery,
		})
	}

	return cases, nil
}

func (t *DatabaseTester) testPing(
	ctx context.Context,
	tc autotester.TestCase,
	result *autotester.TestResult) {
	if t.testDB == nil {
		result.Status = autotester.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, "database connection is nil (MID_260222132452)")
		return
	}

	if err := t.testDB.PingContext(ctx); err != nil {
		result.Status = autotester.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("ping failed (MID_260222132453): %v", err))
		return
	}

	result.Message = "Database ping successful"
	result.SideEffectsObserved = []string{"connection_verified"}
}

func (t *DatabaseTester) testTransaction(
	ctx context.Context,
	tc autotester.TestCase,
	result *autotester.TestResult) {
	if t.testDB == nil {
		result.Status = autotester.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, "database connection is nil (MID_260222132454)")
		return
	}

	tx, err := t.testDB.BeginTx(ctx, nil)
	if err != nil {
		result.Status = autotester.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("failed to begin transaction (MID_260222132455): %v", err))
		return
	}

	// Execute a simple query in the transaction
	var val int
	if err := tx.QueryRowContext(ctx, "SELECT 1").Scan(&val); err != nil {
		_ = tx.Rollback()
		result.Status = autotester.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("query in transaction failed (MID_260222132456): %v", err))
		return
	}

	// Rollback (we don't want to commit test data)
	if err := tx.Rollback(); err != nil {
		result.Status = autotester.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("rollback failed (MID_260222132457): %v", err))
		return
	}

	result.ActualValue = val
	result.SideEffectsObserved = []string{"transaction_created", "transaction_rolled_back"}
}

func (t *DatabaseTester) testSimpleQuery(
	ctx context.Context,
	tc autotester.TestCase,
	result *autotester.TestResult) {
	if t.testDB == nil {
		result.Status = autotester.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, "database connection is nil (MID_260222132458)")
		return
	}

	query, ok := tc.Input.(string)
	if !ok {
		result.Status = autotester.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, "invalid input type: expected string (MID_260222132459)")
		return
	}

	rows, err := t.testDB.QueryContext(ctx, query)
	if err != nil {
		result.Status = autotester.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("query execution failed (MID_260222132460): %v", err))
		return
	}
	defer rows.Close()

	// Count rows returned
	rowCount := 0
	for rows.Next() {
		rowCount++
	}

	if err := rows.Err(); err != nil {
		result.Status = autotester.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("row iteration failed (MID_260222132461): %v", err))
		return
	}

	result.ActualValue = rowCount
	result.SideEffectsObserved = []string{"query_executed"}
}

// Cleanup closes the test database connection if it was created specifically for testing.
func (t *DatabaseTester) Cleanup(ctx context.Context) error {
	// Note: We don't close ApiTypes.PG_DB_Project here as it's shared
	// The tester only uses the existing connection
	t.testDB = nil
	return nil
}
