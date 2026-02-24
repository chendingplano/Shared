package autotesters

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/google/uuid"
)

// TestRunner orchestrates the execution of multiple Testers.
type TestRunner struct {
	testers   []Tester
	config    *RunConfig
	runID     string
	seed      int64
	startTime time.Time
	logger    ApiTypes.JimoLogger
	db        *DBPersistence

	mu      sync.Mutex
	summary RunSummary
	passed  map[string]bool // test_case_id -> pass; used for dependency checks

	// Table names
	runsTable    string
	resultsTable string
	logsTable    string
}

// NewTestRunner creates a new TestRunner instance.
func NewTestRunner(testers []Tester, config *RunConfig, logger ApiTypes.JimoLogger) *TestRunner {
	// Set defaults
	if config.MaxParallel <= 0 {
		config.MaxParallel = 4
	}
	if config.CaseTimeout <= 0 {
		config.CaseTimeout = 30 * time.Second
	}
	if config.RunTimeout <= 0 {
		config.RunTimeout = 30 * time.Minute
	}
	if config.Environment == "" {
		config.Environment = "local"
	}

	return &TestRunner{
		testers: testers,
		config:  config,
		logger:  logger,
		passed:  make(map[string]bool),
		summary: RunSummary{
			Failures: make([]TestResult, 0),
		},

		// Default table names
		runsTable:    "auto_test_runs",
		resultsTable: "auto_test_results",
		logsTable:    "auto_test_logs",
	}
}

// SetDBPersistence sets the database persistence layer.
func (r *TestRunner) SetDBPersistence(db *DBPersistence) {
	r.db = db
}

// SetTableNames sets custom table names for the auto-test tables.
func (r *TestRunner) SetTableNames(runs, results, logs string) {
	r.runsTable = runs
	r.resultsTable = results
	r.logsTable = logs
}

// Run executes all registered testers and returns the summary.
func (r *TestRunner) Run(ctx context.Context) error {
	r.runID = newRunID()
	r.seed = r.resolveSeed()
	r.startTime = time.Now()

	r.logger.Info("AutoTester run started",
		"run_id", r.runID,
		"seed", r.seed,
		"env", r.config.Environment,
	)

	// Create run record in database if DB is available
	if r.db != nil {
		if err := r.createRunRecord(ctx); err != nil {
			return fmt.Errorf("create run record (MID_060221143044): %w", err)
		}
	}

	// Apply overall timeout
	runCtx, cancel := context.WithTimeout(ctx, r.config.RunTimeout)
	defer cancel()

	// Execute testers
	if r.config.Parallel {
		r.executeParallelTesters(runCtx)
	} else {
		r.executeSequentialTesters(runCtx)
	}

	// Finalize
	r.finalizeRunRecord(ctx)
	r.printSummary()
	if err := r.writeJSONReport(); err != nil {
		r.logger.Warn("Failed to write JSON report", "error", err)
	}

	return nil
}

// Summary returns the final run summary.
func (r *TestRunner) Summary() RunSummary {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.summary
}

// resolveSeed determines the random seed to use.
func (r *TestRunner) resolveSeed() int64 {
	if r.config.Seed != 0 {
		return r.config.Seed
	}
	seed := time.Now().UnixNano()
	r.logger.Info("Auto-generated random seed", "seed", seed)
	return seed
}

// newRunID generates a new UUID v4 for the run.
func newRunID() string {
	return uuid.New().String()
}

// setTesterRand sets the random source for testers that embed BaseTester.
// This uses reflection to safely set the rand field if present.
func setTesterRand(tester Tester, r *rand.Rand) {
	// Try to set rand on BaseTester if embedded
	if bt, ok := any(tester).(interface{ SetRand(*rand.Rand) }); ok {
		bt.SetRand(r)
	}
}

// createRunRecord persists the initial run record to the database.
func (r *TestRunner) createRunRecord(ctx context.Context) error {
	run := &TestRun{
		ID:          r.runID,
		StartedAt:   r.startTime,
		Status:      "running",
		Environment: r.config.Environment,
		Seed:        r.seed,
		Config:      r.config,
		EnvMetadata: r.collectEnvMetadata(),
	}

	return r.db.CreateRunRecord(ctx, run, r.runsTable)
}

