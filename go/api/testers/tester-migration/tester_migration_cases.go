// Package tester_migration provides automated testing for the goose database migration system.

package tester_migration

import (
	"context"
	"fmt"
	"time"

	"github.com/chendingplano/shared/go/api/autotesters"
)

// GetTestCases returns the static test cases for migration testing.
func (t *MigrationTester) GetTestCases() []autotesters.TestCase {
	return []autotesters.TestCase{
		// Category 1: Basic Apply Operations
		{
			ID:          "TC_2026022301",
			Name:        "Apply all migrations from empty DB",
			Description: "Tests that Up applies all pending migrations from an empty database",
			Purpose:     "regression",
			Type:        "integration",
			Tags:        []string{"migration", "apply", "critical"},
			Input: migrationInput{
				Operation:       OpUp,
				AllowOutOfOrder: true,
				PreState:        MigrationSUTState{Applied: []MigrationRecord{}, FilesInDir: t.getInitialPoolFiles(), CurrentVersion: 0},
			},
			Expected: autotesters.ExpectedResult{
				Success:     true,
				SideEffects: []string{string(SideEffectTrackingTableCreated), string(SideEffectSchemaTableApplied)},
				MaxDuration: 500 * time.Millisecond,
			},
			Priority: autotesters.PriorityCritical,
		},
		{
			ID:          "TC_2026022302",
			Name:        "Up is no-op when already current",
			Description: "Tests that Up returns successfully when all migrations are already applied",
			Purpose:     "regression",
			Type:        "integration",
			Tags:        []string{"migration", "apply", "edge-case"},
			Input: migrationInput{
				Operation:       OpUp,
				AllowOutOfOrder: true,
				PreState:        t.getFullyAppliedState(),
			},
			Expected: autotesters.ExpectedResult{
				Success:     true,
				MaxDuration: 100 * time.Millisecond,
			},
			Priority: autotesters.PriorityHigh,
		},
		{
			ID:          "TC_2026022303",
			Name:        "Apply migrations one by one with UpByOne",
			Description: "Tests that UpByOne applies exactly one migration at a time",
			Purpose:     "regression",
			Type:        "integration",
			Tags:        []string{"migration", "apply"},
			Input: migrationInput{
				Operation:       OpUpByOne,
				AllowOutOfOrder: true,
				PreState:        MigrationSUTState{Applied: []MigrationRecord{}, FilesInDir: t.getInitialPoolFiles(), CurrentVersion: 0},
			},
			Expected: autotesters.ExpectedResult{
				Success:     true,
				SideEffects: []string{string(SideEffectTrackingTableCreated), string(SideEffectSchemaTableApplied)},
				MaxDuration: 200 * time.Millisecond,
			},
			Priority: autotesters.PriorityHigh,
		},
		{
			ID:          "TC_2026022304",
			Name:        "UpByOne returns error when all applied",
			Description: "Tests that UpByOne fails when there are no pending migrations",
			Purpose:     "regression",
			Type:        "integration",
			Tags:        []string{"migration", "apply", "error"},
			Input: migrationInput{
				Operation:       OpUpByOne,
				AllowOutOfOrder: true,
				PreState:        t.getFullyAppliedState(),
			},
			Expected: autotesters.ExpectedResult{
				Success:       false,
				ExpectedError: "no more pending migrations",
				MaxDuration:   100 * time.Millisecond,
			},
			Priority: autotesters.PriorityHigh,
		},
		{
			ID:          "TC_2026022305",
			Name:        "UpTo applies migrations up to a target version",
			Description: "Tests that UpTo applies all migrations up to and including the target",
			Purpose:     "regression",
			Type:        "integration",
			Tags:        []string{"migration", "apply"},
			Input: migrationInput{
				Operation:       OpUpTo,
				TargetVersion:   5,
				AllowOutOfOrder: true,
				PreState:        MigrationSUTState{Applied: []MigrationRecord{}, FilesInDir: t.getInitialPoolFiles(), CurrentVersion: 0},
			},
			Expected: autotesters.ExpectedResult{
				Success:       true,
				ExpectedValue: int64(5),
				MaxDuration:   300 * time.Millisecond,
			},
			Priority: autotesters.PriorityHigh,
		},
		{
			ID:          "TC_2026022306",
			Name:        "UpTo returns error for nonexistent version",
			Description: "Tests that UpTo fails when the target version does not exist",
			Purpose:     "regression",
			Type:        "integration",
			Tags:        []string{"migration", "apply", "error"},
			Input: migrationInput{
				Operation:       OpUpTo,
				TargetVersion:   9999,
				AllowOutOfOrder: true,
				PreState:        MigrationSUTState{Applied: []MigrationRecord{}, FilesInDir: t.getInitialPoolFiles(), CurrentVersion: 0},
			},
			Expected: autotesters.ExpectedResult{
				Success:       false,
				ExpectedError: "not found",
				MaxDuration:   100 * time.Millisecond,
			},
			Priority: autotesters.PriorityMedium,
		},

		// Category 2: Rollback Operations
		{
			ID:          "TC_2026022307",
			Name:        "Roll back one migration (has Down SQL)",
			Description: "Tests that Down rolls back the most recently applied migration",
			Purpose:     "regression",
			Type:        "integration",
			Tags:        []string{"migration", "rollback", "critical"},
			Input: migrationInput{
				Operation:       OpDown,
				AllowOutOfOrder: true,
				PreState:        t.getFullyAppliedState(),
			},
			Expected: autotesters.ExpectedResult{
				Success:     true,
				SideEffects: []string{string(SideEffectSchemaTableDropped)},
				MaxDuration: 200 * time.Millisecond,
			},
			Priority: autotesters.PriorityCritical,
		},
		{
			ID:          "TC_2026022308",
			Name:        "Down returns error when nothing is applied",
			Description: "Tests that Down fails when there are no migrations to rollback",
			Purpose:     "regression",
			Type:        "integration",
			Tags:        []string{"migration", "rollback", "error"},
			Input: migrationInput{
				Operation:       OpDown,
				AllowOutOfOrder: true,
				PreState:        MigrationSUTState{Applied: []MigrationRecord{}, FilesInDir: t.getInitialPoolFiles(), CurrentVersion: 0},
			},
			Expected: autotesters.ExpectedResult{
				Success:       false,
				ExpectedError: "no version found",
				MaxDuration:   100 * time.Millisecond,
			},
			Priority: autotesters.PriorityHigh,
		},
		{
			ID:          "TC_2026022309",
			Name:        "DownTo rolls back to a target version",
			Description: "Tests that DownTo rolls back all migrations newer than the target",
			Purpose:     "regression",
			Type:        "integration",
			Tags:        []string{"migration", "rollback"},
			Input: migrationInput{
				Operation:       OpDownTo,
				TargetVersion:   5,
				AllowOutOfOrder: true,
				PreState:        t.getFullyAppliedState(),
			},
			Expected: autotesters.ExpectedResult{
				Success:       true,
				ExpectedValue: int64(5),
				MaxDuration:   300 * time.Millisecond,
			},
			Priority: autotesters.PriorityHigh,
		},
		{
			ID:          "TC_2026022310",
			Name:        "DownTo(0) rolls back all migrations",
			Description: "Tests that DownTo(0) rolls back all applied migrations",
			Purpose:     "regression",
			Type:        "integration",
			Tags:        []string{"migration", "rollback"},
			Input: migrationInput{
				Operation:       OpDownTo,
				TargetVersion:   0,
				AllowOutOfOrder: true,
				PreState:        t.getFullyAppliedState(),
			},
			Expected: autotesters.ExpectedResult{
				Success:       true,
				ExpectedValue: int64(0),
				SideEffects:   []string{string(SideEffectSchemaTableDropped)},
				MaxDuration:   500 * time.Millisecond,
			},
			Priority: autotesters.PriorityHigh,
		},

		// Category 3: Status Inspection
		{
			ID:          "TC_2026022311",
			Name:        "GetVersion returns 0 when nothing is applied",
			Description: "Tests that GetVersion returns 0 for an empty database",
			Purpose:     "regression",
			Type:        "integration",
			Tags:        []string{"migration", "status"},
			Input: migrationInput{
				Operation:       OpGetVersion,
				AllowOutOfOrder: true,
				PreState:        MigrationSUTState{Applied: []MigrationRecord{}, FilesInDir: t.getInitialPoolFiles(), CurrentVersion: 0},
			},
			Expected: autotesters.ExpectedResult{
				Success:       true,
				ExpectedValue: int64(0),
				MaxDuration:   100 * time.Millisecond,
			},
			Priority: autotesters.PriorityMedium,
		},
		{
			ID:          "TC_2026022312",
			Name:        "HasPending returns true when pending migrations exist",
			Description: "Tests that HasPending returns true when there are unapplied migrations",
			Purpose:     "regression",
			Type:        "integration",
			Tags:        []string{"migration", "status"},
			Input: migrationInput{
				Operation:       OpHasPending,
				AllowOutOfOrder: true,
				PreState:        MigrationSUTState{Applied: []MigrationRecord{}, FilesInDir: t.getInitialPoolFiles(), CurrentVersion: 0},
			},
			Expected: autotesters.ExpectedResult{
				Success:       true,
				ExpectedValue: true,
				MaxDuration:   100 * time.Millisecond,
			},
			Priority: autotesters.PriorityMedium,
		},
		{
			ID:          "TC_2026022313",
			Name:        "HasPending returns false when fully applied",
			Description: "Tests that HasPending returns false when all migrations are applied",
			Purpose:     "regression",
			Type:        "integration",
			Tags:        []string{"migration", "status"},
			Input: migrationInput{
				Operation:       OpHasPending,
				AllowOutOfOrder: true,
				PreState:        t.getFullyAppliedState(),
			},
			Expected: autotesters.ExpectedResult{
				Success:       true,
				ExpectedValue: false,
				MaxDuration:   100 * time.Millisecond,
			},
			Priority: autotesters.PriorityMedium,
		},
		{
			ID:          "TC_2026022314",
			Name:        "Status returns correct applied/pending counts",
			Description: "Tests that Status returns the correct state of all migrations",
			Purpose:     "regression",
			Type:        "integration",
			Tags:        []string{"migration", "status"},
			Input: migrationInput{
				Operation:       OpStatus,
				AllowOutOfOrder: true,
				PreState:        MigrationSUTState{Applied: []MigrationRecord{}, FilesInDir: t.getInitialPoolFiles(), CurrentVersion: 0},
			},
			Expected: autotesters.ExpectedResult{
				Success:     true,
				MaxDuration: 100 * time.Millisecond,
			},
			Priority: autotesters.PriorityMedium,
		},

		// Category 4: Migration Creation
		{
			ID:          "TC_2026022315",
			Name:        "CreateAndApply writes file and applies migration",
			Description: "Tests that CreateAndApply creates a file and applies it immediately",
			Purpose:     "regression",
			Type:        "integration",
			Tags:        []string{"migration", "create"},
			Input: migrationInput{
				Operation:       OpCreateAndApply,
				Description:     "add_test_column",
				UpSQL:           "CREATE TABLE testonly_dynamic_table (id BIGSERIAL PRIMARY KEY, data TEXT)",
				DownSQL:         "DROP TABLE IF EXISTS testonly_dynamic_table",
				AllowOutOfOrder: true,
				PreState:        MigrationSUTState{Applied: []MigrationRecord{}, FilesInDir: t.getInitialPoolFiles(), CurrentVersion: 0},
			},
			Expected: autotesters.ExpectedResult{
				Success:       true,
				SideEffects:   []string{string(SideEffectMigrationFileWritten), string(SideEffectSchemaTableApplied)},
				MaxDuration:   300 * time.Millisecond,
			},
			Priority: autotesters.PriorityHigh,
		},
		{
			ID:          "TC_2026022316",
			Name:        "CreateAndApply with empty downSQL succeeds",
			Description: "Tests that CreateAndApply succeeds even with empty Down SQL",
			Purpose:     "regression",
			Type:        "integration",
			Tags:        []string{"migration", "create", "edge-case"},
			Input: migrationInput{
				Operation:       OpCreateAndApply,
				Description:     "no_rollback_migration",
				UpSQL:           "CREATE TABLE testonly_no_down_table (id BIGSERIAL PRIMARY KEY)",
				DownSQL:         "",
				AllowOutOfOrder: true,
				PreState:        MigrationSUTState{Applied: []MigrationRecord{}, FilesInDir: t.getInitialPoolFiles(), CurrentVersion: 0},
			},
			Expected: autotesters.ExpectedResult{
				Success:       true,
				SideEffects:   []string{string(SideEffectMigrationFileWritten), string(SideEffectSchemaTableApplied)},
				MaxDuration:   300 * time.Millisecond,
			},
			Priority: autotesters.PriorityHigh,
		},
		{
			ID:          "TC_2026022317",
			Name:        "CreateAndApply with invalid SQL returns error",
			Description: "Tests that CreateAndApply fails with invalid SQL syntax",
			Purpose:     "regression",
			Type:        "integration",
			Tags:        []string{"migration", "create", "error"},
			Input: migrationInput{
				Operation:       OpCreateAndApply,
				Description:     "invalid_sql_test",
				UpSQL:           "INVALID SQL SYNTAX HERE",
				DownSQL:         "DROP TABLE IF EXISTS testonly_invalid_table",
				AllowOutOfOrder: true,
				PreState:        MigrationSUTState{Applied: []MigrationRecord{}, FilesInDir: t.getInitialPoolFiles(), CurrentVersion: 0},
			},
			Expected: autotesters.ExpectedResult{
				Success:       false,
				ExpectedError: "syntax error",
				MaxDuration:   200 * time.Millisecond,
			},
			Priority: autotesters.PriorityHigh,
		},

		// Category 5: Edge Cases
		{
			ID:          "TC_2026022318",
			Name:        "Tracking table is auto-created on first Up",
			Description: "Tests that the goose version tracking table is created automatically",
			Purpose:     "regression",
			Type:        "integration",
			Tags:        []string{"migration", "edge-case", "critical"},
			Input: migrationInput{
				Operation:       OpUp,
				AllowOutOfOrder: true,
				PreState:        MigrationSUTState{Applied: []MigrationRecord{}, FilesInDir: t.getInitialPoolFiles(), CurrentVersion: 0, Tables: make(map[string]bool)},
			},
			Expected: autotesters.ExpectedResult{
				Success:     true,
				SideEffects: []string{string(SideEffectTrackingTableCreated)},
				MaxDuration: 500 * time.Millisecond,
			},
			Priority: autotesters.PriorityCritical,
		},
		{
			ID:          "TC_2026022319",
			Name:        "ListSources returns migration files",
			Description: "Tests that ListSources returns all known migration sources",
			Purpose:     "regression",
			Type:        "integration",
			Tags:        []string{"migration", "status"},
			Input: migrationInput{
				Operation:       OpListSources,
				AllowOutOfOrder: true,
				PreState:        MigrationSUTState{Applied: []MigrationRecord{}, FilesInDir: t.getInitialPoolFiles(), CurrentVersion: 0},
			},
			Expected: autotesters.ExpectedResult{
				Success:     true,
				MaxDuration: 100 * time.Millisecond,
			},
			Priority: autotesters.PriorityMedium,
		},
	}
}

