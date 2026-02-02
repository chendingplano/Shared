package ApiUtils

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	ljack "gopkg.in/natefinch/lumberjack.v2"
)

// ansiRegex matches ANSI escape codes (color codes, cursor movement, etc.)
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// FileLogWriterStruct manages rotating log files
type FileLogWriterStruct struct {
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
	fileLogWriter *FileLogWriterStruct
	fileLogOnce   sync.Once
	FileLogOutput io.Writer // combined stdout + file writer
	FileWriter    io.Writer // file writer only
)

// InitFileLogging initializes file writer (FileLogOutput).
// logDir: directory for log files (will be created if doesn't exist)
// maxSizeMB: max size per log file in megabytes
// numFiles: number of rotating log files (log_00, log_01, ...)
func InitFileLogging(loc string) error {
	var initErr error
	fileLogOnce.Do(func() {
		slog.Info("InitFileLogging (SHD_JLG_078)", "loc", loc)
		LoadLibConfig(loc)

		file_logger := os.Getenv("FILE_LOGGER")
		if len(file_logger) == 0 {
			file_logger = "lumberjack"
			slog.Warn("FILE_LOGGER environment variable not set. Default to lumberjack (SHD_LWT_046)", "loc", loc)
		}

		slog.Info("file_logger (SHD_LWG_059)", "type", file_logger)
		if file_logger == "nofilelogger" {
			// No file logger used.
			slog.Warn("No file logger used (SHD_LWT_051)", "loc", loc)
			FileLogOutput = os.Stdout
			return
		}

		if file_logger != "filewriter" && file_logger != "lumberjack" {
			slog.Warn("Invalid file_logger. Default to lumberjack (SHD_LWT_056)", "value", file_logger)
			file_logger = "lumberjack"
		}

		logFileDir := os.Getenv("LOG_FILE_DIR")
		if len(logFileDir) == 0 {
			slog.Error("LOG_FILE_DIR environment variable not set. Default to stdio only (SHD_LWT_060)")
			FileLogOutput = os.Stdout
			return
		}

		maxAgeStr := os.Getenv("MAX_AGE_IN_DAYS")
		if len(maxAgeStr) == 0 {
			slog.Warn("MAX_AGE_IN_DAYS environment variable not set. Default to 30 days (SHD_LWT_075)")
			maxAgeStr = "30"
		}
		maxAge, err := strconv.Atoi(maxAgeStr)
		if err != nil {
			slog.Error("Error converting MAX_AGE_IN_DAYS to int. Default to 30 (days)", "error", err)
			maxAge = 30 // default value
		}

		var minAgeLimit = 1
		var maxAgeLimit = 365
		if maxAge < minAgeLimit || maxAge > maxAgeLimit {
			slog.Warn("Invalid MAX_AGE_IN_DAYS. Default to 30 days (SHD_LWT_080)",
				"value", maxAge,
				"minAge", minAgeLimit,
				"maxAge", maxAgeLimit)
			maxAge = 30
		}

		maxSizeMBStr := os.Getenv("MAX_SIZE_IN_MB")
		if len(maxSizeMBStr) == 0 {
			slog.Warn("MAX_SIZE_IN_MB environment variable not set. Default to 500 (MB) (SHD_LWT_078)")
			maxSizeMBStr = "500"
		}

		maxSizeMB, err := strconv.Atoi(maxSizeMBStr)
		if err != nil {
			slog.Error("Error converting MAX_SIZE_IN_MB to int. Default to 500 (MB)", "error", err)
			maxSizeMB = 500 // default value
		}

		var minSizeLimit = 10
		var maxSizeLimit = 5000
		if maxSizeMB < minSizeLimit || maxSizeMB > maxSizeLimit {
			slog.Warn("Invalid MAX_SIZE_IN_MB. Default to 500 (MB) (SHD_LWT_082)",
				"value", maxSizeMB,
				"minSize", minSizeLimit,
				"maxSize", maxSizeLimit)
			maxSizeMB = 500
		}

		numFilesStr := os.Getenv("NUM_LOG_FILES")
		if len(numFilesStr) == 0 {
			slog.Warn("NUM_LOG_FILES environment variable not set. Default to 20 (SHD_LWT_084)")
			numFilesStr = "20"
		}
		numFiles, err := strconv.Atoi(numFilesStr)
		if err != nil {
			slog.Error("Error converting NUM_LOG_FILES to int. Default to 20", "error", err)
			numFiles = 20 // default value
		}

		minNumFilesLimit := 2
		maxNumFilesLimit := 50
		if numFiles < minNumFilesLimit || numFiles > maxNumFilesLimit {
			slog.Warn("Invalid num-log-files. Defaults to 20 (SHD_LWT_087)", "value", numFiles)
			numFiles = 20
		}

		// Expand ~ to home directory
		if strings.HasPrefix(logFileDir, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				initErr = fmt.Errorf("failed to get home directory: %w (SHD_LWT_064), LOG_FILE_DIR:%s",
					err, logFileDir)
				slog.Error("failed to get home directory. Default to stdio only (SHD_LWT_065)", "error", err)
				FileLogOutput = os.Stdout
				return
			}
			logFileDir = filepath.Join(home, logFileDir[2:])
		}

		// Create log directory if it doesn't exist
		if err := os.MkdirAll(logFileDir, 0755); err != nil {
			initErr = fmt.Errorf("failed to create log directory %s: %w (SHD_LWT_072)", logFileDir, err)
			slog.Error("failed to create log directory. Default to stdio only (SHD_LWT_073)", "log_dir", logFileDir, "error", err)
			FileLogOutput = os.Stdout
			return
		}

		if file_logger == "filewriter" {
			flw := &FileLogWriterStruct{
				logDir:       logFileDir,
				maxSizeBytes: int64(maxSizeMB) * 1024 * 1024,
				numFiles:     numFiles,
			}

			// Find the most recently modified log file to continue from
			if err := flw.findCurrentLogFile(); err != nil {
				initErr = fmt.Errorf("failed to find current log file: %w", err)
				slog.Error("failed to find current log file. Default to stdio only (SHD_LWT_089)", "error", err)
				FileLogOutput = os.Stdout
				return
			}

			// Open the current log file
			if err := flw.openCurrentFile(); err != nil {
				initErr = fmt.Errorf("failed to open log file: %w", err)
				slog.Error("failed to open log file. Default to stdio only (SHD_LWT_092)", "error", err)
				FileLogOutput = os.Stdout
				return
			}

			FileWriter = flw
			slog.Info("Create file writer (SHD_JLG_115)",
				"file_dir", logFileDir,
				"max_size", maxSizeMB,
				"num_files", numFiles)
			FileLogOutput = io.MultiWriter(os.Stdout, FileWriter)
			return
		}

		filename := fmt.Sprintf("%s/app.log", logFileDir)
		FileWriter = &ljack.Logger{
			Filename:   filename,
			MaxSize:    maxSizeMB, // megabytes
			MaxBackups: numFiles,
			MaxAge:     maxAge, // days
			Compress:   false,  // disabled by default
		}

		// Create a MultiWriter that writes to both stdout and the file
		slog.Info("Create lumberjack (SHD_JLG_138)",
			"file_dir", filename,
			"max_size", maxSizeMB,
			"num_files", numFiles,
			"max_age", maxAge)
		FileLogOutput = io.MultiWriter(os.Stdout, FileWriter)
	})
	return initErr
}

