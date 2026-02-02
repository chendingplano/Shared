## Logs

This document describes the logging system used across the shared library. The system is built on Go's standard `slog` package and provides structured logging with optional file output.

### Architecture Overview

The logging system consists of three main components:

1. **JimoLogger** - The logging interface and implementation (`loggerutil` package)
2. **File Writers** - Backend writers for file-based logging (`ApiUtils` package)
3. **Configuration** - Settings loaded from `libconfig.toml`

```
┌─────────────────────────────────────────────────────────────────┐
│                        JimoLogger                               │
│  (Interface: Info, Warn, Error, Trace, Close)                   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                     slog.Logger                                 │
│  (Handlers: Pretty, JSON, Text)                                 │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    io.MultiWriter                               │
│  (Writes to both stdout and file)                               │
└─────────────────────────────────────────────────────────────────┘
                    │                       │
                    ▼                       ▼
            ┌───────────┐          ┌────────────────┐
            │  stdout   │          │  File Writer   │
            └───────────┘          │  (lumberjack   │
                                   │  or filewriter)│
                                   └────────────────┘
```

---

## JimoLogger

JimoLogger is the primary logging interface used throughout the codebase. It wraps Go's `slog` package to provide structured logging with request tracing and call stack information.

### Understanding slog (Go's Structured Logging)

Go 1.21 introduced `slog` (package `log/slog`) as the standard library's structured logging solution. Key concepts:

- **Structured Logging**: Instead of plain text messages, slog produces key-value pairs that are machine-parseable
- **Handlers**: Determine how logs are formatted (JSON, text, or custom)
- **Levels**: `Debug`, `Info`, `Warn`, `Error` - control log verbosity
- **Attributes**: Additional key-value pairs attached to log entries

Example of slog output:
```
# Text format
time=2026-01-28T10:15:30.000Z level=INFO msg="User logged in" user_id=123 ip=192.168.1.1

# JSON format
{"time":"2026-01-28T10:15:30.000Z","level":"INFO","msg":"User logged in","user_id":123,"ip":"192.168.1.1"}
```

### JimoLogger Interface

Defined in `ApiTypes/ApiTypes.go`:

```go
type JimoLogger interface {
    Info(message string, args ...any)   // Informational messages
    Warn(message string, args ...any)   // Warning messages
    Error(message string, args ...any)  // Error messages (prefixed with "***** Alarm")
    Trace(message string)               // Lightweight tracing (no immediate output)
    Close()                             // Release resources (cancel context if applicable)
}
```

### Log Format Options

JimoLogger supports multiple output formats via the `LogFormat` type:

| Format | Description |
|:-------|:------------|
| `LogHandlerTypeDefault` | Pretty format with colors (uses slogpretty) |
| `LogHandlerTypePretty` | Same as Default |
| `LogHandlerTypeJSON` | Machine-parseable JSON format |
| `LogHandlerTypeText` | Standard slog text format |

### Features

1. **Request ID Tracking**: Each logger instance has a unique request ID (e.g., `e-a1b2c3d4`) included in every log entry for distributed tracing.

2. **Call Stack Information**: Logs automatically include the call flow showing file names and line numbers (e.g., `handler.go:42->service.go:156`).

3. **Singleton Pattern**: Logger handlers are created once and reused across instances to minimize overhead.

4. **Context Integration**: Loggers can be created with different context types (`Background`, `WithCancel`, `Timeout`, `TODO`) for lifecycle management.

### Relations between slog and JimoLogger
JimoLogger internally holds a pointer to slog.logger:
```text
type JimoLoggerImpl struct {
	ctx         context.Context
	cancel      context.CancelFunc
	logger      *slog.Logger
	reqID       string
	trace       string
	currentFile string
	call_depth  int
}
```
This is a wrapper class. In addition to slog, it also hold the context (ctx). This class is supposed to be created for each request the backend receives. At the creation time, it generates a reqID (a random string with fixed length). Its member data 'call_depth' controls the call stack depth (bottom up) it will print in the logs. In most cases, you can safe use the default (depth = 2). But if you want to control the depth, you can used the advanced creation functions (TBD) to adjust the call depth.

