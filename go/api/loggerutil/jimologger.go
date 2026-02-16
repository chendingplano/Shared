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
	"sync"
	"time"

	"github.com/Marlliton/slogpretty"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/lmittmann/tint"
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
	LogHandlerTypePretty
	LogHandlerTypeTint
)

// Singleton logger instances - created once and reused
var (
	devLogger  *slog.Logger
	jsonLogger *slog.Logger

	devOnce  sync.Once
	jsonOnce sync.Once
)

/*
func CreateLogger(
	ctx context.Context,
	loggerType LogFormat,
	loc string) ApiTypes.JimoLogger {

	return &JimoLoggerImpl{
		ctx:    ctx,
		cancel: nil,
		logger: getLogger(loggerType),
		reqID:  generateRequestID("e")}
}
*/

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
			"context_type", contextType,
			"loc", loc)
		default_timeout_sec := 30
		timeout := time.Duration(default_timeout_sec) * time.Second
		ctx, cancelFunc = context.WithTimeout(context.Background(), timeout)
	}

	return &JimoLoggerImpl{
		ctx:        ctx,
		cancel:     cancelFunc,
		call_depth: 2,
		logger:     getLogger(loggerType),
		reqID:      generateRequestID("e")}
}

func newDevLogger(consoleHandler slog.Handler) *slog.Logger {
	ApiUtils.InitFileLogging("SHD_JLG_118")
	fileWriter := ApiUtils.FileWriter

	if fileWriter == nil {
		slog.Info("newDevLogger with no file writer (SHD_JLG_124)")
		return slog.New(consoleHandler)
	}

	// file → text, no color
	slog.Info("newDevLogger with file writer (SHD_JLG_129)")
	fileHandler := slog.NewTextHandler(fileWriter, &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: false,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.String("time", a.Value.Time().Format("2006-01-02 15:04:05"))
			}
			return a
		},
	})

	handler := MultiHandler{
		handlers: []slog.Handler{
			consoleHandler,
			fileHandler,
		},
	}

	return slog.New(handler)
}

func newJSONLogger(consoleHandler slog.Handler) *slog.Logger {
	ApiUtils.InitFileLogging("SHD_JLG_141")
	fileWriter := ApiUtils.FileWriter

	if fileWriter == nil {
		return slog.New(consoleHandler)
	}

	fileHandler := slog.NewJSONHandler(fileWriter, &slog.HandlerOptions{
		Level:     slog.LevelInfo,
		AddSource: true,
	})

	handler := MultiHandler{
		handlers: []slog.Handler{
			consoleHandler,
			fileHandler,
		},
	}

	return slog.New(handler)
}

func getConsoleHandler(handlerType LogFormat, loc string) slog.Handler {
	switch handlerType {
	case LogHandlerTypeDefault:
		// Default: use slogpretty
		// Source: https://github.com/Marlliton/slogpretty
		handler := slogpretty.New(os.Stdout, &slogpretty.Options{
			TimeFormat: "2006-01-02 15:04:05",
		})
		return handler

	case LogHandlerTypePretty:
		// Source: https://github.com/Marlliton/slogpretty
		handler := slogpretty.New(os.Stdout, &slogpretty.Options{
			TimeFormat: "2006-01-02 15:04:05",
		})
		return handler

	case LogHandlerTypeTint:
		handler := tint.NewHandler(os.Stdout, &tint.Options{
			Level:      slog.LevelDebug,
			AddSource:  true,
			TimeFormat: "15:04:05",
		})
		return handler

	default:
		slog.Error("Invalid log handler type. Falling back to default handler",
			"handlerType", handlerType,
			"loc", loc)
		// Fall back to default handler
		return getConsoleHandler(LogHandlerTypeDefault, loc)
	}
}

func getLogger(handlerType LogFormat) *slog.Logger {
	is_json := false
	if is_json {
		jsonOnce.Do(func() {
			consoleHandler := getConsoleHandler(handlerType, "SHD_JLG_250")
			jsonLogger = newJSONLogger(consoleHandler)
		})
		return jsonLogger
	}

	devOnce.Do(func() {
		consoleHandler := getConsoleHandler(handlerType, "SHD_JLG_259")
		devLogger = newDevLogger(consoleHandler)
	})
	return devLogger
}

// Debug logs a debug-level message for happy-path diagnostics that are too noisy for Info
func (l *JimoLoggerImpl) Debug(message string, args ...any) {
	msg := fmt.Sprintf("[req=%s] %s", l.reqID, message)

	call_flow := fmt.Sprintf("[%s]", GetCallStack(l.call_depth))
	logArgs := append([]any{"call_flow", call_flow}, args...)
	l.logger.Debug(msg, logArgs...)
}

// Info logs an informational message with context, location, and additional key-value pairs
func (l *JimoLoggerImpl) Info(message string, args ...any) {
	msg := fmt.Sprintf("[req=%s] %s", l.reqID, message)

	call_flow := fmt.Sprintf("[%s]", GetCallStack(l.call_depth))
	logArgs := append([]any{"call_flow", call_flow}, args...)
	l.logger.Info(msg, logArgs...)
}

// Warn logs a warning message with context, location, and additional key-value pairs
func (l *JimoLoggerImpl) Warn(message string, args ...any) {
	msg := fmt.Sprintf("[req=%s] %s", l.reqID, message)

	call_flow := fmt.Sprintf("[%s]", GetCallStack(l.call_depth))
	logArgs := append([]any{"call_flow", call_flow}, args...)
	l.logger.Warn(msg, logArgs...)
}

// Error logs an error message with context, location, and additional key-value pairs
func (l *JimoLoggerImpl) Error(message string, args ...any) {
	msg := fmt.Sprintf("[req=%s] %s", l.reqID, message)

	call_flow := fmt.Sprintf("[%s]", GetCallStack(l.call_depth))
	logArgs := append([]any{"call_flow", call_flow}, args...)
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
	if depth == 1 {
		// It returns only one level
		return fmt.Sprintf("%s:%d", filename2, line2)
	}

	_, file3, line3, ok3 := runtime.Caller(3)
	if !ok3 {
		return fmt.Sprintf("%s:%d", filename2, line2)
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
