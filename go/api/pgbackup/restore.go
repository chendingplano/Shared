package pgbackup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Location codes for restore operations
const (
	LOC_RESTORE_START   = "SHD_PGB_050"
	LOC_RESTORE_VALIDATE = "SHD_PGB_051"
	LOC_RESTORE_EXTRACT = "SHD_PGB_052"
	LOC_RESTORE_CONFIG  = "SHD_PGB_053"
	LOC_RESTORE_WAL     = "SHD_PGB_054"
)

// RestoreOptions configures a restore operation
type RestoreOptions struct {
	BackupID        string     // Specific backup to restore from
	TargetTime      *time.Time // Point-in-time recovery target (optional)
	TargetXID       string     // Recovery target transaction ID (optional)
	TargetName      string     // Recovery target named restore point (optional)
	TargetDirectory string     // Where to restore (defaults to PGDATA)
	DryRun          bool       // Just validate, don't actually restore
}

// RestoreResult contains information about a restore operation
type RestoreResult struct {
	Success      bool      `json:"success"`
	BackupUsed   string    `json:"backup_used"`
	RecoveredTo  time.Time `json:"recovered_to,omitempty"`
	WALFilesUsed int       `json:"wal_files_used"`
	TargetDir    string    `json:"target_dir"`
	ErrorMsg     string    `json:"error_msg,omitempty"`
}

// PrepareRestore validates and prepares for a restore operation
// IMPORTANT: PostgreSQL must be STOPPED before running Restore
func (s *BackupService) PrepareRestore(ctx context.Context, logger *slog.Logger, opts RestoreOptions) error {
	logger.Info("Preparing restore",
		"backup_id", opts.BackupID,
		"target_time", opts.TargetTime,
		"dry_run", opts.DryRun)

	// 1. Verify backup exists
	backupPath := filepath.Join(s.config.BaseBackupDir, opts.BackupID)
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("backup not found: %s (%s)", opts.BackupID, LOC_RESTORE_START)
	}

	// 2. Verify backup has required files (base.tar.gz at minimum)
	baseTar := filepath.Join(backupPath, "base.tar.gz")
	if _, err := os.Stat(baseTar); os.IsNotExist(err) {
		return fmt.Errorf("backup is incomplete (missing base.tar.gz): %s (%s)", opts.BackupID, LOC_RESTORE_VALIDATE)
	}

	// 3. Determine target directory
	targetDir := opts.TargetDirectory
	if targetDir == "" {
		targetDir = s.config.PGDataDir
	}
	if targetDir == "" {
		return fmt.Errorf("target directory not specified and PGDATA not set (%s)", LOC_RESTORE_START)
	}

	// 4. Check if target directory exists and has data
	if _, err := os.Stat(targetDir); err == nil {
		entries, _ := os.ReadDir(targetDir)
		if len(entries) > 0 {
			// Directory not empty - warn user
			logger.Warn("Target directory is not empty",
				"path", targetDir,
				"files", len(entries))
			if !opts.DryRun {
				return fmt.Errorf("target directory %s is not empty - back it up first or specify a different directory (%s)",
					targetDir, LOC_RESTORE_VALIDATE)
			}
		}
	}

	// 5. Check if PostgreSQL is running (it should be stopped)
	if s.isPostgreSQLRunning(ctx, logger) {
		return fmt.Errorf("PostgreSQL appears to be running - stop it before restore (%s)", LOC_RESTORE_START)
	}

	// 6. If target time specified, verify WAL files are available
	if opts.TargetTime != nil {
		if err := s.verifyWALAvailability(logger, opts.BackupID, *opts.TargetTime); err != nil {
			logger.Warn("WAL availability check", "warning", err)
		}
	}

	logger.Info("Restore preparation complete",
		"backup_id", opts.BackupID,
		"target_dir", targetDir)

	return nil
}

