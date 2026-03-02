package sharedtesters

import (
	"github.com/chendingplano/shared/go/api/autotester"
	tester_migration "github.com/chendingplano/shared/go/api/testers/tester-migration"
)

// RegisterTesters registers all shared-library testers in the GlobalRegistry.
// This function must be called before LoadTOMLPackages() so that all tester
// names referenced in testers.toml files are already present in the registry.
//
// IMPORTANT: this function assumes all testers have a constructor that
// takes no parameters.
//
// Typical usage in an application's registerAll function:
//
//	sharedtesters.RegisterTesters()
//	// ... register app-specific testers ...
//	autotester.LoadTOMLPackages(sharedDir, projectRoot)
func RegisterTesters() {
	autotester.GlobalRegistry.Register("tester_database", func() autotester.Tester {
		return NewDatabaseTester()
	})
	autotester.GlobalRegistry.Register("tester_databaseutil", func() autotester.Tester {
		return NewDatabaseUtilTester()
	})
	autotester.GlobalRegistry.Register("tester_logger", func() autotester.Tester {
		return NewLoggerTester()
	})
	autotester.GlobalRegistry.Register("tester_migration", func() autotester.Tester {
		return tester_migration.NewMigrationTester()
	})
}
