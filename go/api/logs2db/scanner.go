package logs2db

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Location codes for scanner operations
const (
	LOC_SCAN_DISCOVER = "SHD_L2D_010"
	LOC_SCAN_FILE     = "SHD_L2D_011"
	LOC_SCAN_PARSE    = "SHD_L2D_012"
)

// LogEntry represents a single parsed log line ready for database insertion.
type LogEntry struct {
	ID              string
	EntryType       string
	Message         string
	SysPrompt       string
	SysPromptNLines int
	CallerFilename  string
	CallerLine      int
	JSONObj         []byte // raw JSON of the entire line
	LogFilename     string
	LogLineNum      int
	ErrorMsg        string
	Remarks         string
	CreatedAt       time.Time
	CreatedAtRaw    string // intermediate: raw string from JSON before parsing
}

// DiscoverLogFiles returns all log files in the configured directory,
// sorted by modification time (oldest first).
func (s *Log2DBService) DiscoverLogFiles() ([]string, error) {
	entries, err := os.ReadDir(s.config.LogFileDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read log directory %s: %w (%s)",
			s.config.LogFileDir, err, LOC_SCAN_DISCOVER)
	}

	type fileWithTime struct {
		path    string
		modTime time.Time
	}

	var files []fileWithTime
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		// Skip hidden files (state file, PID file, etc.)
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		fullPath := filepath.Join(s.config.LogFileDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			s.logger.Warn("Failed to stat log file", "file", entry.Name(), "error", err)
			continue
		}

		files = append(files, fileWithTime{
			path:    fullPath,
			modTime: info.ModTime(),
		})
	}

	// Sort by modification time, oldest first
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})

	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.path
	}

	return paths, nil
}

// ScanFile reads a single log file starting from the given line offset,
// parses each line as JSON, extracts mapped fields, and returns LogEntry slices.
func (s *Log2DBService) ScanFile(ctx context.Context, filePath string, startLine int) ([]LogEntry, int, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to open log file %s: %w (%s)", filePath, err, LOC_SCAN_FILE)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Allow up to 1MB per line for potentially large JSON objects
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	var entries []LogEntry
	lineNum := 0
	basename := filepath.Base(filePath)

	for scanner.Scan() {
		// Check for cancellation periodically
		if lineNum%1000 == 0 {
			select {
			case <-ctx.Done():
				return entries, lineNum, ctx.Err()
			default:
			}
		}

		lineNum++
		if lineNum <= startLine {
			continue
		}

		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		entry := LogEntry{
			ID:          generateUUIDv7(),
			LogFilename: basename,
			LogLineNum:  lineNum,
			CreatedAt:   time.Now(), // default, overridden if parsed from JSON
		}

		var data map[string]any
		if err := json.Unmarshal([]byte(line), &data); err != nil {
			// Malformed JSON -- record with error
			entry.ErrorMsg = fmt.Sprintf("JSON parse error: %v", err)
			entry.Message = truncateString(line, 4000)
			entry.EntryType = "ERROR"
			entry.JSONObj = []byte("{}") // empty JSON object for JSONB column
		} else {
			entry.JSONObj = []byte(line)
			applyMapping(s.config.JSONMapping, data, &entry)

			// Parse created_at from raw string
			if entry.CreatedAtRaw != "" {
				if t, err := time.Parse(time.RFC3339Nano, entry.CreatedAtRaw); err == nil {
					entry.CreatedAt = t
				} else if t, err := time.Parse(time.RFC3339, entry.CreatedAtRaw); err == nil {
					entry.CreatedAt = t
				}
			}

			// Ensure required fields have values
			if entry.EntryType == "" {
				entry.EntryType = "UNKNOWN"
			}
			if entry.Message == "" {
				entry.Message = truncateString(line, 4000)
			}
		}

		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return entries, lineNum, fmt.Errorf("error reading log file %s: %w (%s)", filePath, err, LOC_SCAN_FILE)
	}

	return entries, lineNum, nil
}

// CountFileLines counts the total number of lines in a file.
func CountFileLines(filePath string) (int, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	count := 0
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
}

// generateUUIDv7 generates a UUID v7 string.
func generateUUIDv7() string {
	id, err := uuid.NewV7()
	if err != nil {
		// Fallback to v4 if v7 fails
		return uuid.NewString()
	}
	return id.String()
}

// truncateString truncates a string to the given maximum length.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