// GenerateTestCases creates dynamic test cases for comprehensive migration testing.
func (t *MigrationTester) GenerateTestCases(ctx context.Context) ([]autotesters.TestCase, error) {
	randFunc := t.GetRandFunc()
	if randFunc == nil {
		return nil, nil // Use static cases only
	}

	cases := make([]autotesters.TestCase, 0, t.cfg.NumDynamicCases)

	// Operation weights for weighted random selection
	opWeights := []int{30, 15, 10, 20, 10, 5, 5, 5, 0} // Up, UpByOne, UpTo, Down, DownTo, Status, GetVersion, HasPending, CreateAndApply

	for i := 0; i < t.cfg.NumDynamicCases; i++ {
		// Select operation based on weights
		op := t.selectOperation(randFunc.Intn(100), opWeights)

		// Generate pre-state
		preState := t.generatePreState(ctx, randFunc)

		// Generate target version for UpTo/DownTo
		var targetVersion int64
		if op == OpUpTo || op == OpDownTo {
			if len(preState.FilesInDir) > 0 {
				idx := randFunc.Intn(len(preState.FilesInDir))
				targetVersion = preState.FilesInDir[idx].Version
			} else {
				targetVersion = 1
			}
		}

		// Build test case
		tc := autotesters.TestCase{
			ID:          fmt.Sprintf("TC_DYN_%04d", i+1),
			Name:        fmt.Sprintf("Dynamic migration test %d", i+1),
			Description: fmt.Sprintf("Randomly generated test for operation %s", op),
			Purpose:     "regression",
			Type:        "integration",
			Tags:        []string{"migration", "dynamic", "random"},
			Input: migrationInput{
				Operation:       op,
				TargetVersion:   targetVersion,
				AllowOutOfOrder: randFunc.Intn(100) < 70, // 70% chance of true
				PreState:        preState,
			},
			Expected: autotesters.ExpectedResult{
				Success:     true,
				MaxDuration: 500 * time.Millisecond,
			},
			Priority: autotesters.PriorityLow,
		}

		cases = append(cases, tc)
	}

	return cases, nil
}