// findCurrentLogFile finds the most recently modified log file and sets currentIndex
func (flw *FileLogWriterStruct) findCurrentLogFile() error {
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
func (flw *FileLogWriterStruct) getLogFileName(index int) string {
	return filepath.Join(flw.logDir, fmt.Sprintf("log_%02d", index))
}

// openCurrentFile opens the current log file for appending
func (flw *FileLogWriterStruct) openCurrentFile() error {
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
func (flw *FileLogWriterStruct) Write(p []byte) (n int, err error) {
	flw.mu.Lock()
	defer flw.mu.Unlock()

	// Strip ANSI escape codes (color codes) for clean plain text output
	cleaned := ansiRegex.ReplaceAll(p, nil)

	// Check if we need to rotate before writing
	if flw.currentSize+int64(len(cleaned)) > flw.maxSizeBytes {
		if err := flw.rotate(); err != nil {
			// If rotation fails, still try to write to current file
			// Log the error but don't fail the write
			fmt.Fprintf(os.Stderr, "log rotation failed: %v\n", err)
		}
	}

	// Write to buffered writer (no hard sync)
	written, err := flw.bufferedWriter.Write(cleaned)
	if err != nil {
		return len(p), err
	}

	flw.currentSize += int64(written)

	// Flush the buffer periodically but don't sync to disk
	// This provides reasonable durability without performance impact
	if flw.bufferedWriter.Buffered() > 4096 {
		flw.bufferedWriter.Flush()
	}

	// Return original length to satisfy io.Writer contract
	return len(p), nil
}

// rotate closes the current file and opens the next one
func (flw *FileLogWriterStruct) rotate() error {
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
