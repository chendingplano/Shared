package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/autotester"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/chendingplano/shared/go/api/goose"
	"github.com/chendingplano/shared/go/api/loggerutil"
	sharedtesters "github.com/chendingplano/shared/go/api/testers"
	"github.com/spf13/viper"
)

func main() {
	// Define command-line flags
	purposes := flag.String("purpose", "", "Comma-separated test purposes to run")
	types := flag.String("type", "", "Comma-separated test types to run")
	tags := flag.String("tags", "", "Comma-separated tags to include")
	testerNames := flag.String("tester", "", "Comma-separated Tester names to run")
	testIDs := flag.String("test-id", "", "Comma-separated TestCase IDs to run")
	packageFlag := flag.String("package", "", "Run a named tester package (e.g., smoke, regression, complete)")
	seed := flag.Int64("seed", 0, "Random seed (0 = auto-generate)")
	parallel := flag.Bool("parallel", false, "Enable parallel Tester execution")
	maxParallel := flag.Int("max-parallel", 4, "Maximum concurrent Testers")
	retryCount := flag.Int("retry", 0, "Retry count for failed cases")
	caseTimeout := flag.Duration("case-timeout", 30*time.Second, "Per-case timeout")
	runTimeout := flag.Duration("run-timeout", 30*time.Minute, "Overall run timeout")
	stopOnFail := flag.Bool("stop-on-fail", false, "Stop on first failure")
	skipCleanup := flag.Bool("skip-cleanup", false, "Skip Cleanup (for debugging)")
	verbose := flag.Bool("verbose", false, "Verbose logging")
	jsonReport := flag.String("json-report", "", "Write JSON report to this file")
	env := flag.String("env", "local", "Environment: local|test|staging")
	sharedDir := flag.String("shared-dir", "", "Path to shared/go/api/testers directory")
	configPath := flag.String("config", "config.toml", "Path to configuration file")
	flag.Parse()

	logger := loggerutil.CreateDefaultLogger("CWB_20260221071600")

	ctx := context.Background()

	// Step 1: Load configuration
	logger.Info("Step 1 Load config")
	cfg, err := loadConfig(ctx, *configPath)
	if err != nil {
		logger.Error("Config load failed", "error", err)
		os.Exit(2)
	}

	// Step 2: Safety check - refuse to run against production
	logger.Info("Step 2 Check ProductionDB")
	if isProductionDB(cfg) {
		logger.Error("Refusing to run AutoTester against production database")
		os.Exit(2)
	}

	// Step 3: Initialize database connections
	logger.Info("Step 3 InitDB")
	newCtx := context.WithValue(ctx, ApiTypes.CallFlowKey, "SHD_0220185500")
	if err := databaseutil.InitDB(newCtx, cfg.MySQLConfig, cfg.PGConfig); err != nil {
		logger.Error("Failed to initialize database", "error", err)
		os.Exit(2)
	}

	// Verify database connections
	if ApiTypes.PG_DB_Project == nil {
		logger.Error("Project database connection not initialized")
		os.Exit(2)
	}
	if ApiTypes.PG_DB_AutoTester == nil {
		logger.Error("AutoTester database connection not initialized")
		os.Exit(2)
	}

	// Step 4: Run auto-test migrations
	logger.Info("Step 4 Run Migrations")
	if err := runAutoTestMigrations(ctx, logger, cfg.MigrationConfig); err != nil {
		logger.Error("Failed to run auto-test migrations", "error", err)
		os.Exit(2)
	}

	// Step 4b: Create auto-test tables
	logger.Info("Step 4b Create AutoTest Tables")
	dbType := ApiTypes.DatabaseInfo.DBType
	if err := autotester.CreateAutoTestTables(logger, ApiTypes.PG_DB_AutoTester, dbType); err != nil {
		logger.Error("Failed to create auto-test tables", "error", err)
		os.Exit(2)
	}

	// Step 5: Register testers
	logger.Info("Step 5 Register testers")
	sharedDirPath := *sharedDir
	if sharedDirPath == "" {
		// Auto-detect: assume running from shared/go directory
		wd, err := os.Getwd()
		if err != nil {
			logger.Error("Failed to get working directory", "error", err)
			os.Exit(2)
		}
		sharedDirPath = filepath.Join(wd, "api", "testers")
	}
	logger.Info("Using testers.toml path", "shared-dir", sharedDirPath)

	// Register shared testers only (no app-specific testers)
	sharedtesters.RegisterTesters()
	// Load packages from shared testers.toml only (no project overrides)
	if err := sharedtesters.LoadTOMLPackages(sharedDirPath, ""); err != nil {
		logger.Error("Failed to load testers.toml", "error", err)
		os.Exit(2)
	}

	// Step 6: Create test runner
	logger.Info("Step 6 Create Test Runner")
	runConfig := &autotester.RunConfig{
		Purposes:    split(*purposes),
		Types:       split(*types),
		Tags:        split(*tags),
		TesterNames: split(*testerNames),
		TestIDs:     split(*testIDs),
		PackageName: *packageFlag,
		Seed:        *seed,
		Parallel:    *parallel,
		MaxParallel: *maxParallel,
		RetryCount:  *retryCount,
		CaseTimeout: *caseTimeout,
		RunTimeout:  *runTimeout,
		StopOnFail:  *stopOnFail,
		SkipCleanup: *skipCleanup,
		Verbose:     *verbose,
		JSONReport:  *jsonReport,
		Environment: *env,
	}

	runner := autotester.NewTestRunner(autotester.GlobalRegistry.Build(), runConfig, logger)

	// Step 7: Set up database persistence
	logger.Info("Step 7 Create DB Persistence")
	dbPersistence := autotester.NewDBPersistence(ApiTypes.PG_DB_AutoTester)
	runner.SetDBPersistence(dbPersistence)

	// Step 8: Run testers
	logger.Info("Step 8 Run testers")
	if err := runner.Run(ctx); err != nil {
		logger.Error("Test run failed", "error", err)
		os.Exit(2)
	}

	// Step 9: Generate summary
	logger.Info("Step 9 Generate summary")
	summary := runner.Summary()
	if summary.Failed > 0 || summary.Errored > 0 {
		os.Exit(1)
	}

	// Step 10: Test finished successfully
	logger.Info("Step 10 Test Finished")
	os.Exit(0)
}