// selectOperation selects an operation based on weighted distribution.
func (t *MigrationTester) selectOperation(randVal int, weights []int) MigrationOperation {
	cumulative := 0
	for i, weight := range weights {
		cumulative += weight
		if randVal < cumulative {
			switch i {
			case 0:
				return OpUp
			case 1:
				return OpUpByOne
			case 2:
				return OpUpTo
			case 3:
				return OpDown
			case 4:
				return OpDownTo
			case 5:
				return OpStatus
			case 6:
				return OpGetVersion
			case 7:
				return OpHasPending
			case 8:
				return OpCreateAndApply
			}
		}
	}
	return OpUp
}

// generatePreState generates a random pre-state for dynamic test cases.
func (t *MigrationTester) generatePreState(ctx context.Context, randFunc interface{}) MigrationSUTState {
	// Get initial pool files
	files := t.getInitialPoolFiles()

	// Randomly select how many migrations are applied
	numApplied := 0
	if len(files) > 0 {
		// Use reflection or type assertion to get random int
		// For now, return empty state
	}

	return MigrationSUTState{
		Applied:        make([]MigrationRecord, numApplied),
		FilesInDir:     files,
		Tables:         make(map[string]bool),
		CurrentVersion: 0,
	}
}

// getInitialPoolFiles returns the migration files from the initial pool.
func (t *MigrationTester) getInitialPoolFiles() []MigrationFile {
	files := make([]MigrationFile, 0, t.cfg.MaxMigrationsInPool)
	for i := 1; i <= t.cfg.MaxMigrationsInPool; i++ {
		tableName := fmt.Sprintf("testonly_table_%02d", i)
		upSQL := fmt.Sprintf("CREATE TABLE %s (id BIGSERIAL PRIMARY KEY, name VARCHAR(255))", tableName)
		downSQL := fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)

		files = append(files, MigrationFile{
			Version:   int64(i),
			Filename:  fmt.Sprintf("20260101120000%02d_create_table_%02d.sql", i, i),
			UpSQL:     upSQL,
			DownSQL:   downSQL,
			IsApplied: false,
		})
	}
	return files
}

// getFullyAppliedState returns a state where all migrations are applied.
func (t *MigrationTester) getFullyAppliedState() MigrationSUTState {
	files := t.getInitialPoolFiles()
	applied := make([]MigrationRecord, 0, len(files))
	for _, file := range files {
		applied = append(applied, MigrationRecord{
			Version:  file.Version,
			Filename: file.Filename,
			UpSQL:    file.UpSQL,
			DownSQL:  file.DownSQL,
			Applied:  true,
		})
	}

	tables := make(map[string]bool)
	for i := 1; i <= t.cfg.MaxMigrationsInPool; i++ {
		tables[fmt.Sprintf("testonly_table_%02d", i)] = true
	}

	return MigrationSUTState{
		Applied:        applied,
		FilesInDir:     files,
		Tables:         tables,
		CurrentVersion: int64(len(files)),
	}
}
