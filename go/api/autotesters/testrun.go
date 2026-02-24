package autotesters

import (
	"time"
)

// RunConfig holds the configuration for a test run.
type RunConfig struct {
	// Purposes filters testers by Purpose() matching any of these.
	Purposes []string

	// Types filters testers by Type() matching any of these.
	Types []string

	// Tags filters testers by Tags() containing any of these.
	Tags []string

	// TesterNames filters to run only these specific Testers by Name().
	TesterNames []string

	// TestIDs filters to run only these specific TestCase IDs.
	TestIDs []string

	// Seed is the randomness seed; 0 = auto-generate and log.
	Seed int64

	// Parallel enables goroutine-per-Tester execution.
	Parallel bool

	// MaxParallel caps concurrent goroutines (default: 4).
	MaxParallel int

	// RetryCount is the default retry count for failed cases (default: 0).
	RetryCount int

	// CaseTimeout is the per-test-case timeout (default: 30s).
	CaseTimeout time.Duration

	// RunTimeout is the overall run timeout (default: 30m).
	RunTimeout time.Duration

	// StopOnFail aborts the run on first StatusFail.
	StopOnFail bool

	// SkipCleanup skips Tester.Cleanup (for post-mortem debugging).
	SkipCleanup bool

	// Verbose emits DEBUG-level logs to stdout.
	Verbose bool

	// JSONReport writes JSON summary to this file path if non-empty.
	JSONReport string

	// Environment is "local", "test", "staging" (default: "local").
	Environment string
}

// TestRun represents a single execution of the AutoTester.
type TestRun struct {
	// ID is the globally unique run identifier (UUID).
	ID string

	// StartedAt is when the run began.
	StartedAt time.Time

	// EndedAt is when the run completed (zero while running).
	EndedAt time.Time

	// Status is "running", "completed", "failed", or "partial".
	Status string

	// Environment is the environment name.
	Environment string

	// Seed is the random seed used for this run.
	Seed int64

	// Config holds the run configuration.
	Config *RunConfig

	// EnvMetadata holds environment details (Go version, DB version, hostname, etc.).
	EnvMetadata map[string]string

	// Counters
	Total   int
	Passed  int
	Failed  int
	Skipped int
	Errored int

	// Duration is the total run duration in milliseconds.
	DurationMs int64

	// ReportPath is the path to the JSON report if written.
	ReportPath string
}

// RunSummary is the final summary of a test run.
type RunSummary struct {
	// RunID is the unique identifier of the run.
	RunID string

	// Seed is the random seed used.
	Seed int64

	// Environment is the environment name.
	Environment string

	// StartedAt is when the run began.
	StartedAt time.Time

	// EndedAt is when the run completed.
	EndedAt time.Time

	// Duration is the total run duration.
	Duration time.Duration

	// Counters
	Total   int
	Passed  int
	Failed  int
	Skipped int
	Errored int

	// Failures contains details of failed and errored test cases.
	Failures []TestResult
}

// PassRate returns the percentage of passed tests (excluding skipped).
func (s *RunSummary) PassRate() float64 {
	executed := s.Total - s.Skipped
	if executed == 0 {
		return 0
	}
	return float64(s.Passed) / float64(executed) * 100
}