// Restore performs the actual restore operation
func (s *BackupService) Restore(ctx context.Context, logger *slog.Logger, opts RestoreOptions) (*RestoreResult, error) {
	result := &RestoreResult{
		BackupUsed: opts.BackupID,
	}

	// Validate first
	if err := s.PrepareRestore(ctx, logger, opts); err != nil {
		result.Success = false
		result.ErrorMsg = err.Error()
		return result, err
	}

	targetDir := opts.TargetDirectory
	if targetDir == "" {
		targetDir = s.config.PGDataDir
	}
	result.TargetDir = targetDir

	if opts.DryRun {
		logger.Info("Dry run - restore validated but not executed")
		result.Success = true
		return result, nil
	}

	backupPath := filepath.Join(s.config.BaseBackupDir, opts.BackupID)

	// 1. Create target directory if it doesn't exist
	if err := os.MkdirAll(targetDir, 0700); err != nil {
		result.Success = false
		result.ErrorMsg = fmt.Sprintf("failed to create target directory: %v", err)
		return result, fmt.Errorf("%s (%s)", result.ErrorMsg, LOC_RESTORE_EXTRACT)
	}

	// 2. Extract base backup
	logger.Info("Extracting base backup", "from", backupPath, "to", targetDir)
	if err := s.extractBackup(ctx, logger, backupPath, targetDir); err != nil {
		result.Success = false
		result.ErrorMsg = fmt.Sprintf("failed to extract backup: %v", err)
		return result, fmt.Errorf("%s (%s)", result.ErrorMsg, LOC_RESTORE_EXTRACT)
	}

	// 3. Create recovery configuration (PostgreSQL 12+)
	if err := s.createRecoveryConfig(logger, targetDir, opts); err != nil {
		result.Success = false
		result.ErrorMsg = fmt.Sprintf("failed to create recovery config: %v", err)
		return result, fmt.Errorf("%s (%s)", result.ErrorMsg, LOC_RESTORE_CONFIG)
	}

	result.Success = true
	if opts.TargetTime != nil {
		result.RecoveredTo = *opts.TargetTime
	}

	logger.Info("Restore complete - start PostgreSQL to begin recovery",
		"backup_used", opts.BackupID,
		"target_dir", targetDir,
		"target_time", opts.TargetTime)

	return result, nil
}

// extractBackup extracts the backup tar files to the target directory
func (s *BackupService) extractBackup(ctx context.Context, logger *slog.Logger, backupPath, targetDir string) error {
	// Find tar files in backup
	entries, err := os.ReadDir(backupPath)
	if err != nil {
		return fmt.Errorf("failed to read backup directory: %w", err)
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".tar.gz") && !strings.HasSuffix(entry.Name(), ".tar") {
			continue
		}

		tarPath := filepath.Join(backupPath, entry.Name())
		logger.Info("Extracting", "file", entry.Name())

		var cmd *exec.Cmd
		if strings.HasSuffix(entry.Name(), ".tar.gz") {
			// Compressed tar
			cmd = exec.CommandContext(ctx, "tar", "-xzf", tarPath, "-C", targetDir)
		} else {
			// Uncompressed tar
			cmd = exec.CommandContext(ctx, "tar", "-xf", tarPath, "-C", targetDir)
		}

		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to extract %s: %v, output: %s", entry.Name(), err, string(output))
		}
	}

	// Set proper permissions on data directory
	if err := os.Chmod(targetDir, 0700); err != nil {
		logger.Warn("Failed to set permissions on data directory", "error", err)
	}

	return nil
}

