// //////////////////////////////////////////////////////////
//
// Description:
// The configuration utility for table-syncher.
//
// Created: 2026/02/26 by Claude Code based on Documents/syncdata-v2.md
// //////////////////////////////////////////////////////////
package tablesyncher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Location codes for config operations
const (
	LOC_CFG_LOAD  = "SHD_SYN_001"
	LOC_CFG_VALID = "SHD_SYN_002"
	LOC_CFG_PATH  = "SHD_SYN_003"
)

// SyncConfig holds all configuration for the sync service.
type SyncConfig struct {
	// Archive source (remote backup machine)
	ArchiveHost string `mapstructure:"archive_host"`
	ArchiveUser string `mapstructure:"archive_user"`
	ArchiveDir  string `mapstructure:"archive_dir"`
	ArchivePort int    `mapstructure:"archive_port"`

	// Local PostgreSQL connection
	PGHost     string `mapstructure:"pg_host"`
	PGPort     int    `mapstructure:"pg_port"`
	PGUser     string `mapstructure:"pg_user"`
	PGPassword string `mapstructure:"pg_password"`
	PGDatabase string `mapstructure:"pg_database"`

	// Sync settings
	DataSyncFreq int `mapstructure:"data_sync_freq"` // Frequency in seconds
	MetricFreq   int `mapstructure:"metric_freq"`    // Frequency in hours

	// Derived paths (computed after loading)
	StateFilePath string // <config_dir>/.syncdata_state.json
	PIDFilePath   string // <config_dir>/.syncdata.pid
	ConfigDir     string // Directory containing the config file
}

// LoadConfig loads configuration from the TOML file specified by DATA_SYNC_CONFIG env var.
func LoadConfig() (*SyncConfig, error) {
	configPath := os.Getenv("DATA_SYNC_CONFIG")
	if configPath == "" {
		return nil, fmt.Errorf("DATA_SYNC_CONFIG environment variable not set (%s) (SHD_02070554)", LOC_CFG_LOAD)
	}

	// Expand ~ to home directory
	configPath, err := expandPath(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to expand config path: %w (%s) (SHD_02070555)", err, LOC_CFG_PATH)
	}

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found: %s (%s) (SHD_02070556)", configPath, LOC_CFG_LOAD)
	}

	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("toml")

	// Set defaults
	v.SetDefault("archive_port", 22)
	v.SetDefault("pg_host", "127.0.0.1")
	v.SetDefault("pg_port", 5432)
	v.SetDefault("pg_user", "admin")
	v.SetDefault("data_sync_freq", 600)
	v.SetDefault("metric_freq", 24)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w (%s) (SHD_02070557)", configPath, err, LOC_CFG_LOAD)
	}

	// Allow environment variable overrides
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Map environment variables to config keys
	v.BindEnv("pg_host", "PG_HOST")
	v.BindEnv("pg_port", "PG_PORT")
	v.BindEnv("pg_user", "PG_USER_NAME")
	v.BindEnv("pg_password", "PG_PASSWORD")
	v.BindEnv("pg_database", "PG_DB_NAME")
	v.BindEnv("data_sync_freq", "DATA_SYNC_FREQ")
	v.BindEnv("metric_freq", "METRIC_FREQ")

	config := &SyncConfig{}
	if err := v.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w (%s) (SHD_02070558)", err, LOC_CFG_LOAD)
	}

	// Also check environment variables directly for PG credentials
	// (in case they weren't in the TOML file)
	if config.PGPassword == "" {
		config.PGPassword = os.Getenv("PG_PASSWORD")
	}
	if config.PGDatabase == "" {
		config.PGDatabase = os.Getenv("PG_DB_NAME")
	}
	if config.PGUser == "" && os.Getenv("PG_USER_NAME") != "" {
		config.PGUser = os.Getenv("PG_USER_NAME")
	}

	// Set derived paths
	config.ConfigDir = filepath.Dir(configPath)
	config.StateFilePath = filepath.Join(config.ConfigDir, ".syncdata_state.json")
	config.PIDFilePath = filepath.Join(config.ConfigDir, ".syncdata.pid")

	if err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}

// Validate checks that required configuration is present.
func (c *SyncConfig) Validate() error {
	// Archive settings
	if c.ArchiveHost == "" {
		return fmt.Errorf("archive_host is required in config (%s) (SHD_02070559)", LOC_CFG_VALID)
	}
	if c.ArchiveDir == "" {
		return fmt.Errorf("archive_dir is required in config (%s) (SHD_02070560)", LOC_CFG_VALID)
	}
	if c.ArchiveUser == "" {
		return fmt.Errorf("archive_user is required in config (%s) (SHD_02070561)", LOC_CFG_VALID)
	}

	// Database settings
	if c.PGPassword == "" {
		return fmt.Errorf("pg_password (or PG_PASSWORD env) is required (%s) (SHD_02070562)", LOC_CFG_VALID)
	}
	if c.PGDatabase == "" {
		return fmt.Errorf("pg_database (or PG_DB_NAME env) is required (%s) (SHD_02070563)", LOC_CFG_VALID)
	}

	// Sync settings validation
	if c.DataSyncFreq < 60 {
		return fmt.Errorf("data_sync_freq must be at least 60 seconds (%s) (SHD_02070564)", LOC_CFG_VALID)
	}
	if c.MetricFreq < 1 {
		return fmt.Errorf("metric_freq must be at least 1 hour (%s) (SHD_02070565)", LOC_CFG_VALID)
	}

	return nil
}

// ConnectionString returns a PostgreSQL connection string.
func (c *SyncConfig) ConnectionString() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable (SHD_02070566)",
		c.PGHost, c.PGPort, c.PGUser, c.PGPassword, c.PGDatabase)
}

// SSHAddress returns the SSH connection address (user@host:port).
func (c *SyncConfig) SSHAddress() string {
	return fmt.Sprintf("%s@%s:%d (SHD_02070567)", c.ArchiveUser, c.ArchiveHost, c.ArchivePort)
}

// expandPath expands ~ to the user's home directory.
func expandPath(path string) (string, error) {
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w (SHD_02070568)", err)
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, path[2:]), nil
	}
	return filepath.Abs(path)
}

/*
// getEnvOrDefault returns the environment variable value or a default.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvIntOrDefault returns the environment variable as int or a default.
func getEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
*/
