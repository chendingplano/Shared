package autotester

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/pelletier/go-toml/v2"
)

var AutotesterConfig ApiTypes.AutotesterConfigDef

// LoadTOMLConfig parses a testers.toml file at the given path and returns the
// config. If the file does not exist the call succeeds with an empty config so
// that callers do not need to guard against missing files.
func LoadTOMLConfig(
	ctx context.Context,
	logger ApiTypes.JimoLogger) error {

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("(MID_26031210) failed get working dir, error:%w", err)
	}

	sharedPath := filepath.Join(wd, "config.toml")  // project overrides
	testerPath := filepath.Join(wd, "testers.toml") // project overrides

	logger.Info("LoadTOMLConfig", "testerPath", testerPath)

	err2 := ApiUtils.LoadConfig(ctx, logger, sharedPath)
	if err2 != nil {
		return fmt.Errorf("(MID_26030912) failed parsing common config, path:%s, error:%w", sharedPath, err2)
	}

	// Step 2: Load Autotester Config
	data, err3 := os.ReadFile(testerPath)
	if err3 != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("(MID_26030922) tester config file not exist:%s", testerPath)
		}

		return fmt.Errorf("(MID_26030913) reading tester config failed%s: %w", testerPath, err2)
	}

	if err := toml.Unmarshal(data, &AutotesterConfig); err != nil {
		return fmt.Errorf("(MID_26030932) failed parsing autotester config, path:%s", testerPath)
	}

	logger.Info("AutotesterConfig loaded",
		"testers filename", testerPath,
		"db_type", AutotesterConfig.DBType,
		"tester_db_name", AutotesterConfig.DUTDBName,
		"tester_migration_dbname", AutotesterConfig.MigrationConfig.DBName,
		"tester_migrationDir", AutotesterConfig.MigrationConfig.MigrationsDir)

	return nil
}

// LoadAndRegisterTOMLConfigs processes one or more testers.toml files in order.
// Each file's packages are upserted into GlobalPackageRegistry, so a package
// name defined in a later file overrides the same name from an earlier file or
// from a prior programmatic RegisterPackage() call.
//
// Conventional call site (from an application's registerAll function):
//
//	autotester.LoadAndRegisterTOMLConfigs(
//	    filepath.Join(sharedDir,   "testers.toml"),  // shared baseline
//	    filepath.Join(projectRoot, "testers.toml"),  // project overrides
//	)
//
// Missing files are silently skipped.
func LoadAndRegisterTOMLConfigs(
	ctx context.Context,
	logger ApiTypes.JimoLogger) error {
	err := LoadTOMLConfig(ctx, logger)
	if err != nil {
		return fmt.Errorf("(MID_26031001) failed LoadTOMLConfig, error:%w", err)
	}

	dbType := ApiTypes.DBType
	if dbType == "" {
		return fmt.Errorf("(MID_26030401) missing database type")
	}

	// Phase 1: register tester definitions so the package filter below can use them.
	for i, td := range AutotesterConfig.Testers {
		if td.Name == "" {
			return fmt.Errorf("autotester: tester definition at index %d in 'testers.toml' is missing a name (MID_260226100001)", i)
		}
		GlobalTesterDefinitionRegistry.Upsert(&td)
	}

	// Phase 2: build and upsert packages, honouring both package-level and global enabled flags.
	for i, p := range AutotesterConfig.Packages {
		if p.Name == "" {
			return fmt.Errorf("autotester: package at index %d in 'testers.toml' is missing a name", i)
		}
		testerNames := make([]string, 0, len(p.Testers))
		for j, tc := range p.Testers {
			if tc.Name == "" {
				return fmt.Errorf("autotester: tester at index %d in package %q is missing a name (MID_260226100002)", j, p.Name)
			}
			// Skip if disabled at the package level.
			if !tc.Enable {
				continue
			}
			// Skip if disabled globally (via [[testers]] definition).
			if !GlobalTesterDefinitionRegistry.IsEnabled(tc.Name) {
				continue
			}
			testerNames = append(testerNames, tc.Name)
		}
		GlobalPackageRegistry.Upsert(&TesterPackage{
			Name:        p.Name,
			Description: p.Description,
			Enable:      p.Enable,
			TesterNames: testerNames,
		})
	}
	return nil
}

