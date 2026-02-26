package sharedtesters

import (
	"path/filepath"

	"github.com/chendingplano/shared/go/api/autotester"
	"github.com/chendingplano/shared/go/api/testers/tester-migration"
)

// RegisterTesters registers all shared-library testers in the GlobalRegistry.
// This function must be called before LoadTOMLPackages() so that all tester
// names referenced in testers.toml files are already present in the registry.
//
// Registered testers:
//   - tester_database: tests database connectivity and basic CRUD operations
//   - tester_databaseutil: tests database utility functions
//   - tester_logger: tests logger functionality
//   - tester_migration: tests database migration scripts
//
// Typical usage in an application's registerAll function:
//
//	sharedtesters.RegisterTesters()
//	// ... register app-specific testers ...
//	sharedtesters.LoadTOMLPackages(sharedDir, projectRoot)
func RegisterTesters() {
	autotester.GlobalRegistry.Register("tester_database", func() autotester.Tester {
		return NewDatabaseTester(nil) // DB config will be set in Prepare
	})
	autotester.GlobalRegistry.Register("tester_databaseutil", func() autotester.Tester {
		return NewDatabaseUtilTester()
	})
	autotester.GlobalRegistry.Register("tester_logger", func() autotester.Tester {
		return NewLoggerTester()
	})
	autotester.GlobalRegistry.Register("tester_migration", func() autotester.Tester {
		return tester_migration.NewMigrationTester(nil)
	})
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
func LoadTOMLPackages(sharedDir, projectRoot string) error {
	return autotester.LoadAndRegisterTOMLConfigs(
		filepath.Join(sharedDir, "testers.toml"),
		filepath.Join(projectRoot, "testers.toml"),
	)
}
