package ApiUtils

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
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
		LoadLibConfig()

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
