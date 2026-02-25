package autotester

import (
	"fmt"
	"time"
)

// LogEntry represents a single structured log line from test execution.
type LogEntry struct {
	// Level is the log level: DEBUG, INFO, WARN, ERROR.
	Level string

	// Message is the log message.
	Message string

	// Context holds additional structured data (will be JSON-encoded).
	Context map[string]interface{}

	// Timestamp is when the log was emitted.
	Timestamp time.Time
}

// TestResult captures what happened when a TestCase was executed.
type TestResult struct {
	// RunID is the unique ID of the test run.
	RunID string

	// TestCaseID is the ID of the test case.
	TestCaseID string

	// TesterName is the name of the Tester that executed this case.
	TesterName string

	// Status is the outcome: pass/fail/skip/error.
	Status Status

	// Message is a human-readable explanation of the outcome.
	Message string

	// ErrorMsgs contains error messages if Status is not pass.
	ErrorMsgs []string

	// StartTime is when execution began.
	StartTime time.Time

	// EndTime is when execution completed.
	EndTime time.Time

	// Duration is the total execution time.
	Duration time.Duration

	// RetryCount is how many retries were actually performed.
	RetryCount int

	// ActualValue is the raw output from the SUT (before any assertion).
	ActualValue interface{}

	// SideEffectsObserved lists side effects that were actually observed.
	SideEffectsObserved []string

	// Logs contains structured log entries emitted during execution.
	Logs []LogEntry
}

// AddLog appends a log entry to the test result.
func (tr *TestResult) AddLog(level, message string, ctx map[string]interface{}) {
	tr.Logs = append(tr.Logs, LogEntry{
		Level:     level,
		Message:   message,
		Context:   ctx,
		Timestamp: time.Now(),
	})
}

// AddLogf appends a formatted log entry to the test result.
func (tr *TestResult) AddLogf(level, format string, args ...interface{}) {
	tr.Logs = append(tr.Logs, LogEntry{
		Level:     level,
		Message:   fmt.Sprintf(format, args...),
		Context:   nil,
		Timestamp: time.Now(),
	})
}
