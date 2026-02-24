// Package tester_migration provides automated testing for the goose database migration system.

package tester_migration

import (
	"context"
	"fmt"
	"strings"

	"github.com/chendingplano/shared/go/api/autotesters"
	sharedgoose "github.com/chendingplano/shared/go/api/goose"
)

// handleUp executes the Up operation (apply all pending migrations).
func (t *MigrationTester) handleUp(ctx context.Context, migrator *sharedgoose.Migrator, result *autotesters.TestResult) {
	err := migrator.Up(ctx)
	if err != nil {
		result.Status = autotesters.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("Up failed (MID_260224100050): %v", err))
		return
	}

	result.Status = autotesters.StatusPass
	result.Message = "All pending migrations applied successfully"
}

// handleUpByOne executes the UpByOne operation (apply exactly one pending migration).
func (t *MigrationTester) handleUpByOne(ctx context.Context, migrator *sharedgoose.Migrator, result *autotesters.TestResult) {
	err := migrator.UpByOne(ctx)
	if err != nil {
		// Check for expected "no next version" error
		if strings.Contains(err.Error(), "no more pending migrations") ||
			strings.Contains(err.Error(), "ErrNoNextVersion") {
			result.Status = autotesters.StatusFail
			result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("UpByOne: no pending migrations (MID_260224100051): %v", err))
			return
		}
		result.Status = autotesters.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("UpByOne failed (MID_260224100052): %v", err))
		return
	}

	result.Status = autotesters.StatusPass
	result.Message = "One migration applied successfully"
}

// handleUpTo executes the UpTo operation (apply up to a target version).
func (t *MigrationTester) handleUpTo(ctx context.Context, migrator *sharedgoose.Migrator, targetVersion int64, result *autotesters.TestResult) {
	err := migrator.UpTo(ctx, targetVersion)
	if err != nil {
		result.Status = autotesters.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("UpTo(%d) failed (MID_260224100053): %v", targetVersion, err))
		return
	}

	result.Status = autotesters.StatusPass
	result.Message = fmt.Sprintf("Migrations applied up to version %d", targetVersion)
	result.ActualValue = targetVersion
}

// handleDown executes the Down operation (rollback one migration).
func (t *MigrationTester) handleDown(ctx context.Context, migrator *sharedgoose.Migrator, result *autotesters.TestResult) {
	err := migrator.Down(ctx)
	if err != nil {
		result.Status = autotesters.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("Down failed (MID_260224100054): %v", err))
		return
	}

	result.Status = autotesters.StatusPass
	result.Message = "One migration rolled back successfully"
}

// handleDownTo executes the DownTo operation (rollback to a target version).
func (t *MigrationTester) handleDownTo(ctx context.Context, migrator *sharedgoose.Migrator, targetVersion int64, result *autotesters.TestResult) {
	err := migrator.DownTo(ctx, targetVersion)
	if err != nil {
		result.Status = autotesters.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("DownTo(%d) failed (MID_260224100055): %v", targetVersion, err))
		return
	}

	result.Status = autotesters.StatusPass
	result.Message = fmt.Sprintf("Migrations rolled back to version %d", targetVersion)
	result.ActualValue = targetVersion
}

// handleStatus executes the Status operation (get applied/pending status).
func (t *MigrationTester) handleStatus(ctx context.Context, migrator *sharedgoose.Migrator, result *autotesters.TestResult) {
	statuses, err := migrator.Status(ctx)
	if err != nil {
		result.Status = autotesters.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("Status failed (MID_260224100056): %v", err))
		return
	}

	result.Status = autotesters.StatusPass
	result.Message = "Status retrieved successfully"
	result.ActualValue = statuses
}

// handleGetVersion executes the GetVersion operation (get current version).
func (t *MigrationTester) handleGetVersion(ctx context.Context, migrator *sharedgoose.Migrator, result *autotesters.TestResult) {
	version, err := migrator.GetVersion(ctx)
	if err != nil {
		result.Status = autotesters.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("GetVersion failed (MID_260224100057): %v", err))
		return
	}

	result.Status = autotesters.StatusPass
	result.Message = "Version retrieved successfully"
	result.ActualValue = version
}

// handleHasPending executes the HasPending operation (check for pending migrations).
func (t *MigrationTester) handleHasPending(ctx context.Context, migrator *sharedgoose.Migrator, result *autotesters.TestResult) {
	hasPending, err := migrator.HasPending(ctx)
	if err != nil {
		result.Status = autotesters.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("HasPending failed (MID_260224100058): %v", err))
		return
	}

	result.Status = autotesters.StatusPass
	result.Message = "HasPending check completed"
	result.ActualValue = hasPending
}

// handleCreateAndApply executes the CreateAndApply operation (create file and apply).
func (t *MigrationTester) handleCreateAndApply(
	ctx context.Context,
	migrator *sharedgoose.Migrator,
	description string,
	upSQL string,
	downSQL string,
	result *autotesters.TestResult) {

	filename, err := migrator.CreateAndApply(ctx, description, upSQL, downSQL)
	if err != nil {
		result.Status = autotesters.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("CreateAndApply failed (MID_260224100059): %v", err))
		return
	}

	result.Status = autotesters.StatusPass
	result.Message = fmt.Sprintf("Migration created and applied: %s", filename)
	result.SideEffectsObserved = append(result.SideEffectsObserved, string(SideEffectMigrationFileWritten))
}

// handleListSources executes the ListSources operation (list all migration sources).
func (t *MigrationTester) handleListSources(migrator *sharedgoose.Migrator, result *autotesters.TestResult) {
	sources := migrator.ListSources()

	result.Status = autotesters.StatusPass
	result.Message = "Migration sources listed"
	result.ActualValue = sources
}
