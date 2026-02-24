package autotesters

import (
	"context"
	"fmt"
	"math/rand"
	"time"
)

// Tester is the contract every automated tester must implement.
type Tester interface {
	// Identity / metadata
	Name() string        // unique machine name, e.g. "tester_database"
	Description() string // human-readable summary
	Purpose() string     // e.g. "validation", "regression", "smoke", "load"
	Type() string        // e.g. "unit", "integration", "e2e"
	Tags() []string      // optional labels, e.g. ["database","critical"]

	// Lifecycle
	Prepare(ctx context.Context) error // set up SUT, fixtures, connections
	Cleanup(ctx context.Context) error // tear down, roll back, close connections

	// Test case supply (implement one or both)
	GenerateTestCases(ctx context.Context) ([]TestCase, error)
	// GenerateTestCases returns dynamically created cases.
	// Return nil (not error) to fall through to GetTestCases.

	GetTestCases() []TestCase
	// GetTestCases returns hard-coded static cases.
	// Return nil if the tester relies entirely on GenerateTestCases.

	// Execution
	RunTestCase(ctx context.Context, tc TestCase) TestResult
	// RunTestCase executes exactly one test case and returns the raw result.
	// The runner handles timing, retry, and logging wrappers.

	// Statistics (for BaseTester embedders)
	IncrementSuccess()
	IncrementFail()
	IncrementError()
	SetEndTime(t time.Time)
}

// BaseTester provides default no-op implementations of every interface method
// except Name, RunTestCase, and the case supply methods. Embed it to reduce boilerplate.
type BaseTester struct {
	name         string
	description  string
	purpose      string
	testType     string
	tags         []string
	rand         *rand.Rand
	startTime    time.Time
	endTime      time.Time
	successCount int
	failCount    int
	errorCount   int
}

// NewBaseTester creates a new BaseTester with the given metadata.
func NewBaseTester(name, description, purpose, testType string, tags []string) BaseTester {
	return BaseTester{
		name:         name,
		description:  description,
		purpose:      purpose,
		testType:     testType,
		tags:         tags,
		startTime:    time.Now(),
		successCount: 0,
		failCount:    0,
		errorCount:   0,
	}
}

// Name returns the tester's machine name.
func (b *BaseTester) Name() string {
	return b.name
}

// Description returns the human-readable summary.
func (b *BaseTester) Description() string {
	return b.description
}

// Purpose returns the test purpose.
func (b *BaseTester) Purpose() string {
	return b.purpose
}

// Type returns the test type.
func (b *BaseTester) Type() string {
	return b.testType
}

// Tags returns the tester's tags.
func (b *BaseTester) Tags() []string {
	return b.tags
}

// Prepare is a no-op by default. Override to set up the SUT.
func (b *BaseTester) Prepare(ctx context.Context) error {
	return nil
}

// Cleanup is a no-op by default. Override to tear down the SUT.
func (b *BaseTester) Cleanup(ctx context.Context) error {
	return nil
}

// GenerateTestCases returns nil by default. Override to generate dynamic cases.
func (b *BaseTester) GenerateTestCases(ctx context.Context) ([]TestCase, error) {
	return nil, nil // signal: use GetTestCases
}

// GetTestCases returns nil by default. Override to provide static cases.
func (b *BaseTester) GetTestCases() []TestCase {
	return nil
}

// SetRand sets the seeded random source for dynamic case generation.
// Called by the runner before GenerateTestCases.
func (b *BaseTester) SetRand(r *rand.Rand) {
	b.rand = r
}

// Rand returns the seeded random source. Use this for deterministic random generation.
func (b *BaseTester) GetRandFunc() *rand.Rand {
	return b.rand
}

// IncrementSuccess increments the success count.
func (b *BaseTester) IncrementSuccess() {
	b.successCount++
}

// IncrementFail increments the fail count.
func (b *BaseTester) IncrementFail() {
	b.failCount++
}

// IncrementError increments the error count.
func (b *BaseTester) IncrementError() {
	b.errorCount++
}

// SetEndTime sets the end time of the tester.
func (b *BaseTester) SetEndTime(t time.Time) {
	b.endTime = t
}

// SuccessCount returns the success count.
func (b *BaseTester) SuccessCount() int {
	return b.successCount
}

// FailCount returns the fail count.
func (b *BaseTester) FailCount() int {
	return b.failCount
}

// ErrorCount returns the error count.
func (b *BaseTester) ErrorCount() int {
	return b.errorCount
}

// StartTime returns the start time.
func (b *BaseTester) StartTime() time.Time {
	return b.startTime
}

// EndTime returns the end time.
func (b *BaseTester) EndTime() time.Time {
	return b.endTime
}

// RunTestCase executes a test case using its runTest function.
// This default implementation handles panic recovery, timing, and status management.
// Embedding testers only need to assign runTest functions to their test cases.
//
// The test case's RunTest function is responsible for:
// - Setting result.Status to StatusPass, StatusFail, or StatusError
// - Appending error messages to result.ErrorMsgs when the test fails
// - Setting result.Message for human-readable explanations
//
// RunTestCase does not determine pass/fail - that is the responsibility of each test case.
func (b *BaseTester) RunTestCase(ctx context.Context, tc TestCase) TestResult {
	start := time.Now()
	result := TestResult{
		TestCaseID: tc.ID,
		StartTime:  start,
	}

	// Guard against panics
	defer func() {
		if r := recover(); r != nil {
			result.Status = StatusError
			result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("panic: %v", r))
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(result.StartTime)
		}
	}()

	// Execute the test case using its runTest function
	if tc.RunTest == nil {
		result.Status = StatusError
		result.ErrorMsgs = append(result.ErrorMsgs, fmt.Sprintf("runTest not defined for test case:%s", tc.ID))
	} else {
		tc.RunTest(ctx, tc, &result)
		if result.Status == "" {
			result.Status = StatusPass
			result.Message = "test passed"
		}
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	return result
}
