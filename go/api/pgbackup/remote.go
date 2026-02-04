package pgbackup

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
)

// Location codes for remote sync operations
const (
	LOC_REMOTE_SYNC     = "SHD_PGB_040"
	LOC_REMOTE_BACKUP   = "SHD_PGB_041"
	LOC_REMOTE_WAL      = "SHD_PGB_042"
	LOC_REMOTE_SYNC_ALL = "SHD_PGB_043"
	LOC_REMOTE_RSYNC    = "SHD_PGB_044"
)

// SyncResult contains information about a remote sync operation
type SyncResult struct {
	Success     bool
	FilesCount  int
	BytesSent   int64
	ErrorMsg    string
	Destination string
}

// SyncBaseBackup rsyncs a single base backup directory to the remote host.
// Failures are logged as warnings and do not return errors.
func (s *BackupService) SyncBaseBackup(ctx context.Context, logger *slog.Logger, backupID string) *SyncResult {
	if !s.config.RemoteEnabled() {
		return &SyncResult{Success: true}
	}

	src := filepath.Join(s.config.BaseBackupDir, backupID) + "/"
	remoteBase := filepath.Join(s.config.RemoteBaseDir(), "base", backupID) + "/"
	dest := fmt.Sprintf("%s@%s:%s", s.config.RemoteUserOrDefault(), s.config.RemoteHost, remoteBase)

	logger.Info("Syncing base backup to remote", "backup_id", backupID, "destination", dest)

	output, err := s.runRsync(ctx, src, dest)
	result := &SyncResult{Destination: dest}
	if err != nil {
		result.Success = false
		result.ErrorMsg = fmt.Sprintf("rsync failed: %v", err)
		logger.Warn("Remote sync failed for base backup",
			"backup_id", backupID,
			"error", err,
			"output", output,
			"location", LOC_REMOTE_BACKUP)
	} else {
		result.Success = true
		logger.Info("Remote sync completed for base backup", "backup_id", backupID)
	}

	return result
}

// SyncWALFile rsyncs a single WAL archive file to the remote host.
// Failures are logged as warnings and do not return errors.
func (s *BackupService) SyncWALFile(ctx context.Context, logger *slog.Logger, walFilename string) *SyncResult {
	if !s.config.RemoteEnabled() {
		return &SyncResult{Success: true}
	}

	src := filepath.Join(s.config.WALArchiveDir, walFilename)
	remoteWALDir := filepath.Join(s.config.RemoteBaseDir(), "wal_archive") + "/"
	dest := fmt.Sprintf("%s@%s:%s", s.config.RemoteUserOrDefault(), s.config.RemoteHost, remoteWALDir)

	logger.Debug("Syncing WAL file to remote", "wal_file", walFilename, "destination", dest)

	output, err := s.runRsync(ctx, src, dest)
	result := &SyncResult{Destination: dest}
	if err != nil {
		result.Success = false
		result.ErrorMsg = fmt.Sprintf("rsync failed: %v", err)
		logger.Warn("Remote sync failed for WAL file",
			"wal_file", walFilename,
			"error", err,
			"output", output,
			"location", LOC_REMOTE_WAL)
	} else {
		result.Success = true
		logger.Debug("Remote sync completed for WAL file", "wal_file", walFilename)
	}

	return result
}

// SyncAll rsyncs the entire backup directory (base/ and wal_archive/) to the remote host.
// Returns an error only if remote is not configured.
func (s *BackupService) SyncAll(ctx context.Context, logger *slog.Logger) (*SyncResult, error) {
	if !s.config.RemoteEnabled() {
		return nil, fmt.Errorf("remote sync not configured: set PG_BACKUP_REMOTE_HOST (%s)", LOC_REMOTE_SYNC_ALL)
	}

	remoteDir := s.config.RemoteBaseDir() + "/"
	dest := fmt.Sprintf("%s@%s:%s", s.config.RemoteUserOrDefault(), s.config.RemoteHost, remoteDir)

	logger.Info("Syncing all backups to remote",
		"source", s.config.BackupBaseDir,
		"destination", dest)

	// Sync base backups
	baseSrc := s.config.BaseBackupDir + "/"
	baseDest := fmt.Sprintf("%s@%s:%s", s.config.RemoteUserOrDefault(), s.config.RemoteHost,
		filepath.Join(s.config.RemoteBaseDir(), "base")+"/")

	logger.Info("Syncing base backups...", "source", baseSrc)
	output, err := s.runRsync(ctx, baseSrc, baseDest)
	if err != nil {
		logger.Error("Failed to sync base backups",
			"error", err,
			"output", output,
			"location", LOC_REMOTE_SYNC_ALL)
		return &SyncResult{
			Success:     false,
			ErrorMsg:    fmt.Sprintf("base backup sync failed: %v", err),
			Destination: dest,
		}, nil
	}
	logger.Info("Base backups synced successfully")

	// Sync WAL archive
	walSrc := s.config.WALArchiveDir + "/"
	walDest := fmt.Sprintf("%s@%s:%s", s.config.RemoteUserOrDefault(), s.config.RemoteHost,
		filepath.Join(s.config.RemoteBaseDir(), "wal_archive")+"/")

	logger.Info("Syncing WAL archives...", "source", walSrc)
	output, err = s.runRsync(ctx, walSrc, walDest)
	if err != nil {
		logger.Error("Failed to sync WAL archives",
			"error", err,
			"output", output,
			"location", LOC_REMOTE_SYNC_ALL)
		return &SyncResult{
			Success:     false,
			ErrorMsg:    fmt.Sprintf("WAL archive sync failed: %v", err),
			Destination: dest,
		}, nil
	}
	logger.Info("WAL archives synced successfully")

	return &SyncResult{
		Success:     true,
		Destination: dest,
	}, nil
}

// ensureRemoteDir creates the remote directory via SSH before rsync.
// This replaces rsync's --mkpath which is not available on older rsync versions.
func (s *BackupService) ensureRemoteDir(ctx context.Context, dest string) error {
	// dest format: user@host:/path/to/dir/
	parts := strings.SplitN(dest, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid remote dest format: %s", dest)
	}
	remoteDir := parts[1]

	cmd := exec.CommandContext(ctx, "ssh",
		"-p", fmt.Sprintf("%d", s.config.RemotePort),
		"-o", "StrictHostKeyChecking=accept-new",
		parts[0],
		fmt.Sprintf("mkdir -p %s", remoteDir),
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create remote dir %s: %w: %s", remoteDir, err, stderr.String())
	}
	return nil
}

// runRsync executes an rsync command with the configured SSH options.
// Returns the combined output and any error.
func (s *BackupService) runRsync(ctx context.Context, src, dest string) (string, error) {
	// Ensure remote directory exists (replaces --mkpath)
	if err := s.ensureRemoteDir(ctx, dest); err != nil {
		return "", fmt.Errorf("%w (%s)", err, LOC_REMOTE_RSYNC)
	}

	sshCmd := fmt.Sprintf("ssh -p %d -o StrictHostKeyChecking=accept-new", s.config.RemotePort)

	args := []string{
		"-az",
		"--timeout=30",
		"-e", sshCmd,
		src,
		dest,
	}

	cmd := exec.CommandContext(ctx, "rsync", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := strings.TrimSpace(stdout.String() + "\n" + stderr.String())

	if err != nil {
		return output, fmt.Errorf("%w: %s (%s)", err, output, LOC_REMOTE_RSYNC)
	}

	return output, nil
}
