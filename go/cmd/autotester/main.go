package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/autotester"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/chendingplano/shared/go/api/goose"
	"github.com/chendingplano/shared/go/api/loggerutil"
	sharedtesters "github.com/chendingplano/shared/go/api/testers"
	"github.com/spf13/viper"
)

type SharedTesterConfigDef struct {
	AppName string `mapstructure:"app_name"`
	Debug   bool   `mapstructure:"debug"`

	Server struct {
		Port int    `mapstructure:"port"`
		Host string `mapstructure:"host"`
	} `mapstructure:"server"`

	Database        ApiTypes.DatabaseConfig  `mapstructure:"database"`
	MigrationConfig ApiTypes.MigrationConfig `mapstructure:"migration"`
}

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
	configPath := flag.String("config", "config.toml", "Path to configuration file")
	flag.Parse()

	// Load .env from current working directory (project root)
	// err := godotenv.Load()
	// if err != nil {
	// 	slog.Error("Could not load .env file", "error", err)
	// }

	ApiUtils.LoadLibConfig("MID_26022601")

	logger := loggerutil.CreateDefaultLogger("MID_26022805")

	ctx := context.Background()

	// Step 1: Load configuration
	logger.Info("Step 1 Load config")
	err := LoadConfig(ctx, logger, *configPath)
	if err != nil {
		logger.Error("Config load failed", "error", err)
		os.Exit(2)
	}

	// Step 2: Safety check - refuse to run against production
	logger.Info("Step 2 Check ProductionDB")
	if isProductionDB() {
		logger.Error("Refusing to run AutoTester against production database")
		os.Exit(2)
	}

	// Step 3: Initialize database connections
	logger.Info("Step 3 InitDB")
	newCtx := context.WithValue(ctx, ApiTypes.CallFlowKey, "SHD_0220185500")
	if err := databaseutil.InitDB(newCtx, ApiTypes.CommonConfig); err != nil {
		logger.Error("Failed to initialize database", "error", err)
		os.Exit(2)
	}

	// Verify database connections
	if ApiTypes.CommonConfig.PGConf.ProjectDBHandle == nil {
		logger.Error("Project database connection not initialized")
		os.Exit(2)
	}

	if ApiTypes.CommonConfig.PGConf.AutotesterDBHandle == nil {
		logger.Error("AutoTester database connection not initialized")
		os.Exit(2)
	}

	// Step 4: Run auto-test migrations
	logger.Info("Step 4 Run Migrations")
	if err := runAutoTestMigrations(ctx, logger, autotester.AutotesterConfig.MigrationConfig); err != nil {
		logger.Error("Failed to run auto-test migrations", "error", err)
		os.Exit(2)
	}

	// Step 4b: Create auto-test tables
	logger.Info("Step 4b Create AutoTest Tables")
	dbType := ApiTypes.DBType
	if err := autotester.CreateAutoTestTables(logger, ApiTypes.CommonConfig.PGConf.AutotesterDBHandle, dbType); err != nil {
		logger.Error("Failed to create auto-test tables", "error", err)
		os.Exit(2)
	}

	// Step 5: Register testers
	logger.Info("Step 5 Register testers")

	// Register shared testers only (no app-specific testers)
	sharedtesters.RegisterTesters()
	// Load packages from shared testers.toml only (no project overrides)

	logger.Info("Using testers.toml path")
	if err := autotester.LoadTOMLPackages(newCtx, logger); err != nil {
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
	dbPersistence := autotester.NewDBPersistence(ApiTypes.CommonConfig.PGConf.AutotesterDBHandle)
	runner.SetDBPersistence(dbPersistence)

	// Step 8: Run testers
	logger.Info("Step 8 Run testers")
	if err := runner.Run(ctx); err != nil {
		logger.Error("Test run failed", "error", err)
		os.Exit(2)
	}

	summary := runner.Summary()
	if summary.Failed > 0 || summary.Errored > 0 {
		os.Exit(1)
	}

	// Step 9: Test finished successfully
	logger.Info("Step 9 Test Finished")
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

// isProductionDB checks if the config points to a production database.
func isProductionDB() bool {
	productionHosts := []string{
		"prod-db.example.com",
		"production.database.example.com",
	}

	dbHost := ApiTypes.CommonConfig.PGConf.Host
	for _, prodHost := range productionHosts {
		if dbHost == prodHost {
			return true
		}
	}
	return false
}

// runAutoTestMigrations runs goose migrations for auto-test tables.
func runAutoTestMigrations(
	ctx context.Context,
	logger ApiTypes.JimoLogger,
	migrateCfg ApiTypes.MigrationConfig) error {
	var projectDB *sql.DB = ApiTypes.ProjectDBHandle
	var migrateDB *sql.DB = ApiTypes.SharedMigrationDBHandle
	var autotesterDB *sql.DB = ApiTypes.AutotesterDBHandle
	dbType := ApiTypes.DBType

	if projectDB == nil {
		return fmt.Errorf("project db is not set (SHD_GSE_067) for db type: %s", dbType)
	}

	if migrateDB == nil {
		return fmt.Errorf("migrate database connection is not set (SHD_GSE_067) for db type: %s", dbType)
	}

	logger.Info("Running project migrations")
	if err := goose.RunProjectMigrations(ctx, logger, migrateCfg, projectDB); err != nil {
		return fmt.Errorf("failed to run project migrator: %w", err)
	}

	logger.Info("Running shared migrations")
	if err := goose.RunSharedMigrations(ctx, logger, migrateCfg, migrateDB); err != nil {
		return fmt.Errorf("failed to run shared migrator: %w", err)
	}

	logger.Info("Running autotester migrations")
	if err := goose.RunAutoTesterMigrations(ctx, logger, migrateCfg, autotesterDB); err != nil {
		return fmt.Errorf("failed to run auto-tester migrator: %w", err)
	}

	logger.Info("Auto-test migrations completed successfully")
	return nil
}

func LoadConfig(
	ctx context.Context,
	logger ApiTypes.JimoLogger,
	configPath string) error {

	logger.Info("Loading config", "config_path", configPath)
	viper.SetConfigFile(configPath)
	viper.SetConfigType("toml")

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			return fmt.Errorf("config file not found (MID_26022801): %s", configPath)
		}
		return fmt.Errorf("error reading config (MID_26022802): %w, config_path:%s", err, configPath)
	}

	// Override with environment variables (e.g., DATABASE_URL)
	viper.AutomaticEnv()

	var err error
	err = autotester.LoadAndRegisterTOMLConfigs(ctx, logger)
	if err != nil {
		return fmt.Errorf("failed load config, error:%w", err)
	}
	logger.Info("CommonConfig loaded",
		"db_type", ApiTypes.CommonConfig.AppInfo.DatabaseType,
		"db_type1", ApiTypes.DBType)

	logger.Info("Database type", "db_type", ApiTypes.DBType)

	// Fall back to config file values if env vars are not set (for backwards compatibility)
	if ApiTypes.CommonConfig.PGConf.UserName == "" {
		logger.Warn("(MID_26031209) PG_USER_NAME not set in env, falling back to config")
	}
	if ApiTypes.CommonConfig.PGConf.Password == "" {
		logger.Error("(MID_26031205) PG_PASSWORD not set in env, falling back to config")
	}
	if ApiTypes.CommonConfig.PGConf.ProjectDBName == "" {
		logger.Error("(MID_26031206) PG_DB_NAME not set in env, falling back to config")
	}

	logger.Info("Config load success",
		"database_type", ApiTypes.DBType,
		"need_create_tables", ApiTypes.CommonConfig.AppInfo.NeedCreateTables,
		"pg", ApiTypes.CommonConfig.PGConf.Create,
		"mysql", ApiTypes.CommonConfig.MySQLConf.Create)

	return nil
}
