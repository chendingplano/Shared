package loggerutil

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/Marlliton/slogpretty"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
)

type ContextType string

const (
	ContextTypeBackground ContextType = "background"
	ContextTypeWithCancel ContextType = "with_cancel"
	ContextTypeTimeout    ContextType = "timeout"
	ContextTypeTODO       ContextType = "todo"
)

type LogFormat int

const (
	LogHandlerTypeDefault LogFormat = iota
	LogHandlerTypeJSON
	LogHandlerTypePretty
	LogHandlerTypeText
)

// Singleton logger instances - created once and reused
var (
	defaultLogger *slog.Logger
	jsonLogger    *slog.Logger
	prettyLogger  *slog.Logger
	textLogger    *slog.Logger

	defaultOnce sync.Once
	jsonOnce    sync.Once
	prettyOnce  sync.Once
	textOnce    sync.Once
)

// getOutput returns the appropriate io.Writer for logging
// Returns MultiWriter(stdout, file) if file logging is initialized, otherwise just stdout
func getOutput(loc string) io.Writer {
	ApiUtils.InitFileLogging(loc)

	if ApiUtils.FileLogOutput != nil {
		slog.Info("User fileLog (SHD_JLG_280)")
		return ApiUtils.FileLogOutput
	}
	slog.Info("User stdio (SHD_JLG_283)")
	return os.Stdout
}

func CreateLogger(
	ctx context.Context,
	loggerType LogFormat,
	loc string) ApiTypes.JimoLogger {

	return &JimoLoggerImpl{
		ctx:    ctx,
		cancel: nil,
		logger: getLoggerHandler(loggerType, loc),
		reqID:  generateRequestID("e")}
}

type JimoLoggerImpl struct {
	ctx         context.Context
	cancel      context.CancelFunc
	logger      *slog.Logger
	reqID       string
	trace       string
	currentFile string
	call_depth  int
}

func CreateDefaultLogger(loc string) ApiTypes.JimoLogger {
	return createLogger(ContextTypeBackground, LogHandlerTypeDefault, 10000, loc)
}

func createLogger(
	contextType ContextType,
	loggerType LogFormat,
	timeoutSec int,
	loc string) ApiTypes.JimoLogger {
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

	return &JimoLoggerImpl{
		ctx:        ctx,
		cancel:     cancelFunc,
		call_depth: 2,
		logger:     getLoggerHandler(loggerType, loc),
		reqID:      generateRequestID("e")}
}

func newTextLogger(loc string) *slog.Logger {
	handler := slog.NewTextHandler(getOutput(loc), &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	return slog.New(handler)
}

func newJSONLogger(loc string) *slog.Logger {
	handler := slog.NewJSONHandler(getOutput(loc), &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	return slog.New(handler)
}

func newPrettyLogger(loc string) *slog.Logger {
	handler := slogpretty.New(getOutput(loc), &slogpretty.Options{
		TimeFormat: "2006-01-02 15:04:05",
	})
	return slog.New(handler)
}

func getLoggerHandler(handlerType LogFormat, loc string) *slog.Logger {
	slog.Info("getLoggerHandler (SHD_JLG_373)")
	switch handlerType {
	case LogHandlerTypeDefault:
		// Default: use slogpretty
		// Source: https://github.com/Marlliton/slogpretty
		defaultOnce.Do(func() {
			defaultLogger = newPrettyLogger(loc)
		})
		return defaultLogger

	case LogHandlerTypeJSON:
		jsonOnce.Do(func() {
			jsonLogger = newJSONLogger(loc)
		})
		return jsonLogger

	case LogHandlerTypeText:
		textOnce.Do(func() {
			textLogger = newTextLogger(loc)
		})
		return textLogger

	case LogHandlerTypePretty:
		// Source: https://github.com/Marlliton/slogpretty
		prettyOnce.Do(func() {
			prettyLogger = newPrettyLogger(loc)
		})
		return prettyLogger

	default:
		slog.Error("***** Alarm",
			"message", "Invalid log handler type",
			"logger_type", handlerType)
		// Fall back to default handler
		return getLoggerHandler(LogHandlerTypeDefault, loc)
	}
}

// Info logs an informational message with context, location, and additional key-value pairs
func (l *JimoLoggerImpl) Info(message string, args ...any) {
	msg := fmt.Sprintf("[req=%s]", l.reqID)

	call_flow := GetCallStack(l.call_depth)
	logArgs := append([]any{"message", message, "call_flow", call_flow}, args...)
	l.logger.Info(msg, logArgs...)
}

// Warn logs a warning message with context, location, and additional key-value pairs
func (l *JimoLoggerImpl) Warn(message string, args ...any) {
	msg := fmt.Sprintf("[req=%s]", l.reqID)

	call_flow := GetCallStack(l.call_depth)
	logArgs := append([]any{"message", message, "call_flow", call_flow}, args...)
	l.logger.Warn(msg, logArgs...)
}

// Error logs an error message with context, location, and additional key-value pairs
func (l *JimoLoggerImpl) Error(message string, args ...any) {
	msg := fmt.Sprintf("[req=%s] ***** Alarm", l.reqID)

	call_flow := GetCallStack(l.call_depth)
	logArgs := append([]any{"message", message, "call_flow", call_flow}, args...)
	l.logger.Error(msg, logArgs...)
}

func (l *JimoLoggerImpl) Trace(msg string) {
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

func (l *JimoLoggerImpl) Close() {
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