// split converts a comma-separated string to a slice.
func split(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// Config holds the configuration for the autotester.
type Config struct {
	MySQLConfig     ApiTypes.DBConfig
	PGConfig        ApiTypes.DBConfig
	MigrationConfig ApiTypes.MigrationConfig
	Database        DatabaseConfig
}

// DatabaseConfig holds database connection settings.
type DatabaseConfig struct {
	PGHost string
}

// loadConfig loads configuration from a TOML file and environment variables.
// The config file path is specified by the 'path' argument (or AUTOTESTER_CONFIG env var).
func loadConfig(_ context.Context, path string) (*Config, error) {
	// Use env var if path not specified
	configPath := path
	if configPath == "" {
		configPath = os.Getenv("AUTOTESTER_CONFIG")
	}
	if configPath == "" {
		configPath = "config.toml"
	}

	// Expand ~ to home directory
	configPath, err := expandPath(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to expand config path: %w", err)
	}

	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("toml")

	// Set defaults
	v.SetDefault("mysql_host", "127.0.0.1")
	v.SetDefault("mysql_port", 3306)
	v.SetDefault("pg_host", "127.0.0.1")
	v.SetDefault("pg_port", 5432)

	// Allow environment variable overrides
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Map environment variables to config keys
	v.BindEnv("mysql_host", "MYSQL_HOST")
	v.BindEnv("mysql_port", "MYSQL_PORT")
	v.BindEnv("mysql_user", "MYSQL_USER_NAME")
	v.BindEnv("mysql_password", "MYSQL_PASSWORD")
	v.BindEnv("mysql_db", "MYSQL_DB_NAME")
	v.BindEnv("pg_host", "PG_HOST")
	v.BindEnv("pg_port", "PG_PORT")
	v.BindEnv("pg_user", "PG_USER_NAME")
	v.BindEnv("pg_password", "PG_PASSWORD")
	v.BindEnv("pg_db", "PG_DB_NAME")

	// Read config file (ignore error if file doesn't exist - use env vars)
	_ = v.ReadInConfig()

	// Build MySQL config
	mysqlCfg := ApiTypes.DBConfig{
		Host:       v.GetString("mysql_host"),
		Port:       v.GetInt("mysql_port"),
		DBType:     "mysql",
		CreateFlag: false,
		UserName:   v.GetString("mysql_user"),
		Password:   v.GetString("mysql_password"),
		DbName:     v.GetString("mysql_db"),
	}

	// Build PostgreSQL config
	pgCfg := ApiTypes.DBConfig{
		Host:       v.GetString("pg_host"),
		Port:       v.GetInt("pg_port"),
		DBType:     "postgres",
		CreateFlag: false,
		UserName:   v.GetString("pg_user"),
		Password:   v.GetString("pg_password"),
		DbName:     v.GetString("pg_db"),
	}

	// Build migration config
	migrationCfg := ApiTypes.MigrationConfig{
		MigrationsFS:  v.GetString("migrations_fs"),
		MigrationsDir: v.GetString("migrations_dir"),
		TableName:     v.GetString("goose_tablename"),
	}

	// Set migration defaults
	if migrationCfg.MigrationsDir == "" {
		migrationCfg.MigrationsDir = "migrations"
	}
	if migrationCfg.TableName == "" {
		migrationCfg.TableName = "goose_db_version"
	}

	cfg := &Config{
		MySQLConfig:     mysqlCfg,
		PGConfig:        pgCfg,
		MigrationConfig: migrationCfg,
		Database: DatabaseConfig{
			PGHost: pgCfg.Host,
		},
	}

	return cfg, nil
}

// expandPath expands ~ to the user's home directory and resolves relative paths.
func expandPath(path string) (string, error) {
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, path[2:]), nil
	}
	return filepath.Abs(path)
}

