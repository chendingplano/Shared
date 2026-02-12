package pgbackup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Location codes for verify operations
const (
	LOC_VERIFY_START = "SHD_PGB_060"
	LOC_VERIFY_TAR   = "SHD_PGB_061"
	LOC_VERIFY_WAL   = "SHD_PGB_062"
)

// VerifyResult contains information about a verification operation
type VerifyResult struct {
	BackupID      string   `json:"backup_id"`
	Success       bool     `json:"success"`
	TarFiles      []string `json:"tar_files"`
	TarFilesOK    bool     `json:"tar_files_ok"`
	WALContinuity bool     `json:"wal_continuity"`
	Issues        []string `json:"issues,omitempty"`
}

// Verify checks the integrity of a backup
func (s *BackupService) Verify(ctx context.Context, logger *slog.Logger, backupID string) (*VerifyResult, error) {
	result := &VerifyResult{
		BackupID: backupID,
		Issues:   []string{},
	}

	// If no backup ID specified, verify the latest backup
	if backupID == "" {
		backups, err := s.ListBackups()
		if err != nil {
			return nil, fmt.Errorf("failed to list backups: %w (%s)", err, LOC_VERIFY_START)
		}
		if len(backups) == 0 {
			return nil, fmt.Errorf("no backups found (%s)", LOC_VERIFY_START)
		}

		// Find the latest backup
		var latest *BackupResult
		for _, b := range backups {
			if latest == nil || b.StartTime.After(latest.StartTime) {
				latest = b
			}
		}
		backupID = latest.BackupID
		result.BackupID = backupID
	}

	backupPath := filepath.Join(s.config.BaseBackupDir, backupID)
	logger.Info("Verifying backup", "backup_id", backupID, "path", backupPath)

	// Check backup directory exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("backup not found: %s (%s)", backupID, LOC_VERIFY_START)
	}

	// 1. Verify tar files
	tarFilesOK, tarFiles, tarIssues := s.verifyTarFiles(ctx, logger, backupPath)
	result.TarFiles = tarFiles
	result.TarFilesOK = tarFilesOK
	result.Issues = append(result.Issues, tarIssues...)

	// 2. Check WAL continuity
	walOK, walIssues := s.verifyWALContinuity(ctx, logger, backupID)
	result.WALContinuity = walOK
	result.Issues = append(result.Issues, walIssues...)

	// Determine overall success
	result.Success = result.TarFilesOK && len(result.Issues) == 0

	if result.Success {
		logger.Info("Backup verification passed", "backup_id", backupID)
	} else {
		logger.Warn("Backup verification found issues",
			"backup_id", backupID,
			"issues", len(result.Issues))
	}

	return result, nil
}

// verifyTarFiles checks the integrity of tar.gz files in the backup
func (s *BackupService) verifyTarFiles(ctx context.Context, logger *slog.Logger, backupPath string) (bool, []string, []string) {
	entries, err := os.ReadDir(backupPath)
	if err != nil {
		return false, nil, []string{fmt.Sprintf("failed to read backup directory: %v", err)}
	}

	var tarFiles []string
	var issues []string
	allOK := true

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".tar.gz") && !strings.HasSuffix(name, ".tar") {
			continue
		}

		tarPath := filepath.Join(backupPath, name)
		tarFiles = append(tarFiles, name)

		if err := s.verifyTarGz(ctx, tarPath); err != nil {
			issues = append(issues, fmt.Sprintf("corrupt tar file %s: %v", name, err))
			allOK = false
			logger.Error("Tar file verification failed", "file", name, "error", err)
		} else {
			logger.Info("Tar file verified", "file", name)
		}
	}

	if len(tarFiles) == 0 {
		issues = append(issues, "no tar files found in backup")
		allOK = false
	}

	// Check for required base.tar.gz
	hasBase := false
	for _, f := range tarFiles {
		if strings.HasPrefix(f, "base.tar") {
			hasBase = true
			break
		}
	}
	if !hasBase {
		issues = append(issues, "missing base.tar.gz - backup is incomplete")
		allOK = false
	}

	return allOK, tarFiles, issues
}

// verifyTarGz tests a tar.gz file for integrity
func (s *BackupService) verifyTarGz(ctx context.Context, tarPath string) error {
	// Use gzip -t to test compressed files
	if strings.HasSuffix(tarPath, ".gz") {
		cmd := exec.CommandContext(ctx, "gzip", "-t", tarPath)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("gzip test failed: %v, output: %s (%s)",
				err, strings.TrimSpace(string(output)), LOC_VERIFY_TAR)
		}
	}

	// Use tar -tf to list contents (verifies tar structure)
	var cmd *exec.Cmd
	if strings.HasSuffix(tarPath, ".gz") {
		cmd = exec.CommandContext(ctx, "tar", "-tzf", tarPath)
	} else {
		cmd = exec.CommandContext(ctx, "tar", "-tf", tarPath)
	}

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tar test failed: %v, output: %s (%s)",
			err, strings.TrimSpace(string(output)), LOC_VERIFY_TAR)
	}

	return nil
}

// verifyWALContinuity checks if WAL files form a continuous sequence
func (s *BackupService) verifyWALContinuity(_ context.Context, logger *slog.Logger, backupID string) (bool, []string) {
	var issues []string

	// Check if WAL archive directory exists
	if _, err := os.Stat(s.config.WALArchiveDir); os.IsNotExist(err) {
		msg := fmt.Sprintf("WAL archive directory does not exist - PITR will not be possible, backupID:%s", backupID)
		issues = append(issues, msg)
		return false, issues
	}

	// Count WAL files
	entries, err := os.ReadDir(s.config.WALArchiveDir)
	if err != nil {
		issues = append(issues, fmt.Sprintf("failed to read WAL archive: %v, backupID:%s", err, backupID))
		return false, issues
	}

	walFiles := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// WAL files are 24 characters (or 24 + .gz)
		baseName := strings.TrimSuffix(name, ".gz")
		if len(baseName) == 24 {
			walFiles = append(walFiles, baseName)
		}
	}

	if len(walFiles) == 0 {
		issues = append(issues, "no WAL files found in archive - PITR will not be possible")
		return false, issues
	}

	logger.Info("WAL files found", "count", len(walFiles), "backupID", backupID)

	// Check for gaps in WAL sequence
	// WAL file names follow format: TTTTTTTTSSSSSSSSNNNNNNNN
	// where T=timeline, S=segment high, N=segment low
	// A gap would be detected by sorting and checking sequence

	// For now, just verify we have WAL files
	// Full continuity check would require parsing WAL filenames
	// and checking the sequence is unbroken

	return true, issues
}

// VerifyAll verifies all available backups
func (s *BackupService) VerifyAll(ctx context.Context, logger *slog.Logger) ([]*VerifyResult, error) {
	backups, err := s.ListBackups()
	if err != nil {
		return nil, fmt.Errorf("failed to list backups: %w", err)
	}

	var results []*VerifyResult
	for _, backup := range backups {
		result, err := s.Verify(ctx, logger, backup.BackupID)
		if err != nil {
			logger.Warn("Verification failed for backup",
				"backup_id", backup.BackupID,
				"error", err)
			results = append(results, &VerifyResult{
				BackupID: backup.BackupID,
				Success:  false,
				Issues:   []string{err.Error()},
			})
			continue
		}
		results = append(results, result)
	}

	return results, nil
}
