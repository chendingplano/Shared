package pgbackup

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Location codes for backup operations
const (
	LOC_BACKUP_START    = "SHD_PGB_020"
	LOC_BACKUP_DIR      = "SHD_PGB_021"
	LOC_BACKUP_EXEC     = "SHD_PGB_022"
	LOC_BACKUP_MANIFEST = "SHD_PGB_023"
	LOC_BACKUP_SIZE     = "SHD_PGB_024"
)

// BackupResult contains information about a completed backup
type BackupResult struct {
	BackupID   string    `json:"backup_id"`
	BackupPath string    `json:"backup_path"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	SizeBytes  int64     `json:"size_bytes"`
	WALStart   string    `json:"wal_start,omitempty"`
	WALEnd     string    `json:"wal_end,omitempty"`
	Success    bool      `json:"success"`
	ErrorMsg   string    `json:"error_msg,omitempty"`
}

// BackupService provides backup operations
type BackupService struct {
	config *BackupConfig
	db     *sql.DB
}

// NewBackupService creates a new backup service
func NewBackupService(config *BackupConfig) *BackupService {
	return &BackupService{config: config}
}

// NewBackupServiceWithDB creates a new backup service with database connection
func NewBackupServiceWithDB(config *BackupConfig, db *sql.DB) *BackupService {
	return &BackupService{config: config, db: db}
}

// Initialize creates required directories and installs the WAL archive script
func (s *BackupService) Initialize(ctx context.Context, logger *slog.Logger) error {
	logger.Info("Initializing backup environment", "backup_dir", s.config.BackupBaseDir)

	// Create directories
	dirs := []string{
		s.config.BackupBaseDir,
		s.config.BaseBackupDir,
		s.config.WALArchiveDir,
		s.config.LogDir,
		s.config.ScriptsDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("failed to create directory %s: %w (%s)", dir, err, LOC_BACKUP_DIR)
		}
		logger.Info("Created directory", "path", dir)
	}

	// Install WAL archive script
	if err := s.installArchiveScript(logger); err != nil {
		return err
	}

	// Verify PostgreSQL configuration
	if s.db != nil {
		if err := s.verifyPostgreSQLConfig(ctx, logger); err != nil {
			logger.Warn("PostgreSQL configuration check failed", "error", err)
			logger.Info("You may need to configure PostgreSQL manually. See documentation for required settings.")
		}
	}

	logger.Info("Backup environment initialized successfully")
	return nil
}

// installArchiveScript creates the WAL archive shell script
func (s *BackupService) installArchiveScript(logger *slog.Logger) error {
	scriptContent := fmt.Sprintf(`#!/bin/bash
# archive_wal.sh - PostgreSQL WAL archive script
# Called by PostgreSQL: archive_command = '%s %%p %%f'
#
# Arguments:
#   $1 = Full path to WAL file (%%p)
#   $2 = WAL file name only (%%f)

set -euo pipefail

WAL_SOURCE="$1"
WAL_FILENAME="$2"

ARCHIVE_DIR="%s"
LOG_FILE="%s/wal_archive.log"

log() {
    echo "$(date '+%%Y-%%m-%%d %%H:%%M:%%S') - $1" >> "$LOG_FILE"
}

# Ensure directories exist
mkdir -p "$ARCHIVE_DIR"
mkdir -p "$(dirname "$LOG_FILE")"

# Check if already archived (idempotent)
DEST="$ARCHIVE_DIR/$WAL_FILENAME.gz"
if [ -f "$DEST" ]; then
    log "WAL file already archived: $WAL_FILENAME"
    exit 0
fi

# Archive the WAL file with compression
log "Archiving WAL file: $WAL_FILENAME"
TEMP_DEST="$DEST.tmp"

if gzip -c "$WAL_SOURCE" > "$TEMP_DEST"; then
    mv "$TEMP_DEST" "$DEST"
    SIZE=$(stat -f%%z "$DEST" 2>/dev/null || stat -c%%s "$DEST" 2>/dev/null || echo "unknown")
    log "Successfully archived: $WAL_FILENAME ($SIZE bytes)"
else
    rm -f "$TEMP_DEST"
    log "ERROR: Failed to archive $WAL_FILENAME"
    exit 1
fi

# Remote sync (non-blocking)
if [ -n "${PG_BACKUP_REMOTE_HOST:-}" ]; then
    REMOTE_USER="${PG_BACKUP_REMOTE_USER:-$(whoami)}"
    REMOTE_DIR="${PG_BACKUP_REMOTE_DIR:-$(dirname "$ARCHIVE_DIR")}"
    REMOTE_PORT="${PG_BACKUP_REMOTE_PORT:-22}"
    REMOTE_WAL_DIR="$REMOTE_DIR/wal_archive"
    # Create remote directory first (--mkpath not available on older rsync)
    ssh -p "$REMOTE_PORT" -o StrictHostKeyChecking=accept-new \
        "$REMOTE_USER@$PG_BACKUP_REMOTE_HOST" "mkdir -p $REMOTE_WAL_DIR" 2>/dev/null
    if rsync -az --timeout=30 \
        -e "ssh -p $REMOTE_PORT -o StrictHostKeyChecking=accept-new" \
        "$DEST" "$REMOTE_USER@$PG_BACKUP_REMOTE_HOST:$REMOTE_WAL_DIR/" 2>/dev/null; then
        log "Remote sync OK: $WAL_FILENAME"
    else
        log "WARNING: Remote sync failed for $WAL_FILENAME (will be caught up by pgbackup sync)"
    fi
fi

exit 0
`, s.config.ArchiveScriptPath, s.config.WALArchiveDir, s.config.LogDir)

	if err := os.WriteFile(s.config.ArchiveScriptPath, []byte(scriptContent), 0755); err != nil {
		return fmt.Errorf("failed to write archive script: %w (%s)", err, LOC_BACKUP_DIR)
	}

	logger.Info("Installed WAL archive script", "path", s.config.ArchiveScriptPath)
	return nil
}

// verifyPostgreSQLConfig checks if PostgreSQL is configured for WAL archiving
func (s *BackupService) verifyPostgreSQLConfig(ctx context.Context, logger *slog.Logger) error {
	checks := []struct {
		setting      string
		requiredVal  string
		description  string
		configureSQL string
	}{
		{
			setting:      "wal_level",
			requiredVal:  "replica",
			description:  "WAL level must be 'replica' or 'logical' for archiving",
			configureSQL: "ALTER SYSTEM SET wal_level = 'replica';",
		},
		{
			setting:      "archive_mode",
			requiredVal:  "on",
			description:  "Archive mode must be enabled",
			configureSQL: "ALTER SYSTEM SET archive_mode = 'on';",
		},
	}

	var configNeeded []string

	for _, check := range checks {
		var value string
		err := s.db.QueryRowContext(ctx,
			"SELECT setting FROM pg_settings WHERE name = $1",
			check.setting,
		).Scan(&value)
		if err != nil {
			return fmt.Errorf("failed to check %s: %w", check.setting, err)
		}

		logger.Info("PostgreSQL setting", "name", check.setting, "value", value)

		if check.setting == "wal_level" && (value != "replica" && value != "logical") {
			configNeeded = append(configNeeded, check.configureSQL)
			logger.Warn(check.description, "current", value, "required", check.requiredVal)
		} else if check.setting != "wal_level" && value != check.requiredVal {
			configNeeded = append(configNeeded, check.configureSQL)
			logger.Warn(check.description, "current", value, "required", check.requiredVal)
		}
	}

	// Check archive_command
	var archiveCmd string
	err := s.db.QueryRowContext(ctx,
		"SELECT setting FROM pg_settings WHERE name = 'archive_command'",
	).Scan(&archiveCmd)
	if err != nil {
		return fmt.Errorf("failed to check archive_command: %w", err)
	}

	if archiveCmd == "" || archiveCmd == "(disabled)" {
		configSQL := fmt.Sprintf("ALTER SYSTEM SET archive_command = '%s %%p %%f';", s.config.ArchiveScriptPath)
		configNeeded = append(configNeeded, configSQL)
		logger.Warn("archive_command not configured", "script_path", s.config.ArchiveScriptPath)
	} else {
		logger.Info("archive_command configured", "value", archiveCmd)
	}

	if len(configNeeded) > 0 {
		logger.Info("PostgreSQL configuration needed. Run the following commands as superuser:")
		for _, sql := range configNeeded {
			logger.Info("  " + sql)
		}
		logger.Info("  SELECT pg_reload_conf(); -- Or restart PostgreSQL")
		return fmt.Errorf("PostgreSQL configuration incomplete")
	}

	return nil
}

// PerformBaseBackup executes pg_basebackup to create a full backup
func (s *BackupService) PerformBaseBackup(ctx context.Context, logger *slog.Logger) (*BackupResult, error) {
	result := &BackupResult{
		BackupID:  time.Now().Format("20060102_150405"),
		StartTime: time.Now(),
	}

	// Create backup directory with timestamp
	backupDir := filepath.Join(s.config.BaseBackupDir, result.BackupID)
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		result.Success = false
		result.ErrorMsg = fmt.Sprintf("failed to create backup dir: %v", err)
		return result, fmt.Errorf("%s (%s)", result.ErrorMsg, LOC_BACKUP_DIR)
	}

	result.BackupPath = backupDir
	logger.Info("Starting base backup",
		"backup_id", result.BackupID,
		"path", backupDir,
		"host", s.config.PGHost,
		"port", s.config.PGPort)

	// Build pg_basebackup command
	// -D: destination directory
	// -F: format (t = tar)
	// -X: include WAL files (stream = stream during backup)
	// -P: show progress
	// -v: verbose
	// -z: compress (gzip)
	// --checkpoint=fast: start backup immediately
	cmd := exec.CommandContext(ctx, "pg_basebackup",
		"-h", s.config.PGHost,
		"-p", fmt.Sprintf("%d", s.config.PGPort),
		"-U", s.config.PGUser,
		"-D", backupDir,
		"-Ft",                // tar format
		"-Xs",                // stream WAL
		"-P",                 // progress
		"-v",                 // verbose
		"-z",                 // gzip compression
		"--checkpoint=fast",  // don't wait for checkpoint
		"--label", fmt.Sprintf("backup_%s", result.BackupID),
	)

	// Set password via environment
	cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", s.config.PGPassword))

	output, err := cmd.CombinedOutput()
	result.EndTime = time.Now()

	if err != nil {
		result.Success = false
		result.ErrorMsg = fmt.Sprintf("pg_basebackup failed: %v", err)
		logger.Error("Base backup failed",
			"error", err,
			"output", string(output),
			"backup_id", result.BackupID)

		// Clean up failed backup directory
		os.RemoveAll(backupDir)
		return result, fmt.Errorf("%s (%s)", result.ErrorMsg, LOC_BACKUP_EXEC)
	}

	// Calculate backup size
	size, err := s.calculateDirSize(backupDir)
	if err != nil {
		logger.Warn("Failed to calculate backup size", "error", err)
	}
	result.SizeBytes = size
	result.Success = true

	// Write backup manifest
	if err := s.writeBackupManifest(result); err != nil {
		logger.Warn("Failed to write backup manifest", "error", err)
	}

	logger.Info("Base backup completed successfully",
		"backup_id", result.BackupID,
		"duration", result.EndTime.Sub(result.StartTime).Round(time.Second),
		"size_mb", float64(result.SizeBytes)/(1024*1024))

	// Sync to remote if configured (non-blocking: failures are logged as warnings)
	if s.config.RemoteEnabled() {
		syncResult := s.SyncBaseBackup(ctx, logger, result.BackupID)
		if !syncResult.Success {
			logger.Warn("Base backup completed locally but remote sync failed. Run 'pgbackup sync' to retry.",
				"backup_id", result.BackupID)
		}
	}

	return result, nil
}

// writeBackupManifest creates a JSON manifest file for the backup
func (s *BackupService) writeBackupManifest(result *BackupResult) error {
	manifestPath := filepath.Join(result.BackupPath, "pgbackup_manifest.json")

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w (%s)", err, LOC_BACKUP_MANIFEST)
	}

	if err := os.WriteFile(manifestPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write manifest: %w (%s)", err, LOC_BACKUP_MANIFEST)
	}

	return nil
}

// calculateDirSize calculates the total size of a directory
func (s *BackupService) calculateDirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("failed to calculate dir size: %w (%s)", err, LOC_BACKUP_SIZE)
	}
	return size, nil
}

// ListBackups returns all available backups
func (s *BackupService) ListBackups() ([]*BackupResult, error) {
	entries, err := os.ReadDir(s.config.BaseBackupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*BackupResult{}, nil
		}
		return nil, fmt.Errorf("failed to read backup directory: %w", err)
	}

	var backups []*BackupResult
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		manifestPath := filepath.Join(s.config.BaseBackupDir, entry.Name(), "pgbackup_manifest.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			// If no manifest, create a basic result from directory info
			info, err := entry.Info()
			if err != nil {
				continue
			}
			backups = append(backups, &BackupResult{
				BackupID:   entry.Name(),
				BackupPath: filepath.Join(s.config.BaseBackupDir, entry.Name()),
				StartTime:  info.ModTime(),
				Success:    true,
			})
			continue
		}

		var result BackupResult
		if err := json.Unmarshal(data, &result); err != nil {
			continue
		}
		backups = append(backups, &result)
	}

	return backups, nil
}

// GetBackup retrieves a specific backup by ID
func (s *BackupService) GetBackup(backupID string) (*BackupResult, error) {
	backupPath := filepath.Join(s.config.BaseBackupDir, backupID)
	manifestPath := filepath.Join(backupPath, "pgbackup_manifest.json")

	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("backup not found: %s", backupID)
	}

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		// Return basic info if no manifest
		info, err := os.Stat(backupPath)
		if err != nil {
			return nil, fmt.Errorf("failed to stat backup: %w", err)
		}
		return &BackupResult{
			BackupID:   backupID,
			BackupPath: backupPath,
			StartTime:  info.ModTime(),
			Success:    true,
		}, nil
	}

	var result BackupResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	return &result, nil
}