JimoLogger plays two roles: logging and tracing. It is for this reason, JimoLogger is mutable (mainly due to the member 'trace'). Traces are not displayed in the logs. This is reserved for OpenTelemetry (To be implemeneted).

It is for this reason that one need to create a JimoLogger instance at the entrance of every request handler. The same is true for services, background threads, etc.

### Usage Examples

#### Creating a Logger

```go
import (
    "github.com/chendingplano/shared/go/api/loggerutil"
    "github.com/chendingplano/shared/go/api/ApiTypes"
)

// Create a local global logger
logger := loggerutil.CreateDefaultLogger()
defer logger.Close()

// Advanced: Create with specific format and context
logger := loggerutil.CreateLogger(ctx, loggerutil.LogHandlerTypeJSON)
defer logger.Close()
```

// Create a logger for a request handler
```go
import (
    "github.com/chendingplano/shared/go/api/loggerutil"
    "github.com/chendingplano/shared/go/api/ApiTypes"
)

func HandleEmailLogin(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "SHD_EML_073")
	defer rc.Close()
	logger := rc.GetLogger()
```
The function EchoFactory.NewFromEcho(...) will create a logger:

```go
func NewFromEcho(c echo.Context, loc string) ApiTypes.RequestContext {
	ctx := c.Request().Context()
	logger := loggerutil.CreateLogger(ctx, loggerutil.LogHandlerTypeDefault)
	ee := &echoContext{
		c:         c,
		call_flow: []string{loc},
		ctx:       ctx,
		logger:    logger,
		is_admin:  false,
	}

	ee.PushCallFlow(loc)
	return ee
}
```

#### Logging Messages

```go
// Basic info log
logger.Info("User authenticated successfully")

// Info with additional context (key-value pairs)
logger.Info("User authenticated",
    "user_id", userID,
    "method", "OAuth",
    "duration_ms", elapsed.Milliseconds())

// Warning
logger.Warn("Rate limit approaching",
    "current", currentCount,
    "limit", maxLimit)

// Error (automatically prefixed with "***** Alarm" for alerting)
logger.Error("Database connection failed",
    "error", err.Error(),
    "host", dbHost,
    "retry_count", retries)

// Trace (lightweight, accumulates without immediate output)
logger.Trace("checkpoint-1")
```

#### Output Examples

Pretty format (default):
```
2026-01-28 10:15:30 INFO [req=e-a1b2c3d4] message="User authenticated" call_flow="handler.go:42->auth.go:156" user_id=123
```

JSON format:
```json
{"time":"2026-01-28T10:15:30Z","level":"INFO","msg":"[req=e-a1b2c3d4]","message":"User authenticated","call_flow":"handler.go:42->auth.go:156","user_id":123}
```

Error format (note the alarm marker):
```
2026-01-28 10:15:30 ERROR [req=e-a1b2c3d4] ***** Alarm message="Database connection failed" call_flow="db.go:89->service.go:45" error="connection refused"
```

#### Using with RequestContext

In request handlers, loggers are typically accessed via `RequestContext`:

```go
func HandleRequest(rc ApiTypes.RequestContext) {
    logger := rc.GetLogger()
    defer rc.Close()  // This also closes the logger

    logger.Info("Processing request", "endpoint", "/api/users")
    // ... handle request
}
```

---

## File Writers

The logging system supports two file writers for persistent log storage. When configured with a file writer, logs are written to both stdout and log files simultaneously via `io.MultiWriter`.

### Concurrency Policy

File writers are thread-safe through mutex protection. However, **only one process should use the same file writer configuration**. Using identical configurations from multiple processes on the same machine will result in undefined behavior (file corruption, interleaved writes).

### Available Writers

| Writer | Package | Description |
|:-------|:--------|:------------|
| **lumberjack** | `gopkg.in/natefinch/lumberjack.v2` | Third-party, feature-rich, production-proven |
| **filewriter** | `ApiUtils` | Self-developed, lightweight (experimental as of 2026/01/28) |

### lumberjack

A popular third-party log rotation library. Features:

- Automatic file rotation based on size
- Configurable retention by age and backup count
- Optional compression of rotated files
- Time-based file deletion

**Retention Policy**: A log file is deleted when:
- The total number of backup files exceeds `MaxBackups`, OR
- The file age exceeds `MaxAge` (in days)