// createRecoveryConfig creates the recovery configuration files (PostgreSQL 12+)
func (s *BackupService) createRecoveryConfig(logger *slog.Logger, pgDataDir string, opts RestoreOptions) error {
	// For PostgreSQL 12+: create recovery.signal and set parameters in postgresql.auto.conf

	// 1. Create recovery.signal (empty file, signals recovery mode)
	signalPath := filepath.Join(pgDataDir, "recovery.signal")
	if err := os.WriteFile(signalPath, []byte{}, 0600); err != nil {
		return fmt.Errorf("failed to create recovery.signal: %w", err)
	}
	logger.Info("Created recovery.signal")

	// 2. Build recovery parameters
	var recoveryParams strings.Builder
	recoveryParams.WriteString("\n# Recovery configuration added by pgbackup\n")
	recoveryParams.WriteString("# Remove these lines after recovery is complete\n")

	// restore_command - how to fetch archived WAL files
	restoreCmd := fmt.Sprintf("gunzip -c %s/%%f.gz > %%p || cp %s/%%f %%p",
		s.config.WALArchiveDir, s.config.WALArchiveDir)
	recoveryParams.WriteString(fmt.Sprintf("restore_command = '%s'\n", restoreCmd))

	// Point-in-time recovery target
	if opts.TargetTime != nil {
		recoveryParams.WriteString(fmt.Sprintf("recovery_target_time = '%s'\n",
			opts.TargetTime.Format("2006-01-02 15:04:05-07")))
		logger.Info("Set recovery target time", "time", opts.TargetTime)
	}

	if opts.TargetXID != "" {
		recoveryParams.WriteString(fmt.Sprintf("recovery_target_xid = '%s'\n", opts.TargetXID))
		logger.Info("Set recovery target XID", "xid", opts.TargetXID)
	}

	if opts.TargetName != "" {
		recoveryParams.WriteString(fmt.Sprintf("recovery_target_name = '%s'\n", opts.TargetName))
		logger.Info("Set recovery target name", "name", opts.TargetName)
	}

	// What to do when recovery target is reached
	recoveryParams.WriteString("recovery_target_action = 'promote'\n")

	// 3. Append to postgresql.auto.conf
	autoConfPath := filepath.Join(pgDataDir, "postgresql.auto.conf")

	f, err := os.OpenFile(autoConfPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("failed to open postgresql.auto.conf: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(recoveryParams.String()); err != nil {
		return fmt.Errorf("failed to write recovery params: %w", err)
	}

	logger.Info("Updated postgresql.auto.conf with recovery parameters")
	return nil
}

// isPostgreSQLRunning checks if PostgreSQL is running
func (s *BackupService) isPostgreSQLRunning(ctx context.Context, logger *slog.Logger) bool {
	// Try to connect via pg_isready
	cmd := exec.CommandContext(ctx, "pg_isready",
		"-h", s.config.PGHost,
		"-p", fmt.Sprintf("%d", s.config.PGPort),
	)
	err := cmd.Run()
	if err == nil {
		logger.Info("PostgreSQL is running")
		return true
	}

	// Also check for postmaster.pid if PGDATA is set
	if s.config.PGDataDir != "" {
		pidFile := filepath.Join(s.config.PGDataDir, "postmaster.pid")
		if _, err := os.Stat(pidFile); err == nil {
			logger.Info("Found postmaster.pid, PostgreSQL may be running")
			return true
		}
	}

	return false
}

// verifyWALAvailability checks if WAL files are available for recovery to target time
func (s *BackupService) verifyWALAvailability(logger *slog.Logger, backupID string, targetTime time.Time) error {
	// Get backup info to find WAL start position
	backup, err := s.GetBackup(backupID)
	if err != nil {
		return fmt.Errorf("failed to get backup info: %w", err)
	}

	// Check if target time is after backup time
	if targetTime.Before(backup.StartTime) {
		return fmt.Errorf("target time %s is before backup start time %s",
			targetTime.Format(time.RFC3339), backup.StartTime.Format(time.RFC3339))
	}

	// Check WAL archive directory has files
	entries, err := os.ReadDir(s.config.WALArchiveDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("WAL archive directory does not exist")
		}
		return fmt.Errorf("failed to read WAL archive: %w", err)
	}

	walCount := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".gz") || len(entry.Name()) == 24 {
			walCount++
		}
	}

	if walCount == 0 {
		return fmt.Errorf("no WAL files found in archive - PITR may not be possible")
	}

	logger.Info("WAL archive status", "files", walCount)
	return nil
}
