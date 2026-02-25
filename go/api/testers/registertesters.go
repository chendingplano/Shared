package sharedtesters

import (
	"path/filepath"

	"github.com/chendingplano/shared/go/api/autotester"
)

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
}

// RegisterPackages registers the predefined tester packages for the shared library.
// Call this after RegisterTesters() so that all referenced tester names are already
// present in GlobalRegistry before any package is resolved.
//
// Predefined packages:
//
//	"smoke"      — fast sanity check: database connectivity and logger only
//	"regression" — core library regression: databaseutil and logger
//	"complete"   — full shared-library suite: all three shared testers
//
// Applications may register their own packages on top of these using
// autotester.RegisterPackage(). The package names must be unique across
// GlobalPackageRegistry.
//
// To load packages from testers.toml files instead of (or in addition to) calling
// this function, use LoadTOMLPackages.
func RegisterPackages() {
	autotester.GlobalPackageRegistry.Register(&autotester.TesterPackage{
		Name:        "smoke",
		Description: "Fast sanity check: verifies database connectivity and logger are operational",
		TesterNames: []string{"tester_database", "tester_logger"},
	})
	autotester.GlobalPackageRegistry.Register(&autotester.TesterPackage{
		Name:        "regression",
		Description: "Core regression suite: database utilities and logger correctness",
		TesterNames: []string{"tester_databaseutil", "tester_logger"},
	})
	autotester.GlobalPackageRegistry.Register(&autotester.TesterPackage{
		Name:        "complete",
		Description: "Full shared-library suite: all three shared testers",
		TesterNames: []string{"tester_database", "tester_databaseutil", "tester_logger"},
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
// earlier file or from a prior RegisterPackages() / RegisterPackage() call.
//
// Typical usage — call this after RegisterTesters() (and optionally after
// RegisterPackages() if you still want the hard-coded defaults as fallback):
//
//	sharedtesters.RegisterTesters()
//	sharedtesters.LoadTOMLPackages(sharedDir, projectRoot)
func LoadTOMLPackages(sharedDir, projectRoot string) error {
	return autotester.LoadAndRegisterTOMLConfigs(
		filepath.Join(sharedDir, "testers.toml"),
		filepath.Join(projectRoot, "testers.toml"),
	)
}
