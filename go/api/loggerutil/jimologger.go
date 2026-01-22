package loggerutil

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const (
	ContextTypeBackground string = "background"
	ContextTypeWithCancel string = "with_cancel"
	ContextTypeTimeout    string = "timeout"
	ContextTypeTODO       string = "todo"
)

const (
	LogHandlerTypeDefault string = "default"
	LogHandlerTypeJSON    string = "json"
)

type JimoLogger struct {
	ctx         context.Context
	cancel      context.CancelFunc
	logger      *slog.Logger
	reqID       string
	trace       string
	currentFile string
	call_depth  int
}

func CreateLogger(
	ctx context.Context,
	loggerType string) *JimoLogger {

	return &JimoLogger{
		ctx:    ctx,
		cancel: nil,
		logger: getLoggerHandler(loggerType),
		reqID:  generateRequestID("e")}
}

func CreateLogger2(
	contextType string,
	loggerType string,
	timeoutSec int) *JimoLogger {
	var ctx context.Context
	var cancelFunc context.CancelFunc
	if timeoutSec < 5 {
		timeoutSec = 5
	}
	switch contextType {
	case ContextTypeBackground:
		ctx = context.Background()

	case ContextTypeWithCancel:
		ctx, cancelFunc = context.WithCancel(context.Background())

	case ContextTypeTimeout:
		timeout := time.Duration(timeoutSec) * time.Second
		ctx, cancelFunc = context.WithTimeout(context.Background(), timeout)

	case ContextTypeTODO:
		ctx = context.TODO()

	default:
		slog.Error("***** Alarm",
			"message", "Invalid context type",
			"context_type", contextType)
		default_timeout_sec := 30
		timeout := time.Duration(default_timeout_sec) * time.Second
		ctx, cancelFunc = context.WithTimeout(context.Background(), timeout)
	}

	return &JimoLogger{
		ctx:        ctx,
		cancel:     cancelFunc,
		call_depth: 2,
		logger:     getLoggerHandler(loggerType),
		reqID:      generateRequestID("e")}
}

func getLoggerHandler(handlerType string) *slog.Logger {
	switch handlerType {
	case LogHandlerTypeDefault:
		return slog.New(slog.NewTextHandler(os.Stdout, nil))

	case LogHandlerTypeJSON:
		return slog.New(slog.NewJSONHandler(os.Stdout, nil))

	default:
		slog.Error("***** Alarm",
			"message", "Invalid log handler type",
			"logger_type", handlerType)
		return slog.New(slog.NewTextHandler(os.Stdout, nil))
	}
}

// Info logs an informational message with context, location, and additional key-value pairs
func (l *JimoLogger) Info(message string, args ...any) {
	msg := fmt.Sprintf("[req=%s]", l.reqID)

	call_flow := GetCallStack(l.call_depth)
	logArgs := append([]any{"message", message, "call_flow", call_flow}, args...)
	l.logger.Info(msg, logArgs...)
}

// Warn logs a warning message with context, location, and additional key-value pairs
func (l *JimoLogger) Warn(message string, args ...any) {
	msg := fmt.Sprintf("[req=%s]", l.reqID)

	call_flow := GetCallStack(l.call_depth)
	logArgs := append([]any{"message", message, "call_flow", call_flow}, args...)
	l.logger.Warn(msg, logArgs...)
}

// Error logs an error message with context, location, and additional key-value pairs
func (l *JimoLogger) Error(message string, args ...any) {
	msg := fmt.Sprintf("[req=%s] ***** Alarm", l.reqID)

	call_flow := GetCallStack(l.call_depth)
	logArgs := append([]any{"message", message, "call_flow", call_flow}, args...)
	l.logger.Error(msg, logArgs...)
}

func (l *JimoLogger) Trace(msg string) {
	filename, line := GetCurrentLoc()
	if l.trace == "" {
		l.trace = fmt.Sprintf("[%s:%d %s]", filename, line, msg)
	}

	if len(l.trace) > 300 {
		l.trace = l.trace[50:]
	}

	if filename == l.currentFile {
		l.trace += fmt.Sprintf(", [%d %s]", line, msg)
		return
	}

	l.currentFile = filename
	l.trace += fmt.Sprintf(", [%s:%d %s]", filename, line, msg)
}

func (l *JimoLogger) Close() {
	if l.cancel != nil {
		l.cancel()
		l.cancel = nil // avoid double-cancel (harmless but good practice)
	}
}

func GetCallStack(depth int) string {
	_, file1, line1, ok1 := runtime.Caller(1)
	if !ok1 {
		return "empty stack"
	}
	filename1 := filepath.Base(file1)

	_, file2, line2, ok2 := runtime.Caller(2)
	if !ok2 {
		return fmt.Sprintf("%s:%d", filename1, line1)
	}

	filename2 := filepath.Base(file2)
	if depth == 2 {
		// It returns only one level
		return fmt.Sprintf("%s:%d", filename2, line2)
	}

	_, file3, line3, ok3 := runtime.Caller(3)
	if !ok3 {
		return fmt.Sprintf("%s:%d->%s:%d", filename2, line2, filename1, line1)
	}

	filename3 := filepath.Base(file3)
	return fmt.Sprintf("%s:%d->%s:%d", filename3, line3, filename2, line2)
}

func GetCurrentLoc() (string, int) {
	_, file, line, ok := runtime.Caller(0)
	if !ok {
		return "callstack-error", -1
	}
	filename := filepath.Base(file)
	return filename, line
}

// Note: this function is copied from ApiUtils. It is created so that
// this package is not dependent on ApiUtils.
func generateRequestID(key string) string {
	bytes := make([]byte, 4) // 4 bytes = 8 hex chars
	if _, err := rand.Read(bytes); err != nil {
		// Fallback if crypto/rand fails (very rare)
		return "fallback-req-id"
	}
	return key + "-" + hex.EncodeToString(bytes)
}
