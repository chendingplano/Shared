package pgbackup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"time"
)

// Location codes for status operations
const (
	LOC_STATUS_START = "SHD_PGB_070"
	LOC_STATUS_PG    = "SHD_PGB_071"
)

// BackupStatus contains comprehensive status information
type BackupStatus struct {
	// Configuration
	BackupDir     string `json:"backup_dir"`
	WALArchiveDir string `json:"wal_archive_dir"`

	// Backup counts
	TotalBackups  int   `json:"total_backups"`
	TotalSizeBytes int64 `json:"total_size_bytes"`

	// Latest backup info
	LatestBackupID   string    `json:"latest_backup_id,omitempty"`
	LatestBackupTime time.Time `json:"latest_backup_time,omitempty"`
	LatestBackupSize int64     `json:"latest_backup_size,omitempty"`

	// Oldest backup info
	OldestBackupID   string    `json:"oldest_backup_id,omitempty"`
	OldestBackupTime time.Time `json:"oldest_backup_time,omitempty"`

	// WAL archive info
	WALFileCount int   `json:"wal_file_count"`
	WALSizeBytes int64 `json:"wal_size_bytes"`
	OldestWAL    string `json:"oldest_wal,omitempty"`
	NewestWAL    string `json:"newest_wal,omitempty"`

	// Recovery window
	RecoveryWindowStart time.Time `json:"recovery_window_start,omitempty"`
	RecoveryWindowEnd   time.Time `json:"recovery_window_end,omitempty"`

	// PostgreSQL configuration
	PGConfigured    bool   `json:"pg_configured"`
	WALLevel        string `json:"wal_level,omitempty"`
	ArchiveMode     string `json:"archive_mode,omitempty"`
	ArchiveCommand  string `json:"archive_command,omitempty"`

	// Retention settings
	RetainDays  int `json:"retain_days"`
	RetainCount int `json:"retain_count"`

	// All backups
	Backups []*BackupResult `json:"backups"`
}

// GetStatus returns comprehensive backup status information
func (s *BackupService) GetStatus(ctx context.Context, logger *slog.Logger) (*BackupStatus, error) {
	status := &BackupStatus{
		BackupDir:     s.config.BackupBaseDir,
		WALArchiveDir: s.config.WALArchiveDir,
		RetainDays:    s.config.RetainDays,
		RetainCount:   s.config.RetainCount,
		Backups:       []*BackupResult{},
	}

	// List all backups
	backups, err := s.ListBackups()
	if err != nil {
		logger.Warn("Failed to list backups", "error", err)
	} else {
		status.Backups = backups
		status.TotalBackups = len(backups)

		// Sort by time (newest first)
		sort.Slice(backups, func(i, j int) bool {
			return backups[i].StartTime.After(backups[j].StartTime)
		})

		// Calculate totals and find latest/oldest
		for _, b := range backups {
			status.TotalSizeBytes += b.SizeBytes
		}

		if len(backups) > 0 {
			latest := backups[0]
			status.LatestBackupID = latest.BackupID
			status.LatestBackupTime = latest.StartTime
			status.LatestBackupSize = latest.SizeBytes

			oldest := backups[len(backups)-1]
			status.OldestBackupID = oldest.BackupID
			status.OldestBackupTime = oldest.StartTime

			// Recovery window starts at oldest backup
			status.RecoveryWindowStart = oldest.StartTime
		}
	}

	// WAL archive info
	walCount, walSize, err := s.CountWALFiles()
	if err != nil {
		logger.Warn("Failed to count WAL files", "error", err)
	} else {
		status.WALFileCount = walCount
		status.WALSizeBytes = walSize
	}

	oldestWAL, _, err := s.GetOldestWALFile()
	if err == nil && oldestWAL != "" {
		status.OldestWAL = oldestWAL
	}

	newestWAL, newestTime, err := s.GetNewestWALFile()
	if err == nil && newestWAL != "" {
		status.NewestWAL = newestWAL
		status.RecoveryWindowEnd = newestTime
	}

	// Check PostgreSQL configuration if we have a database connection
	if s.db != nil {
		status.PGConfigured = true
		s.getPGSettings(ctx, logger, status)
	}

	return status, nil
}

// getPGSettings retrieves PostgreSQL configuration settings
func (s *BackupService) getPGSettings(ctx context.Context, logger *slog.Logger, status *BackupStatus) {
	settings := []struct {
		name   string
		target *string
	}{
		{"wal_level", &status.WALLevel},
		{"archive_mode", &status.ArchiveMode},
		{"archive_command", &status.ArchiveCommand},
	}

	for _, setting := range settings {
		var value string
		err := s.db.QueryRowContext(ctx,
			"SELECT setting FROM pg_settings WHERE name = $1",
			setting.name,
		).Scan(&value)
		if err != nil {
			logger.Warn("Failed to get PostgreSQL setting",
				"setting", setting.name,
				"error", err)
			continue
		}
		*setting.target = value
	}
}

