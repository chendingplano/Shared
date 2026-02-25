package autotester

import (
	"context"
	"time"
)

// Priority represents the importance level of a test case.
type Priority int

const (
	PriorityCritical Priority = iota // Must pass for deployment
	PriorityHigh                     // Core functionality
	PriorityMedium                   // Standard coverage
	PriorityLow                      // Additional coverage
)

// Status represents the outcome of a test case execution.
type Status string

const (
	StatusPass  Status = "pass"
	StatusFail  Status = "fail"
	StatusSkip  Status = "skip"
	StatusError Status = "error" // infrastructure/panic, not assertion failure
)

// ExpectedResult defines what a test case expects from the SUT.
type ExpectedResult struct {
	// Success indicates whether the operation should succeed (true = no error expected).
	Success bool

	// ExpectedError is a substring that must appear in the error message if Success=false.
	ExpectedError string

	// ExpectedValue is the exact value to compare with ActualValue using DeepEqual.
	ExpectedValue interface{}

	// MaxDuration fails the test if execution exceeds this duration.
	MaxDuration time.Duration

	// SideEffects lists side effect keys that must appear in SideEffectsObserved.
	SideEffects []string

	// CustomValidator is an optional function for custom assertion logic.
	// It receives the actual value and expected result, returns pass status and reason.
	CustomValidator func(actual interface{}, expected ExpectedResult) (pass bool, reason string)
}

// TestCase represents a single test scenario.
type TestCase struct {
	// ID is a unique identifier: "<module>.<feature>.<variant>"
	ID string

	// Name is a human-readable name for the test case.
	Name string

	// Description explains what this case validates and why.
	Description string

	// Purpose indicates the test type: "smoke", "regression", "load", "fuzz", "compliance".
	Purpose string

	// Type indicates the testing level: "unit", "integration", "e2e".
	Type string

	// Tags are free-form labels for filtering.
	Tags []string

	// Input is any serializable input value passed to the SUT.
	Input interface{}

	// Expected defines the expected outcome.
	Expected ExpectedResult

	// Priority indicates the importance level.
	Priority Priority

	// RetryCount overrides RunConfig.RetryCount if > 0.
	RetryCount int

	// Timeout overrides RunConfig.CaseTimeout if > 0.
	Timeout time.Duration

	// Dependencies lists TestCase IDs that must have StatusPass before this case runs.
	Dependencies []string

	// SkipReason: if non-empty, skip this case with this reason.
	SkipReason string

	// runTest is the test execution function for this case.
	// It receives the test case, context and result pointer.
	// The function is responsible for:
	// - Setting result.Status to StatusPass, StatusFail, or StatusError
	// - Appending error messages to result.ErrorMsgs when the test fails
	// - Setting result.Message for human-readable explanations
	//
	// The function does NOT return an error - all outcomes are reported via result.
	RunTest func(ctx context.Context, tc TestCase, result *TestResult)
}

// SetRunTest sets the test execution function for this test case.
func (tc *TestCase) SetRunTest(fn func(ctx context.Context, tc TestCase, result *TestResult)) {
	tc.RunTest = fn
}
