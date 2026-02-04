// Package pgbackup provides PostgreSQL backup management with WAL archiving
// and Point-in-Time Recovery (PITR) capabilities.
package pgbackup

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Location codes for config operations
const (
	LOC_CFG_LOAD  = "SHD_PGB_001"
	LOC_CFG_VALID = "SHD_PGB_002"
	LOC_CFG_PATH  = "SHD_PGB_003"
)

// BackupConfig holds all configuration for backup operations
type BackupConfig struct {
	// PostgreSQL connection
	PGHost     string
	PGPort     int
	PGUser     string
	PGPassword string
	PGDatabase string

	// Backup paths
	BackupBaseDir string // Root backup directory (from PG_BACKUP_DIR)
	BaseBackupDir string // Where base backups go ($PG_BACKUP_DIR/base)
	WALArchiveDir string // Where WAL files are archived ($PG_BACKUP_DIR/wal_archive)
	LogDir        string // Log directory ($PG_BACKUP_DIR/logs)
	ScriptsDir    string // Scripts directory ($PG_BACKUP_DIR/scripts)

	// Archive script path
	ArchiveScriptPath string

	// Retention settings
	RetainDays    int // Keep backups for N days (default: 7)
	RetainCount   int // Keep at least N backups (default: 3)
	RetainWALDays int // Keep WAL files for N days (default: 14)

	// Remote sync (optional - enabled when RemoteHost is set)
	RemoteHost string // Remote hostname/IP (PG_BACKUP_REMOTE_HOST)
	RemoteUser string // SSH username (PG_BACKUP_REMOTE_USER, default: current user)
	RemoteDir  string // Remote backup directory (PG_BACKUP_REMOTE_DIR, default: same as BackupBaseDir)
	RemotePort int    // SSH port (PG_BACKUP_REMOTE_PORT, default: 22)

	// PostgreSQL data directory (for recovery)
	PGDataDir string
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*BackupConfig, error) {
	backupDir := os.Getenv("PG_BACKUP_DIR")
	if backupDir == "" {
		return nil, fmt.Errorf("PG_BACKUP_DIR environment variable not set (%s)", LOC_CFG_LOAD)
	}

	// Expand ~ to home directory
	backupDir, err := expandPath(backupDir)
	if err != nil {
		return nil, fmt.Errorf("failed to expand backup dir path: %w (%s)", err, LOC_CFG_PATH)
	}

	config := &BackupConfig{
		PGHost:            getEnvOrDefault("PG_HOST", "127.0.0.1"),
		PGPort:            getEnvIntOrDefault("PG_PORT", 5432),
		PGUser:            os.Getenv("PG_USER_NAME"),
		PGPassword:        os.Getenv("PG_PASSWORD"),
		PGDatabase:        os.Getenv("PG_DB_NAME"),
		BackupBaseDir:     backupDir,
		BaseBackupDir:     filepath.Join(backupDir, "base"),
		WALArchiveDir:     filepath.Join(backupDir, "wal_archive"),
		LogDir:            filepath.Join(backupDir, "logs"),
		ScriptsDir:        filepath.Join(backupDir, "scripts"),
		ArchiveScriptPath: filepath.Join(backupDir, "scripts", "archive_wal.sh"),
		RetainDays:        getEnvIntOrDefault("PG_BACKUP_RETAIN_DAYS", 7),
		RetainCount:       getEnvIntOrDefault("PG_BACKUP_RETAIN_COUNT", 3),
		RetainWALDays:     getEnvIntOrDefault("PG_BACKUP_RETAIN_WAL_DAYS", 14),
		RemoteHost:        os.Getenv("PG_BACKUP_REMOTE_HOST"),
		RemoteUser:        getEnvOrDefault("PG_BACKUP_REMOTE_USER", ""),
		RemoteDir:         getEnvOrDefault("PG_BACKUP_REMOTE_DIR", ""),
		RemotePort:        getEnvIntOrDefault("PG_BACKUP_REMOTE_PORT", 22),
		PGDataDir:         os.Getenv("PGDATA"),
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}

// Validate checks that required configuration is present
func (c *BackupConfig) Validate() error {
	if c.PGUser == "" {
		return fmt.Errorf("PG_USER_NAME environment variable not set (%s)", LOC_CFG_VALID)
	}
	if c.PGPassword == "" {
		return fmt.Errorf("PG_PASSWORD environment variable not set (%s)", LOC_CFG_VALID)
	}
	if c.PGDatabase == "" {
		return fmt.Errorf("PG_DB_NAME environment variable not set (%s)", LOC_CFG_VALID)
	}
	if c.BackupBaseDir == "" {
		return fmt.Errorf("PG_BACKUP_DIR environment variable not set (%s)", LOC_CFG_VALID)
	}
	return nil
}

// ValidateForRestore checks additional requirements for restore operations
func (c *BackupConfig) ValidateForRestore() error {
	if err := c.Validate(); err != nil {
		return err
	}
	if c.PGDataDir == "" {
		return fmt.Errorf("PGDATA environment variable not set (required for restore) (%s)", LOC_CFG_VALID)
	}
	return nil
}

// ConnectionString returns a PostgreSQL connection string (without password for logging)
func (c *BackupConfig) ConnectionString() string {
	return fmt.Sprintf("host=%s port=%d user=%s dbname=%s",
		c.PGHost, c.PGPort, c.PGUser, c.PGDatabase)
}

// RemoteEnabled returns true if remote sync is configured
func (c *BackupConfig) RemoteEnabled() bool {
	return c.RemoteHost != ""
}

// RemoteBaseDir returns the remote backup base directory, defaulting to the local BackupBaseDir
func (c *BackupConfig) RemoteBaseDir() string {
	if c.RemoteDir != "" {
		return c.RemoteDir
	}
	return c.BackupBaseDir
}

// RemoteUserOrDefault returns the configured remote user, or the current OS user
func (c *BackupConfig) RemoteUserOrDefault() string {
	if c.RemoteUser != "" {
		return c.RemoteUser
	}
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return "root"
}

// expandPath expands ~ to the user's home directory
func expandPath(path string) (string, error) {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

// getEnvOrDefault returns the environment variable value or a default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvIntOrDefault returns the environment variable as int or a default
func getEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
