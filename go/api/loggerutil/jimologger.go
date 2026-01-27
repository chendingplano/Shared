package loggerutil

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
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

// FileLogWriter manages rotating log files
type FileLogWriter struct {
	mu             sync.Mutex
	logDir         string
	maxSizeBytes   int64
	numFiles       int
	currentIndex   int
	currentSize    int64
	currentFile    *os.File
	bufferedWriter *bufio.Writer
}

var (
	fileLogWriter *FileLogWriter
	fileLogOnce   sync.Once
	fileLogOutput io.Writer // combined stdout + file writer
)

// InitFileLogging initializes file logging with rotation support.
// Must be called after LibConfig is loaded.
// logDir: directory for log files (will be created if doesn't exist)
// maxSizeMB: max size per log file in megabytes
// numFiles: number of rotating log files (log_00, log_01, ...)
func initFileLogging() error {
	var initErr error
	slog.Info("InitFileLogging (SHD_JLG_078)")
	fileLogOnce.Do(func() {
		ApiUtils.LoadLibConfig()

		procLog := ApiTypes.LibConfig.ProcLog
		logDir := procLog.LogFileDir
		maxSizeMB := procLog.FileMaxSizeInMB
		numFiles := procLog.NumLogFiles

		// Expand ~ to home directory
		if strings.HasPrefix(logDir, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				initErr = fmt.Errorf("failed to get home directory: %w", err)
				return
			}
			logDir = filepath.Join(home, logDir[2:])
		}

		// Create log directory if it doesn't exist
		if err := os.MkdirAll(logDir, 0755); err != nil {
			initErr = fmt.Errorf("failed to create log directory %s: %w", logDir, err)
			return
		}

		flw := &FileLogWriter{
			logDir:       logDir,
			maxSizeBytes: int64(maxSizeMB) * 1024 * 1024,
			numFiles:     numFiles,
		}

		// Find the most recently modified log file to continue from
		if err := flw.findCurrentLogFile(); err != nil {
			initErr = fmt.Errorf("failed to find current log file: %w", err)
			return
		}

		// Open the current log file
		if err := flw.openCurrentFile(); err != nil {
			initErr = fmt.Errorf("failed to open log file: %w", err)
			return
		}

		fileLogWriter = flw
		// Create a MultiWriter that writes to both stdout and the file
		slog.Info("Create fileLog (SHD_JLG_115)",
			"file_dir", procLog.LogFileDir,
			"max_size", procLog.FileMaxSizeInMB,
			"num_files", procLog.NumLogFiles)
		fileLogOutput = io.MultiWriter(os.Stdout, fileLogWriter)
	})
	return initErr
}

// findCurrentLogFile finds the most recently modified log file and sets currentIndex
func (flw *FileLogWriter) findCurrentLogFile() error {
	type fileInfo struct {
		index   int
		modTime time.Time
		size    int64
	}

	var files []fileInfo

	for i := 0; i < flw.numFiles; i++ {
		filename := flw.getLogFileName(i)
		info, err := os.Stat(filename)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		files = append(files, fileInfo{
			index:   i,
			modTime: info.ModTime(),
			size:    info.Size(),
		})
	}

	if len(files) == 0 {
		// No existing log files, start with log_00
		flw.currentIndex = 0
		flw.currentSize = 0
		return nil
	}

	// Sort by modification time, most recent first
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})

	// Use the most recently modified file
	mostRecent := files[0]
	flw.currentIndex = mostRecent.index
	flw.currentSize = mostRecent.size

	// If current file is at or over max size, rotate to next
	if flw.currentSize >= flw.maxSizeBytes {
		flw.currentIndex = (flw.currentIndex + 1) % flw.numFiles
		flw.currentSize = 0
	}

	return nil
}

// getLogFileName returns the full path for a log file at the given index
func (flw *FileLogWriter) getLogFileName(index int) string {
	return filepath.Join(flw.logDir, fmt.Sprintf("log_%02d", index))
}

// openCurrentFile opens the current log file for appending
func (flw *FileLogWriter) openCurrentFile() error {
	filename := flw.getLogFileName(flw.currentIndex)

	// Open file for append, create if doesn't exist
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	// Get current file size
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return err
	}

	flw.currentFile = file
	flw.currentSize = info.Size()
	flw.bufferedWriter = bufio.NewWriter(file)

	return nil
}

