package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
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

type SharedConfig struct {
	AppName string `mapstructure:"app_name"`
	Debug   bool   `mapstructure:"debug"`

	Server struct {
		Port int    `mapstructure:"port"`
		Host string `mapstructure:"host"`
	} `mapstructure:"server"`

	Database struct {
		CreateMySQL      bool   `mapstructure:"create_mysql"`
		CreatePG         bool   `mapstructure:"create_pg"`
		DatabaseType     string `mapstructure:"database_type"`
		PGHost           string `mapstructure:"pg_host"`
		PGPort           int    `mapstructure:"pg_port"`
		PGUserName       string `mapstructure:"pg_user_name"`
		PGPassword       string `mapstructure:"pg_password"`
		PGDBName         string `mapstructure:"pg_db_name"`
		MySQLHost        string `mapstructure:"mysql_host"`
		MySQLPort        int    `mapstructure:"mysql_port"`
		MySQLUserName    string `mapstructure:"mysql_user_name"`
		MySQLPassword    string `mapstructure:"mysql_password"`
		MySQLDBName      string `mapstructure:"mysql_db_name"`
		MaxConnections   int    `mapstructure:"max_connections"`
		NeedCreateTables bool   `mapstructure:"need_create_tables"`
	} `mapstructure:"database"`

	MigrationConfig ApiTypes.MigrationConfig `mapstructure:"migration"`
}

var GlobalConfig SharedConfig
var MySQLConfig ApiTypes.DBConfig
var PGConfig ApiTypes.DBConfig

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
	if err := databaseutil.InitDB(newCtx, MySQLConfig, PGConfig); err != nil {
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
	if err := runAutoTestMigrations(ctx, logger, GlobalConfig.MigrationConfig); err != nil {
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

	logger.Info("Test run configuration",
		"global registry", autotester.GlobalRegistry,
		"runConfig", runConfig,
	)
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

/*
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
*/

// isProductionDB checks if the config points to a production database.
func isProductionDB() bool {
	productionHosts := []string{
		"prod-db.example.com",
		"production.database.example.com",
	}

	dbHost := GlobalConfig.Database.PGHost
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
	var migrateDB *sql.DB
	var autotesterDB *sql.DB
	dbType := ApiTypes.DatabaseInfo.DBType

	switch dbType {
	case ApiTypes.PgName:
		projectDB = ApiTypes.PG_DB_Project
		migrateDB = ApiTypes.PG_DB_Migration
		autotesterDB = ApiTypes.PG_DB_AutoTester
	case ApiTypes.MysqlName:
		projectDB = ApiTypes.MySql_DB_Project
		migrateDB = ApiTypes.MySql_DB_Migration
		autotesterDB = ApiTypes.MySql_DB_AutoTester
	default:
		return fmt.Errorf("unsupported db type (MID_060221143000): %s", dbType)
	}

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

	// Unmarshal into struct
	if err := viper.Unmarshal(&GlobalConfig); err != nil {
		return fmt.Errorf("unable to decode config (MID_26022803): %w", err)
	}

	logger.Info("Database type", "db_type", GlobalConfig.Database.DatabaseType)
	if err := ApiUtils.ParseDatabaseInfo(
		logger,
		GlobalConfig.Database.DatabaseType,
		PGConfig.DbName,
		GlobalConfig.Database.MySQLDBName,
		ApiTypes.PG_DB_Project,
		ApiTypes.MySql_DB_Project); err != nil {
		logger.Error("failed config database info", "error", err)
		panic(err)
	}

	var err1 error
	var err2 error
	MySQLConfig, err1 = ApiUtils.ParseMySQLConfig(
		logger,
		GlobalConfig.Database.MySQLHost,
		GlobalConfig.Database.MySQLPort,
		GlobalConfig.Database.CreateMySQL)
	if err1 != nil {
		logger.Error("failed config MySQL", "error", err1)
		panic(err1)
	}

	PGConfig, err2 = ApiUtils.ParsePGConfig(
		logger,
		GlobalConfig.Database.PGHost,
		GlobalConfig.Database.PGPort,
		GlobalConfig.Database.CreatePG)
	if err2 != nil {
		logger.Error("failed config PG", "error", err2)
		panic(err2)
	}

	logger.Info("PG env vars (TAX_CFG_115)", "user", PGConfig.UserName, "db", PGConfig.DbName, "pwd_set", PGConfig.Password != "")

	// Fall back to config file values if env vars are not set (for backwards compatibility)
	if PGConfig.UserName == "" {
		logger.Warn("PG_USER_NAME not set in env, falling back to config (TAX_CFG_119)")
	}
	if PGConfig.Password == "" {
		logger.Error("PG_PASSWORD not set in env, falling back to config (TAX_CFG_122)")
	}
	if PGConfig.DbName == "" {
		logger.Error("PG_DB_NAME not set in env, falling back to config (TAX_CFG_125)")
	}

	logger.Info("Config load success",
		"database_type", GlobalConfig.Database.DatabaseType,
		"need_create_tables", GlobalConfig.Database.NeedCreateTables,
		"pg", GlobalConfig.Database.CreatePG,
		"mysql", GlobalConfig.Database.CreateMySQL)
	return nil
}