// collectEnvMetadata gathers information about the test environment.
func (r *TestRunner) collectEnvMetadata() map[string]string {
	return map[string]string{
		"go_version": runtime.Version(),
		"go_os":      runtime.GOOS,
		"go_arch":    runtime.GOARCH,
		"num_cpu":    fmt.Sprintf("%d", runtime.NumCPU()),
		"hostname":   getHostname(),
		"timestamp":  time.Now().Format(time.RFC3339),
	}
}

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

// executeSequentialTesters runs testers one after another.
func (r *TestRunner) executeSequentialTesters(ctx context.Context) {
	for _, tester := range r.testers {
		if ctx.Err() != nil {
			return
		}
		if !r.testerMatches(tester) {
			continue
		}
		r.executeTester(ctx, tester)
	}
}

// executeParallelTesters runs testers concurrently with a limit.
func (r *TestRunner) executeParallelTesters(ctx context.Context) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, r.config.MaxParallel)

	for _, tester := range r.testers {
		if ctx.Err() != nil {
			break
		}
		if !r.testerMatches(tester) {
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(t Tester) {
			defer wg.Done()
			defer func() { <-sem }()
			r.executeTester(ctx, t)
		}(tester)
	}

	wg.Wait()
}

// testerMatches checks if a tester matches the configured filters.
func (r *TestRunner) testerMatches(tester Tester) bool {
	// Filter by tester names
	if len(r.config.TesterNames) > 0 {
		matched := false
		for _, name := range r.config.TesterNames {
			if tester.Name() == name {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Filter by purpose
	if len(r.config.Purposes) > 0 {
		matched := false
		for _, purpose := range r.config.Purposes {
			if tester.Purpose() == purpose {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Filter by type
	if len(r.config.Types) > 0 {
		matched := false
		for _, t := range r.config.Types {
			if tester.Type() == t {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Filter by tags
	if len(r.config.Tags) > 0 {
		matched := false
		testerTags := tester.Tags()
		for _, tag := range r.config.Tags {
			for _, testerTag := range testerTags {
				if tag == testerTag {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			return false
		}
	}

	return true
}

// executeTester runs a single tester through its full lifecycle.
func (r *TestRunner) executeTester(ctx context.Context, tester Tester) {
	startTime := time.Now()
	r.logger.Line("")
	r.logger.Line("========================================")

	// Set up random source for this tester if it embeds BaseTester
	setTesterRand(tester, rand.New(rand.NewSource(r.seed)))

	// Prepare
	if err := tester.Prepare(ctx); err != nil {
		r.logger.Error("Tester prepare failed", "tester", tester.Name(), "error", err)
		r.recordTesterError(tester.Name(), err)
		return
	}

	// Cleanup at the end unless skipped
	if !r.config.SkipCleanup {
		defer func() {
			if err := tester.Cleanup(ctx); err != nil {
				r.logger.Warn("Tester cleanup failed", "tester", tester.Name(), "error", err)
			}
		}()
	}

	// Get test cases
	cases, err := r.collectTestCases(ctx, tester)
	if err != nil {
		r.logger.Error("Failed to collect test cases", "tester", tester.Name(), "error", err)
		return
	}

	// Filter test cases
	cases = r.filterTestCases(cases)

	// r.logger.Info("===== Collected test cases", "tester", tester.Name(), "count", len(cases))

	// Run test cases
	errorMsgs := make([]string, 0)
	for _, tc := range cases {
		if ctx.Err() != nil {
			return
		}
		errorMsgs = append(errorMsgs, r.runTestCase(ctx, tester, tc)...)

		// Check stop-on-fail
		if r.config.StopOnFail {
			r.mu.Lock()
			hasFailures := r.summary.Failed > 0 || r.summary.Errored > 0
			r.mu.Unlock()
			if hasFailures {
				return
			}
		}
	}
	r.logger.Line(fmt.Sprintf("test case:%s", tester.Name()))
	r.logger.Line(fmt.Sprintf("total:%d", len(cases)))
	r.logger.Line(fmt.Sprintf("passed:%d", r.summary.Passed))
	r.logger.Line(fmt.Sprintf("failed:%d", r.summary.Failed))
	r.logger.Line(fmt.Sprintf("errored:%d", r.summary.Errored))
	r.logger.Line(fmt.Sprintf("time:%.4f(s)", time.Since(startTime).Seconds()))

	if len(errorMsgs) > 0 {
		for _, msg := range errorMsgs {
			r.logger.Line(fmt.Sprintf("Error:%s", msg))
		}
	}
	r.logger.Line("========================================")

	// Set end time for the tester
	tester.SetEndTime(time.Now())
}

// collectTestCases gets cases from both GenerateTestCases and GetTestCases.
func (r *TestRunner) collectTestCases(ctx context.Context, tester Tester) ([]TestCase, error) {
	var allCases []TestCase

	// Try GenerateTestCases first
	genCases, err := tester.GenerateTestCases(ctx)
	if err != nil {
		return nil, fmt.Errorf("generate test cases (MID_060221143040): %w", err)
	}
	if genCases != nil {
		allCases = append(allCases, genCases...)
	}

	// Fall back to GetTestCases
	staticCases := tester.GetTestCases()
	if staticCases != nil {
		allCases = append(allCases, staticCases...)
	}

	return allCases, nil
}

// filterTestCases applies case-level filters.
func (r *TestRunner) filterTestCases(cases []TestCase) []TestCase {
	if len(r.config.TestIDs) == 0 && len(r.config.Purposes) == 0 &&
		len(r.config.Types) == 0 && len(r.config.Tags) == 0 {
		return cases
	}

	// If test IDs are specified, only those IDs run (ignores other filters)
	if len(r.config.TestIDs) > 0 {
		idSet := make(map[string]bool)
		for _, id := range r.config.TestIDs {
			idSet[id] = true
		}

		filtered := make([]TestCase, 0)
		for _, tc := range cases {
			if idSet[tc.ID] {
				filtered = append(filtered, tc)
			}
		}
		return filtered
	}

	// Apply other filters
	filtered := make([]TestCase, 0, len(cases))
	for _, tc := range cases {
		if r.testCaseMatches(tc) {
			filtered = append(filtered, tc)
		}
	}
	return filtered
}

// testCaseMatches checks if a test case matches the configured filters.
func (r *TestRunner) testCaseMatches(tc TestCase) bool {
	// Filter by purpose
	if len(r.config.Purposes) > 0 {
		purpose := tc.Purpose
		if purpose == "" {
			// Fall back to tester purpose if not set
			// (caller should handle this)
		}
		matched := false
		for _, p := range r.config.Purposes {
			if purpose == p {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Filter by type
	if len(r.config.Types) > 0 {
		typ := tc.Type
		matched := false
		for _, t := range r.config.Types {
			if typ == t {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Filter by tags
	if len(r.config.Tags) > 0 {
		matched := false
		for _, tag := range r.config.Tags {
			for _, tcTag := range tc.Tags {
				if tag == tcTag {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			return false
		}
	}

	return true
}

// runTestCase executes a single test case with retry logic.
func (r *TestRunner) runTestCase(
	ctx context.Context,
	tester Tester,
	tc TestCase) []string {
	var result TestResult
	errorMsgs := make([]string, 0)

	// Check dependencies
	if !r.dependenciesSatisfied(tc.Dependencies) {
		r.recordSkippedCase(tc, "dependency not met (MID_260221143040)")
		errorMsgs = append(errorMsgs, fmt.Sprintf("(MID_260221143041) dependencies not satisfied: %v", tc.Dependencies))
		return errorMsgs
	}

	// Check skip reason
	if tc.SkipReason != "" {
		r.recordSkippedCase(tc, tc.SkipReason)
		errorMsgs = append(errorMsgs, fmt.Sprintf("(MID_260221143042) skipped: %s", tc.SkipReason))
		return errorMsgs
	}

	// Determine retry count and timeout
	retryCount := r.config.RetryCount
	if tc.RetryCount > 0 {
		retryCount = tc.RetryCount
	}

	timeout := r.config.CaseTimeout
	if tc.Timeout > 0 {
		timeout = tc.Timeout
	}

	// Execute with retries
	var attempt int

	for attempt = 0; attempt <= retryCount; attempt++ {
		if attempt > 0 {
			r.logger.Debug("Retrying test case", "test_case", tc.ID, "attempt", attempt+1)
		}

		// Create context with timeout
		caseCtx, cancel := context.WithTimeout(ctx, timeout)
		result = tester.RunTestCase(caseCtx, tc)
		cancel()

		result.RunID = r.runID
		result.TesterName = tester.Name()
		result.RetryCount = attempt

		// Verify the result
		result = r.verifyResult(tc, result)
		if len(result.ErrorMsgs) > 0 {
			errorMsgs = append(errorMsgs, result.ErrorMsgs...)
		}

		// If passed or not a retryable error, stop retrying
		if result.Status == StatusPass || result.Status == StatusSkip {
			break
		}
		if result.Status == StatusError {
			// Infrastructure errors might be retryable
			continue
		}
		// StatusFail - might be flaky, retry
	}

	r.recordResult(result)
	return errorMsgs
}

// dependenciesSatisfied checks if all dependencies have passed.
func (r *TestRunner) dependenciesSatisfied(deps []string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, dep := range deps {
		if !r.passed[dep] {
			return false
		}
	}
	return true
}

// verifyResult applies assertions to determine pass/fail status.
func (r *TestRunner) verifyResult(tc TestCase, result TestResult) TestResult {
	// Skip check
	if tc.SkipReason != "" {
		result.Status = StatusSkip
		result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("tc skipped (MID_260221143042): %s", tc.SkipReason))
		return result
	}

	// Dependency check already handled before execution

	// Check for execution errors first
	if result.Status == StatusError {
		return result
	}

	// Success/error expectation
	expected := tc.Expected
	if !expected.Success && len(result.ErrorMsgs) == 0 {
		result.Status = StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, "expected an error but got success (MID_260221143043)")
		return result
	}

	// Expected error content
	if expected.ExpectedError != "" {
		errorStr := strings.Join(result.ErrorMsgs, "; ")
		pass, msg := AssertErrorContains(fmt.Errorf("expected error (MID_060221143041): %s", errorStr), expected.ExpectedError)
		if !pass {
			result.Status = StatusFail
			result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("expected error content mismatch (MID_260221143044): %s", msg))
			return result
		}
	}

	// Value equality
	if expected.ExpectedValue != nil {
		pass, msg := AssertEqual(expected.ExpectedValue, result.ActualValue)
		if !pass {
			result.Status = StatusFail
			result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("value mismatch (MID_260221143046): %s", msg))
			return result
		}
	}

	// Duration constraint
	if expected.MaxDuration > 0 && result.Duration > expected.MaxDuration {
		result.Status = StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("exceeded max duration (MID_260221143045): %v > %v", result.Duration, expected.MaxDuration))
		return result
	}

	// Side effects
	for _, expectedEffect := range expected.SideEffects {
		found := false
		for _, observedEffect := range result.SideEffectsObserved {
			if expectedEffect == observedEffect {
				found = true
				break
			}
		}
		if !found {
			result.Status = StatusFail
			result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("expected side effect %q not observed (MID_260221143047)", expectedEffect))
			return result
		}
	}

	// Custom validator
	if expected.CustomValidator != nil {
		pass, reason := expected.CustomValidator(result.ActualValue, expected)
		if !pass {
			result.Status = StatusFail
			result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("custom validation failed (MID_260221143048): %s", reason))
			return result
		}
	}

	// All checks passed
	result.Status = StatusPass
	result.Message = "test passed"
	return result
}

// recordResult stores a test result and updates counters.
func (r *TestRunner) recordResult(result TestResult) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Update summary
	r.summary.Total++
	switch result.Status {
	case StatusPass:
		r.summary.Passed++
		r.passed[result.TestCaseID] = true
	case StatusFail:
		r.summary.Failed++
		r.summary.Failures = append(r.summary.Failures, result)
	case StatusSkip:
		r.summary.Skipped++
	case StatusError:
		r.summary.Errored++
		r.summary.Failures = append(r.summary.Failures, result)
	}

	// Update tester statistics
	for _, tester := range r.testers {
		if tester.Name() == result.TesterName {
			switch result.Status {
			case StatusPass:
				tester.IncrementSuccess()
			case StatusFail:
				tester.IncrementFail()
			case StatusError:
				tester.IncrementError()
			}
			break
		}
	}

	// Persist to database
	if r.db != nil {
		ctx := context.Background()
		if err := r.db.InsertTestResult(ctx, &result, r.resultsTable); err != nil {
			r.logger.Error("Failed to persist test result", "test_case", result.TestCaseID, "error", err)
		}
		if err := r.db.InsertTestLogs(ctx, &result, r.logsTable); err != nil {
			r.logger.Error("Failed to persist test logs", "test_case", result.TestCaseID, "error", err)
		}
	}
}

// recordSkippedCase records a skipped test case.
func (r *TestRunner) recordSkippedCase(tc TestCase, reason string) {
	result := TestResult{
		RunID:      r.runID,
		TestCaseID: tc.ID,
		TesterName: "", // Will be set by caller
		Status:     StatusSkip,
		Message:    reason,
		StartTime:  time.Now(),
		EndTime:    time.Now(),
		Duration:   0,
	}

	r.recordResult(result)
}

// recordTesterError records an error at the tester level.
func (r *TestRunner) recordTesterError(testerName string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.summary.Errored++
	r.logger.Error("Tester error", "tester", testerName, "error", err)

	// Set end time for the tester
	for _, tester := range r.testers {
		if tester.Name() == testerName {
			tester.SetEndTime(time.Now())
			break
		}
	}
}

// finalizeRunRecord updates the run record with final statistics.
func (r *TestRunner) finalizeRunRecord(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.summary.EndedAt = time.Now()
	r.summary.Duration = r.summary.EndedAt.Sub(r.startTime)

	// Determine final status
	status := "completed"
	if r.summary.Failed > 0 || r.summary.Errored > 0 {
		status = "failed"
	} else if r.summary.Skipped > 0 && r.summary.Passed == 0 {
		status = "partial"
	}

	run := &TestRun{
		ID:          r.runID,
		StartedAt:   r.startTime,
		EndedAt:     r.summary.EndedAt,
		Status:      status,
		Environment: r.config.Environment,
		Seed:        r.seed,
		Total:       r.summary.Total,
		Passed:      r.summary.Passed,
		Failed:      r.summary.Failed,
		Skipped:     r.summary.Skipped,
		Errored:     r.summary.Errored,
		DurationMs:  r.summary.Duration.Milliseconds(),
	}

	if r.db != nil {
		if err := r.db.UpdateRunRecord(ctx, run, r.runsTable); err != nil {
			r.logger.Error("Failed to update run record", "error", err)
		}
	}
}

// printSummary outputs the run summary to stdout.
func (r *TestRunner) printSummary() {
	r.mu.Lock()
	defer r.mu.Unlock()

	passRate := r.summary.PassRate()

	r.logger.Info("AutoTester Run Complete",
		"run_id", r.runID,
		"seed", r.seed,
		"env", r.config.Environment,
		"duration", r.summary.Duration.String(),
		"total", r.summary.Total,
		"passed", r.summary.Passed,
		"failed", r.summary.Failed,
		"skipped", r.summary.Skipped,
		"errored", r.summary.Errored,
		"pass_rate", fmt.Sprintf("%.1f%%", passRate),
	)

	if len(r.summary.Failures) > 0 {
		r.logger.Info("FAILURES:")
		for _, f := range r.summary.Failures {
			errorStr := strings.Join(f.ErrorMsgs, "; ")
			r.logger.Line(fmt.Sprintf("  [%s] %s (%v) error:[%s]",
				f.Status, f.TestCaseID, f.Duration, errorStr))
		}
	}
}

// writeJSONReport writes the run summary to a JSON file.
func (r *TestRunner) writeJSONReport() error {
	if r.config.JSONReport == "" {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := json.MarshalIndent(r.summary, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal summary (MID_060221143042): %w", err)
	}

	if err := os.WriteFile(r.config.JSONReport, data, 0644); err != nil {
		return fmt.Errorf("write report (MID_060221143043): %w", err)
	}

	r.logger.Info("JSON report written", "path", r.config.JSONReport)
	return nil
}