// Write implements io.Writer interface
func (flw *FileLogWriter) Write(p []byte) (n int, err error) {
	flw.mu.Lock()
	defer flw.mu.Unlock()

	// Check if we need to rotate before writing
	if flw.currentSize+int64(len(p)) > flw.maxSizeBytes {
		if err := flw.rotate(); err != nil {
			// If rotation fails, still try to write to current file
			// Log the error but don't fail the write
			fmt.Fprintf(os.Stderr, "log rotation failed: %v\n", err)
		}
	}

	// Write to buffered writer (no hard sync)
	n, err = flw.bufferedWriter.Write(p)
	if err != nil {
		return n, err
	}

	flw.currentSize += int64(n)

	// Flush the buffer periodically but don't sync to disk
	// This provides reasonable durability without performance impact
	if flw.bufferedWriter.Buffered() > 4096 {
		flw.bufferedWriter.Flush()
	}

	return n, nil
}

// rotate closes the current file and opens the next one
func (flw *FileLogWriter) rotate() error {
	// Flush and close current file
	if flw.bufferedWriter != nil {
		flw.bufferedWriter.Flush()
	}
	if flw.currentFile != nil {
		flw.currentFile.Close()
	}

	// Move to next file index (wrapping around)
	flw.currentIndex = (flw.currentIndex + 1) % flw.numFiles

	// Truncate the next file (overwrite old logs)
	filename := flw.getLogFileName(flw.currentIndex)
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	flw.currentFile = file
	flw.currentSize = 0
	flw.bufferedWriter = bufio.NewWriter(file)

	return nil
}

// CloseFileLogging flushes and closes the log file
func CloseFileLogging() {
	if fileLogWriter != nil {
		fileLogWriter.mu.Lock()
		defer fileLogWriter.mu.Unlock()

		if fileLogWriter.bufferedWriter != nil {
			fileLogWriter.bufferedWriter.Flush()
		}
		if fileLogWriter.currentFile != nil {
			fileLogWriter.currentFile.Close()
			fileLogWriter.currentFile = nil
		}
	}
}

// getOutput returns the appropriate io.Writer for logging
// Returns MultiWriter(stdout, file) if file logging is initialized, otherwise just stdout
func getOutput() io.Writer {
	initFileLogging()

	if fileLogOutput != nil {
		slog.Info("User fileLog (SHD_JLG_280)")
		return fileLogOutput
	}
	slog.Info("User stdio (SHD_JLG_283)")
	return os.Stdout
}

func CreateLogger(
	ctx context.Context,
	loggerType LogFormat) ApiTypes.JimoLogger {

	return &JimoLoggerImpl{
		ctx:    ctx,
		cancel: nil,
		logger: getLoggerHandler(loggerType),
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

func CreateDefaultLogger() ApiTypes.JimoLogger {
	return createLogger(ContextTypeBackground, LogHandlerTypeDefault, 10000)
}

func createLogger(
	contextType ContextType,
	loggerType LogFormat,
	timeoutSec int) ApiTypes.JimoLogger {
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
		logger:     getLoggerHandler(loggerType),
		reqID:      generateRequestID("e")}
}

func newTextLogger() *slog.Logger {
	handler := slog.NewTextHandler(getOutput(), &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	return slog.New(handler)
}

func newJSONLogger() *slog.Logger {
	handler := slog.NewJSONHandler(getOutput(), &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	return slog.New(handler)
}

func newPrettyLogger() *slog.Logger {
	handler := slogpretty.New(getOutput(), nil)
	return slog.New(handler)
}

func getLoggerHandler(handlerType LogFormat) *slog.Logger {
	slog.Info("getLoggerHandler (SHD_JLG_373)")
	switch handlerType {
	case LogHandlerTypeDefault:
		// Default: use slogpretty
		// Source: https://github.com/Marlliton/slogpretty
		defaultOnce.Do(func() {
			defaultLogger = newPrettyLogger()
		})
		return defaultLogger

	case LogHandlerTypeJSON:
		jsonOnce.Do(func() {
			jsonLogger = newJSONLogger()
		})
		return jsonLogger

	case LogHandlerTypeText:
		textOnce.Do(func() {
			textLogger = newTextLogger()
		})
		return textLogger

	case LogHandlerTypePretty:
		// Source: https://github.com/Marlliton/slogpretty
		prettyOnce.Do(func() {
			prettyLogger = newPrettyLogger()
		})
		return prettyLogger

	default:
		slog.Error("***** Alarm",
			"message", "Invalid log handler type",
			"logger_type", handlerType)
		// Fall back to default handler
		return getLoggerHandler(LogHandlerTypeDefault)
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
