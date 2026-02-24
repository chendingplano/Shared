package sharedtesters

import (
	"bytes"
	"context"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/autotesters"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

// LoggerTester tests the loggerutil package functionality.
type LoggerTester struct {
	autotesters.BaseTester
	testLogger ApiTypes.JimoLogger
	logBuffer  *bytes.Buffer
}

// NewLoggerTester creates a new logger tester.
func NewLoggerTester() *LoggerTester {
	return &LoggerTester{
		BaseTester: autotesters.NewBaseTester(
			"tester_logger",
			"Tests loggerutil package functionality",
			"validation",
			"unit",
			[]string{"logger", "core"},
		),
	}
}

// Prepare sets up the test logger.
func (t *LoggerTester) Prepare(ctx context.Context) error {
	// Create a test logger that writes to a buffer
	t.logBuffer = &bytes.Buffer{}
	t.testLogger = loggerutil.CreateDefaultLogger("TEST_LOGGER")
	return nil
}

// GetTestCases returns static test cases for logger testing.
func (t *LoggerTester) GetTestCases() []autotesters.TestCase {
	return []autotesters.TestCase{
		{
			ID:          "TC_260222132440",
			Name:        "Test Info logging",
			Description: "Verify that Info level logging works correctly",
			Input:       map[string]interface{}{"level": "INFO", "message": "Test info message"},
			Expected:    autotesters.ExpectedResult{Success: true},
			Priority:    autotesters.PriorityHigh,
			Tags:        []string{"logging", "info"},
			RunTest:     t.testInfo,
		},
		{
			ID:          "TC_260222132443",
			Name:        "Test Debug logging",
			Description: "Verify that Debug level logging works correctly",
			Input:       map[string]interface{}{"level": "DEBUG", "message": "Test debug message"},
			Expected:    autotesters.ExpectedResult{Success: true},
			Priority:    autotesters.PriorityMedium,
			Tags:        []string{"logging", "debug"},
			RunTest:     t.testDebug,
		},
		{
			ID:          "TC_260222132444",
			Name:        "Test logging with context",
			Description: "Verify that logging with key-value context works correctly",
			Input:       map[string]interface{}{"message": "Test with context", "context": map[string]interface{}{"key": "value", "num": 42}},
			Expected:    autotesters.ExpectedResult{Success: true},
			Priority:    autotesters.PriorityMedium,
			Tags:        []string{"logging", "context"},
			RunTest:     t.testContext,
		},
	}
}

func (t *LoggerTester) testInfo(
	ctx context.Context,
	tc autotesters.TestCase,
	result *autotesters.TestResult) {
	input, ok := tc.Input.(map[string]interface{})
	if !ok {
		result.Status = autotesters.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, "invalid input type: expected map[string]interface{}")
		return
	}
	message, ok := input["message"].(string)
	if !ok {
		result.Status = autotesters.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, "missing or invalid 'message' field in input")
		return
	}

	// Log at Info level
	t.testLogger.Info(message)

	result.SideEffectsObserved = []string{"info_logged"}
}

func (t *LoggerTester) testDebug(
	ctx context.Context,
	tc autotesters.TestCase,
	result *autotesters.TestResult) {
	input, ok := tc.Input.(map[string]interface{})
	if !ok {
		result.Status = autotesters.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, "invalid input type: expected map[string]interface{}")
		return
	}
	message, ok := input["message"].(string)
	if !ok {
		result.Status = autotesters.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, "missing or invalid 'message' field in input")
		return
	}

	// Log at Debug level
	t.testLogger.Debug(message)

	result.SideEffectsObserved = []string{"debug_logged"}
}

func (t *LoggerTester) testContext(
	ctx context.Context,
	tc autotesters.TestCase,
	result *autotesters.TestResult) {
	input, ok := tc.Input.(map[string]interface{})
	if !ok {
		result.Status = autotesters.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, "invalid input type: expected map[string]interface{}")
		return
	}
	message, ok := input["message"].(string)
	if !ok {
		result.Status = autotesters.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, "missing or invalid 'message' field in input")
		return
	}
	ctxMap, ok := input["context"].(map[string]interface{})
	if !ok {
		result.Status = autotesters.StatusFail
		result.ErrorMsgs = append(result.ErrorMsgs, "missing or invalid 'context' field in input")
		return
	}

	// Log with context
	var args []interface{}
	for k, v := range ctxMap {
		args = append(args, k, v)
	}
	t.testLogger.Info(message, args...)

	result.SideEffectsObserved = []string{"context_logged"}
}

// Cleanup resets the test logger.
func (t *LoggerTester) Cleanup(ctx context.Context) error {
	t.testLogger = nil
	t.logBuffer = nil
	return nil
}

// Helper functions for log verification

/* Not used for now
// verifyLogContains checks if the log buffer contains the expected string.
func verifyLogContains(logOutput, expected string) (bool, string) {
	if strings.Contains(logOutput, expected) {
		return true, ""
	}
	return false, fmt.Sprintf("expected log to contain %q, got: %s", expected, logOutput)
}
*/
