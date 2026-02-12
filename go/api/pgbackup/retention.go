package pgbackup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Location codes for retention operations
const (
	LOC_RETENTION_START = "SHD_PGB_030"
	LOC_RETENTION_LIST  = "SHD_PGB_031"
	LOC_RETENTION_DEL   = "SHD_PGB_032"
	LOC_RETENTION_WAL   = "SHD_PGB_033"
)

// RetentionResult contains information about a cleanup operation
type RetentionResult struct {
	DeletedBackups  []string `json:"deleted_backups"`
	DeletedWALFiles int      `json:"deleted_wal_files"`
	RetainedBackups []string `json:"retained_backups"`
	FreedSpaceBytes int64    `json:"freed_space_bytes"`
}

// ApplyRetention removes old backups according to retention policy
func (s *BackupService) ApplyRetention(ctx context.Context, logger *slog.Logger) (*RetentionResult, error) {
	logger.Info("Applying retention policy",
		"retain_days", s.config.RetainDays,
		"retain_count", s.config.RetainCount,
		"retain_wal_days", s.config.RetainWALDays)

	result := &RetentionResult{
		DeletedBackups:  []string{},
		RetainedBackups: []string{},
	}

	// List all base backups
	backups, err := s.ListBackups()
	if err != nil {
		return nil, fmt.Errorf("failed to list backups: %w (%s)", err, LOC_RETENTION_LIST)
	}

	if len(backups) == 0 {
		logger.Info("No backups found")
		return result, nil
	}

	// Sort by date (newest first)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].StartTime.After(backups[j].StartTime)
	})

	cutoffDate := time.Now().AddDate(0, 0, -s.config.RetainDays)

	// Process each backup
	for i, backup := range backups {
		// Always keep minimum count (newest backups)
		if i < s.config.RetainCount {
			result.RetainedBackups = append(result.RetainedBackups, backup.BackupID)
			logger.Info("Retaining backup (within minimum count)",
				"backup_id", backup.BackupID,
				"age", time.Since(backup.StartTime).Round(time.Hour))
			continue
		}

		// Delete backups older than retention period
		if backup.StartTime.Before(cutoffDate) {
			logger.Info("Deleting old backup",
				"backup_id", backup.BackupID,
				"age_days", int(time.Since(backup.StartTime).Hours()/24))

			// Calculate size before deletion
			size, _ := s.calculateDirSize(backup.BackupPath)

			if err := s.deleteBackup(backup.BackupPath); err != nil {
				logger.Warn("Failed to delete backup",
					"backup_id", backup.BackupID,
					"error", err)
				continue
			}

			result.DeletedBackups = append(result.DeletedBackups, backup.BackupID)
			result.FreedSpaceBytes += size
		} else {
			result.RetainedBackups = append(result.RetainedBackups, backup.BackupID)
			logger.Info("Retaining backup (within retention period)",
				"backup_id", backup.BackupID,
				"age_days", int(time.Since(backup.StartTime).Hours()/24))
		}
	}

	// Clean old WAL files
	walDeleted, walFreed, err := s.cleanOldWALFiles(ctx, logger, result.RetainedBackups)
	if err != nil {
		logger.Warn("Failed to clean WAL files", "error", err)
	} else {
		result.DeletedWALFiles = walDeleted
		result.FreedSpaceBytes += walFreed
	}

	logger.Info("Retention policy applied",
		"deleted_backups", len(result.DeletedBackups),
		"retained_backups", len(result.RetainedBackups),
		"deleted_wal_files", result.DeletedWALFiles,
		"freed_space_mb", float64(result.FreedSpaceBytes)/(1024*1024))

	return result, nil
}

// deleteBackup removes a backup directory
func (s *BackupService) deleteBackup(backupPath string) error {
	// Verify the path is within our backup directory (safety check)
	absBackupDir, _ := filepath.Abs(s.config.BaseBackupDir)
	absPath, _ := filepath.Abs(backupPath)

	if !strings.HasPrefix(absPath, absBackupDir) {
		return fmt.Errorf("refusing to delete path outside backup directory: %s (%s)",
			backupPath, LOC_RETENTION_DEL)
	}

	if err := os.RemoveAll(backupPath); err != nil {
		return fmt.Errorf("failed to remove backup: %w (%s)", err, LOC_RETENTION_DEL)
	}

	return nil
}

// cleanOldWALFiles removes WAL files no longer needed for recovery
func (s *BackupService) cleanOldWALFiles(_ context.Context, logger *slog.Logger, retainedBackups []string) (int, int64, error) {
	if _, err := os.Stat(s.config.WALArchiveDir); os.IsNotExist(err) {
		return 0, 0, nil
	}

	entries, err := os.ReadDir(s.config.WALArchiveDir)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read WAL archive: %w (%s)", err, LOC_RETENTION_WAL)
	}

	// Find the oldest retained backup to determine WAL cutoff
	var oldestRetainedTime time.Time
	for _, backupID := range retainedBackups {
		backup, err := s.GetBackup(backupID)
		if err != nil {
			continue
		}
		if oldestRetainedTime.IsZero() || backup.StartTime.Before(oldestRetainedTime) {
			oldestRetainedTime = backup.StartTime
		}
	}

	// If no retained backups, use WAL retention days
	walCutoff := time.Now().AddDate(0, 0, -s.config.RetainWALDays)
	if !oldestRetainedTime.IsZero() && oldestRetainedTime.Before(walCutoff) {
		walCutoff = oldestRetainedTime
	}

	var deleted int
	var freedBytes int64

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Delete WAL files older than cutoff
		if info.ModTime().Before(walCutoff) {
			walPath := filepath.Join(s.config.WALArchiveDir, entry.Name())

			// Safety check
			if !strings.HasPrefix(walPath, s.config.WALArchiveDir) {
				continue
			}

			size := info.Size()
			if err := os.Remove(walPath); err != nil {
				logger.Warn("Failed to delete WAL file", "file", entry.Name(), "error", err)
				continue
			}

			deleted++
			freedBytes += size
			logger.Info("Deleted old WAL file",
				"file", entry.Name(),
				"age_days", int(time.Since(info.ModTime()).Hours()/24))
		}
	}

	return deleted, freedBytes, nil
}

// GetOldestWALFile returns information about the oldest WAL file in the archive
func (s *BackupService) GetOldestWALFile() (string, time.Time, error) {
	entries, err := os.ReadDir(s.config.WALArchiveDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", time.Time{}, nil
		}
		return "", time.Time{}, err
	}

	var oldest string
	var oldestTime time.Time

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if oldestTime.IsZero() || info.ModTime().Before(oldestTime) {
			oldest = entry.Name()
			oldestTime = info.ModTime()
		}
	}

	return oldest, oldestTime, nil
}

// GetNewestWALFile returns information about the newest WAL file in the archive
func (s *BackupService) GetNewestWALFile() (string, time.Time, error) {
	entries, err := os.ReadDir(s.config.WALArchiveDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", time.Time{}, nil
		}
		return "", time.Time{}, err
	}

	var newest string
	var newestTime time.Time

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().After(newestTime) {
			newest = entry.Name()
			newestTime = info.ModTime()
		}
	}

	return newest, newestTime, nil
}

// CountWALFiles returns the number of WAL files in the archive
func (s *BackupService) CountWALFiles() (int, int64, error) {
	entries, err := os.ReadDir(s.config.WALArchiveDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}

	var count int
	var totalSize int64

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		count++
		totalSize += info.Size()
	}

	return count, totalSize, nil
}