**Caution**: Lumberjack does not control total disk space directly. Within the max age period, log files can grow unbounded. To prevent excessive disk usage, configure `MaxBackups` appropriately:

```
Total disk space ≈ MaxSize × (MaxBackups + 1)

Example: 500 MB max size × 21 files = ~10 GB maximum
```

### filewriter (Custom Implementation)

A lightweight rotating file writer implemented in `ApiUtils/log-writer.go`. Features:

- Fixed number of rotating log files (`log_00`, `log_01`, ..., `log_N`)
- Circular rotation: when `log_N` is full, overwrites `log_00`
- Buffered writing for performance
- ANSI escape code stripping for clean file output
- Resumes from most recently modified file on restart

**Retention Policy**: Maintains exactly `numFiles` log files. When the current file exceeds `maxSizeBytes`:
1. Flushes and closes current file
2. Advances to next file index (wrapping to 0 after reaching max)
3. Truncates the target file (overwrites old content)

Unlike lumberjack, filewriter does not delete files but old contents are overwritten.

**Disk space calculation**:
```
Total disk space ≈ FileMaxSize × NumFiles

Example: 500 MB × 20 files = 10 GB maximum
```

---

## Configuration

Logs are configured in `libconfig.toml` under the `[proc_log]` section.

### Environment Variables
| Name | Values | Description |
|:-----|:-----|:------------|
| `FILE_LOGGER` | 'nofilelogger', 'filewriter', 'lumberjack' | The file writer to use. Defaults to `"lumberjack"` if not specified or invalid. |


| Name | Type | Description |
|:-----|:-----|:------------|
| `log_file_dir` | **mandatory** | Directory for log files. Supports `~` expansion. System exits on error if not configured. |
| `file_max_size_in_mb` | optional | Maximum log file size in MB. Range: [10, 5000]. Default: 500. |
| `num_log_files` | optional | Number of rotating log files. Range: [2, 50]. Default: 20. |
| `max_age_in_days` | optional | Maximum retention days (lumberjack only). Range: [2, 180]. Default: 30. |
| `need_compress` | optional | Compress rotated files (lumberjack only). Values: `"true"` or anything else (false). Default: false. |

### Example Configuration

```toml
[proc_log]
log_file_dir = "~/logs/myapp"
file_max_size_in_mb = 500
num_log_files = 20
max_age_in_days = 30
need_compress = "false"
```

### Disabling File Logging

To log only to stdout, set the "proc_log" section in libconfig.toml as follows:

```toml
[proc_log]
log_file_dir = "/dev/null"  # Still required but unused
```

and set the environment variable in .env file:

```text
FILE_LOGGER=nofilelogger
```
---

## Initialization and Cleanup

### Automatic Initialization

File logging is initialized automatically when the first logger is created via `getOutput()` in the `loggerutil` package. This calls `ApiUtils.InitFileLogging()` which:

1. Loads `LibConfig` if not already loaded
2. Validates configuration parameters
3. Creates the log directory if needed
4. Initializes the appropriate file writer
5. Sets up `FileLogOutput` as a `MultiWriter`

### Manual Cleanup

For graceful shutdown, call `ApiUtils.CloseFileLogging()` to flush buffers and close file handles:

```go
import "github.com/chendingplano/shared/go/api/ApiUtils"

func main() {
    defer ApiUtils.CloseFileLogging()
    // ... application code
}
```

---

## Best Practices

1. **Always close loggers**: Use `defer logger.Close()` to release resources.

2. **Use structured key-value pairs**: Prefer `logger.Info("msg", "key", value)` over string formatting.

3. **Include context in errors**: Add relevant identifiers (user ID, request ID, etc.) for debugging.

4. **Reserve Error level for true errors**: Use `Warn` for recoverable issues, `Error` for failures requiring attention.

5. **Calculate disk space needs**: Configure `file_max_size_in_mb` × `num_log_files` to match your disk budget.

6. **Single process per configuration**: Don't share file logging configuration across multiple processes.

---

## TODO

- Implement an archive service for situations where logs should never be deleted (move old logs to cold storage instead of deletion)
- Add Debug level support to JimoLogger interface
- Consider adding log sampling for high-volume scenarios