// LoadTOMLPackages loads tester packages from testers.toml files and upserts
// them into GlobalPackageRegistry. It looks for:
//
//  1. <sharedDir>/testers.toml  — shared-library baseline packages
//  2. <projectRoot>/testers.toml — project-specific packages (override shared)
//
// Both files are optional; a missing file is silently skipped.
// A package name that appears in a later file replaces the same name from an
// earlier file or from a prior RegisterPackage() call.
//
// Each package in the TOML file defines:
//   - name: unique package identifier
//   - description: human-readable explanation
//   - enable: whether the package is enabled
//   - testers: array of tester configurations (name, enable, num_tcs, seconds)
//
// Typical usage — call this after RegisterTesters() and registering all
// app-specific testers:
//
//	sharedtesters.RegisterTesters()
//	// ... register app-specific testers via autotester.GlobalRegistry.Register() ...
//	sharedtesters.LoadTOMLPackages(sharedDir, projectRoot)
func LoadTOMLPackages(
	ctx context.Context,
	logger ApiTypes.JimoLogger) error {

	err := LoadAndRegisterTOMLConfigs(ctx, logger)

	if err != nil {
		return fmt.Errorf("(MID_26030303) load tester config error:%w", err)
	}
	logger.Info("MigrationDBName", "dbname", AutotesterConfig.MigrationConfig.DBName)

	dbType := ApiTypes.DBType

	if dbType != "pg" && dbType != "mysql" {
		return fmt.Errorf("(MID_26030304) db_type not supported:%s", dbType)
	}

	if dbType != "pg" {
		return fmt.Errorf("(MID_26030308) db_type not supported yet:%s", dbType)
	}

	host := ApiTypes.CommonConfig.PGConf.Host
	port := ApiTypes.CommonConfig.PGConf.Port
	username := ApiTypes.CommonConfig.PGConf.UserName
	password := ApiTypes.CommonConfig.PGConf.Password

	if password == "" {
		return fmt.Errorf("(MID_26030501) missing password")
	}

	if username == "" {
		return fmt.Errorf("(MID_26030502) missing username")
	}

	dbName := AutotesterConfig.DUTDBName
	if dbName == "" {
		return fmt.Errorf("(MID_26030305) missing tester db_name")
	}

	// Step 1: Create the DB handle for DUTDBHandle
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s sslmode=disable dbname=%s",
		host, port, username, password, dbName)

	AutotesterConfig.DUTDBHandle, err = sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("(MID_26030306) Failed to connect to testger PG, error:%w", err)
	}

	// Test the connection
	if err = AutotesterConfig.DUTDBHandle.Ping(); err != nil {
		// SECURITY: Don't log connection string or credentials
		return fmt.Errorf("(MID_26030307) failed connecting PostgreSQL for tester DB, error: %w", err)
	}

	logger.Info("PostgreSQL tester db created ", "dbname", dbName, "user", username)
	logger.Info("Tester Info",
		"dbname", AutotesterConfig.MigrationConfig.DBName,
		"tablename", AutotesterConfig.MigrationConfig.TableName)

	migrationDBName := AutotesterConfig.MigrationConfig.DBName
	if migrationDBName == "" {
		return fmt.Errorf("(MID_26030311) missing tester migration db_name")
	}

	// Step 2: Create the DB Handle for tester migrations
	connStr = fmt.Sprintf("host=%s port=%d user=%s password=%s sslmode=disable dbname=%s",
		host, port, username, password, migrationDBName)

	AutotesterConfig.MigrationDBHandle, err = sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("(MID_26030309) Failed to connect to testger migration PG, error:%w, dbname:%s",
			err, migrationDBName)
	}

	// Test the connection
	if err = AutotesterConfig.MigrationDBHandle.Ping(); err != nil {
		// SECURITY: Don't log connection string or credentials
		return fmt.Errorf("(MID_26030310) failed connecting migration DB, error: %w, dbname:%s",
			err, migrationDBName)
	}

	logger.Info("PostgreSQL tester migration db created ", "dbname", migrationDBName, "user", username)

	return nil
}