// PrintStatus outputs human-readable status information
func (s *BackupService) PrintStatus(ctx context.Context, logger *slog.Logger) error {
	status, err := s.GetStatus(ctx, logger)
	if err != nil {
		return err
	}

	fmt.Println("=== PostgreSQL Backup Status ===")
	fmt.Println()

	// Configuration
	fmt.Println("Configuration:")
	fmt.Printf("  Backup Directory:     %s\n", status.BackupDir)
	fmt.Printf("  WAL Archive Directory: %s\n", status.WALArchiveDir)
	fmt.Printf("  Retention Policy:     %d days, minimum %d backups\n",
		status.RetainDays, status.RetainCount)
	fmt.Println()

	// Backups summary
	fmt.Println("Backups:")
	fmt.Printf("  Total Backups:        %d\n", status.TotalBackups)
	fmt.Printf("  Total Size:           %.2f MB\n", float64(status.TotalSizeBytes)/(1024*1024))
	if status.LatestBackupID != "" {
		fmt.Printf("  Latest Backup:        %s (%s ago)\n",
			status.LatestBackupID,
			formatDuration(time.Since(status.LatestBackupTime)))
		fmt.Printf("  Latest Backup Size:   %.2f MB\n", float64(status.LatestBackupSize)/(1024*1024))
	}
	if status.OldestBackupID != "" {
		fmt.Printf("  Oldest Backup:        %s (%s ago)\n",
			status.OldestBackupID,
			formatDuration(time.Since(status.OldestBackupTime)))
	}
	fmt.Println()

	// WAL archive
	fmt.Println("WAL Archive:")
	fmt.Printf("  WAL Files:            %d\n", status.WALFileCount)
	fmt.Printf("  WAL Size:             %.2f MB\n", float64(status.WALSizeBytes)/(1024*1024))
	if status.OldestWAL != "" {
		fmt.Printf("  Oldest WAL:           %s\n", status.OldestWAL)
	}
	if status.NewestWAL != "" {
		fmt.Printf("  Newest WAL:           %s\n", status.NewestWAL)
	}
	fmt.Println()

	// Recovery window
	if !status.RecoveryWindowStart.IsZero() {
		fmt.Println("Recovery Window:")
		fmt.Printf("  From:                 %s\n", status.RecoveryWindowStart.Format(time.RFC3339))
		if !status.RecoveryWindowEnd.IsZero() {
			fmt.Printf("  To:                   %s\n", status.RecoveryWindowEnd.Format(time.RFC3339))
		} else {
			fmt.Printf("  To:                   now (continuous archiving)\n")
		}
		fmt.Println()
	}

	// PostgreSQL configuration (if available)
	if status.PGConfigured {
		fmt.Println("PostgreSQL Configuration:")
		fmt.Printf("  wal_level:            %s\n", status.WALLevel)
		fmt.Printf("  archive_mode:         %s\n", status.ArchiveMode)
		if status.ArchiveCommand != "" && status.ArchiveCommand != "(disabled)" {
			fmt.Printf("  archive_command:      (configured)\n")
		} else {
			fmt.Printf("  archive_command:      NOT CONFIGURED\n")
		}
		fmt.Println()
	}

	// Backup list
	if len(status.Backups) > 0 {
		fmt.Println("Available Backups:")
		for _, b := range status.Backups {
			successMark := "OK"
			if !b.Success {
				successMark = "FAILED"
			}
			fmt.Printf("  %s  %s  %.2f MB  [%s]\n",
				b.BackupID,
				b.StartTime.Format("2006-01-02 15:04:05"),
				float64(b.SizeBytes)/(1024*1024),
				successMark)
		}
	}

	return nil
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%.1f hours", d.Hours())
	}
	return fmt.Sprintf("%.1f days", d.Hours()/24)
}

// CheckDiskSpace checks if there's enough disk space for a new backup
func (s *BackupService) CheckDiskSpace(ctx context.Context, logger *slog.Logger) error {
	// Get disk space info for backup directory
	var stat os.FileInfo
	var err error

	if stat, err = os.Stat(s.config.BackupBaseDir); err != nil {
		if os.IsNotExist(err) {
			// Directory doesn't exist yet, that's OK
			return nil
		}
		return fmt.Errorf("failed to stat backup directory: %w", err)
	}

	if !stat.IsDir() {
		return fmt.Errorf("backup path is not a directory: %s", s.config.BackupBaseDir)
	}

	// Note: Getting actual free disk space requires platform-specific code
	// For now, we just verify the directory exists and is writable
	testFile := fmt.Sprintf("%s/.pgbackup_test_%d", s.config.BackupBaseDir, time.Now().UnixNano())
	f, err := os.Create(testFile)
	if err != nil {
		return fmt.Errorf("backup directory is not writable: %w", err)
	}
	f.Close()
	os.Remove(testFile)

	logger.Info("Backup directory is writable", "path", s.config.BackupBaseDir)
	return nil
}
