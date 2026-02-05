package logs2db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Location codes for purge operations
const (
	LOC_PURGE_START = "SHD_L2D_050"
	LOC_PURGE_DEL   = "SHD_L2D_051"
)

// PurgeResult summarizes the purge operation.
type PurgeResult struct {
	FilesKept    []string
	FilesDeleted []string
	FilesSkipped []string // not fully loaded, skipped
	FreedBytes   int64
	Errors       []string
}

// Purge keeps the maxFiles most recent log files and deletes older ones,
// but ONLY if they have been fully loaded into the database.
func (s *Log2DBService) Purge(ctx context.Context, maxFiles int) (*PurgeResult, error) {
	result := &PurgeResult{}

	if maxFiles < 1 {
		return nil, fmt.Errorf("maxfiles must be at least 1 (%s)", LOC_PURGE_START)
	}

	// Discover all log files sorted by modification time (oldest first)
	files, err := s.DiscoverLogFiles()
	if err != nil {
		return nil, err
	}

	if len(files) <= maxFiles {
		// Nothing to purge
		for _, f := range files {
			result.FilesKept = append(result.FilesKept, filepath.Base(f))
		}
		return result, nil
	}

	// Keep the newest maxFiles files (they are at the end since sorted oldest-first)
	cutoff := len(files) - maxFiles

	// Files to keep (newest)
	for _, f := range files[cutoff:] {
		result.FilesKept = append(result.FilesKept, filepath.Base(f))
	}

	// Files candidates for deletion (oldest)
	for _, filePath := range files[:cutoff] {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		basename := filepath.Base(filePath)

		// Safety: ensure file is within the log directory
		absPath, err := filepath.Abs(filePath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("cannot resolve path for %s: %v", basename, err))
			continue
		}
		absDir, err := filepath.Abs(s.config.LogFileDir)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("cannot resolve log dir: %v", err))
			continue
		}
		if !strings.HasPrefix(absPath, absDir+string(os.PathSeparator)) {
			result.Errors = append(result.Errors, fmt.Sprintf("file %s is outside log directory, skipping", basename))
			continue
		}

		// Check if the file has been fully loaded
		lastLine := s.state.GetLastLine(basename)
		if lastLine == 0 {
			result.FilesSkipped = append(result.FilesSkipped, basename)
			s.logger.Warn("Skipping purge: file not tracked in state",
				"file", basename, "loc", LOC_PURGE_DEL)
			continue
		}

		// Count actual lines in the file to verify it's fully loaded
		totalLines, err := CountFileLines(filePath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("cannot count lines in %s: %v", basename, err))
			continue
		}

		if lastLine < totalLines {
			result.FilesSkipped = append(result.FilesSkipped, basename)
			s.logger.Warn("Skipping purge: file not fully loaded",
				"file", basename,
				"loaded_lines", lastLine,
				"total_lines", totalLines,
				"loc", LOC_PURGE_DEL)
			continue
		}

		// Get file size before deleting
		info, err := os.Stat(filePath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("cannot stat %s: %v", basename, err))
			continue
		}

		// Delete the file
		if err := os.Remove(filePath); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to delete %s: %v", basename, err))
			continue
		}

		result.FilesDeleted = append(result.FilesDeleted, basename)
		result.FreedBytes += info.Size()

		// Remove from state tracking
		if err := s.state.RemoveFile(basename); err != nil {
			s.logger.Warn("Failed to remove file from state after deletion",
				"file", basename, "error", err)
		}

		s.logger.Info("Purged log file",
			"file", basename,
			"size_bytes", info.Size(),
			"loc", LOC_PURGE_DEL)
	}

	return result, nil
}