// isProductionDB checks if the config points to a production database.
func isProductionDB(cfg *Config) bool {
	productionHosts := []string{
		"prod-db.example.com",
		"production.database.example.com",
	}

	dbHost := cfg.Database.PGHost
	for _, prodHost := range productionHosts {
		if dbHost == prodHost {
			return true
		}
	}
	return false
}

// runAutoTestMigrations runs goose migrations for auto-test tables.
func runAutoTestMigrations(ctx context.Context, logger ApiTypes.JimoLogger, migrateCfg ApiTypes.MigrationConfig) error {
	var projectDB *sql.DB
	var sharedDB *sql.DB
	var autotesterDB *sql.DB
	dbType := ApiTypes.DatabaseInfo.DBType

	switch dbType {
	case ApiTypes.PgName:
		projectDB = ApiTypes.PG_DB_Project
		sharedDB = ApiTypes.PG_DB_Shared
		autotesterDB = ApiTypes.PG_DB_AutoTester
	case ApiTypes.MysqlName:
		projectDB = ApiTypes.MySql_DB_Project
		sharedDB = ApiTypes.MySql_DB_Shared
		autotesterDB = ApiTypes.MySql_DB_AutoTester
	default:
		return fmt.Errorf("unsupported db type (MID_060221143000): %s", dbType)
	}

	if projectDB == nil {
		return fmt.Errorf("project db is not set (SHD_GSE_067) for db type: %s", dbType)
	}

	if sharedDB == nil {
		return fmt.Errorf("shared database connection is not set (SHD_GSE_067) for db type: %s", dbType)
	}

	logger.Info("Running project migrations")
	if err := goose.RunProjectMigrations(ctx, logger, migrateCfg, projectDB); err != nil {
		return fmt.Errorf("failed to run project migrator: %w", err)
	}

	logger.Info("Running shared migrations")
	if err := goose.RunSharedMigrations(ctx, logger, migrateCfg, sharedDB); err != nil {
		return fmt.Errorf("failed to run shared migrator: %w", err)
	}

	logger.Info("Running autotester migrations")
	if err := goose.RunAutoTesterMigrations(ctx, logger, migrateCfg, autotesterDB); err != nil {
		return fmt.Errorf("failed to run auto-tester migrator: %w", err)
	}

	logger.Info("Auto-test migrations completed successfully")
	return nil
}
