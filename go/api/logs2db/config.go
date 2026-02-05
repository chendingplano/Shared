// Package logs2db monitors log files and loads their contents into a PostgreSQL database.
package logs2db

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

// Location codes for config operations
const (
	LOC_CFG_LOAD  = "SHD_L2D_001"
	LOC_CFG_VALID = "SHD_L2D_002"
	LOC_CFG_PATH  = "SHD_L2D_003"
)

// Log2DBConfig holds all configuration parsed from the TOML file and environment variables.
type Log2DBConfig struct {
	// From TOML
	LogFileDir     string            `mapstructure:"log_file_dir"`
	DBTableName    string            `mapstructure:"db_table_name"`
	LogEntryFormat string            `mapstructure:"log_entry_format"`
	SyncFreqSec    int               `mapstructure:"sync_freq_in_secon"`
	JSONMapping    map[string]string `mapstructure:"json-mapping"`

	// From environment variables
	PGHost     string
	PGPort     int
	PGUser     string
	PGPassword string
	PGDatabase string

	// Derived paths
	StateFilePath string // <LogFileDir>/.log2db_state.json
	PIDFilePath   string // <LogFileDir>/.log2db.pid
}

// LoadConfig reads the LOG2DB_CONFIG env var, parses the TOML file via Viper,
// merges with PG_* env vars, sets defaults, and validates.
func LoadConfig() (*Log2DBConfig, error) {
	configPath := os.Getenv("LOG2DB_CONFIG")
	if configPath == "" {
		return nil, fmt.Errorf("LOG2DB_CONFIG environment variable not set (%s)", LOC_CFG_LOAD)
	}

	// Expand ~ to home directory
	configPath, err := expandPath(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to expand config path: %w (%s)", err, LOC_CFG_PATH)
	}

	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("toml")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w (%s)", configPath, err, LOC_CFG_LOAD)
	}

	config := &Log2DBConfig{
		LogFileDir:     v.GetString("log_file_dir"),
		DBTableName:    v.GetString("db_table_name"),
		LogEntryFormat: v.GetString("log_entry_format"),
		SyncFreqSec:    v.GetInt("sync_freq_in_secon"),
		JSONMapping:    v.GetStringMapString("json-mapping"),

		PGHost:     getEnvOrDefault("PG_HOST", "127.0.0.1"),
		PGPort:     getEnvIntOrDefault("PG_PORT", 5432),
		PGUser:     os.Getenv("PG_USER_NAME"),
		PGPassword: os.Getenv("PG_PASSWORD"),
		PGDatabase: os.Getenv("PG_DB_NAME"),
	}

	// Defaults
	if config.SyncFreqSec <= 0 {
		config.SyncFreqSec = 10
	}

	// Expand log file dir
	config.LogFileDir, err = expandPath(config.LogFileDir)
	if err != nil {
		return nil, fmt.Errorf("failed to expand log_file_dir: %w (%s)", err, LOC_CFG_PATH)
	}

	// Derived paths
	config.StateFilePath = filepath.Join(config.LogFileDir, ".log2db_state.json")
	config.PIDFilePath = filepath.Join(config.LogFileDir, ".log2db.pid")

	if err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}

// Validate checks that required configuration is present and paths exist.
func (c *Log2DBConfig) Validate() error {
	if c.LogFileDir == "" {
		return fmt.Errorf("log_file_dir is required in config (%s)", LOC_CFG_VALID)
	}
	if c.DBTableName == "" {
		return fmt.Errorf("db_table_name is required in config (%s)", LOC_CFG_VALID)
	}
	if c.LogEntryFormat == "" {
		return fmt.Errorf("log_entry_format is required in config (%s)", LOC_CFG_VALID)
	}
	if c.PGUser == "" {
		return fmt.Errorf("PG_USER_NAME environment variable not set (%s)", LOC_CFG_VALID)
	}
	if c.PGDatabase == "" {
		return fmt.Errorf("PG_DB_NAME environment variable not set (%s)", LOC_CFG_VALID)
	}

	// Verify log file directory exists
	info, err := os.Stat(c.LogFileDir)
	if err != nil {
		return fmt.Errorf("log_file_dir does not exist: %s (%s)", c.LogFileDir, LOC_CFG_VALID)
	}
	if !info.IsDir() {
		return fmt.Errorf("log_file_dir is not a directory: %s (%s)", c.LogFileDir, LOC_CFG_VALID)
	}

	return nil
}

// ConnectionString returns a PostgreSQL connection string.
func (c *Log2DBConfig) ConnectionString() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		c.PGHost, c.PGPort, c.PGUser, c.PGPassword, c.PGDatabase)
}

// expandPath expands ~ to the user's home directory and resolves relative paths.
func expandPath(path string) (string, error) {
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(home, path[1:])
	}
	return filepath.Abs(path)
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getEnvIntOrDefault(key string, defaultValue int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultValue
}
