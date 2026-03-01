package autotester

import (
	"errors"
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

// TesterConfig is the configuration for a single tester within a package.
// It controls whether the tester runs and its execution limits.
//
// Example:
//
//	{ name = "tester_database", enable = true, num_tcs = 20, seconds = 60 }
type TesterConfig struct {
	Name    string `toml:"name"`
	Enable  bool   `toml:"enable"`  // If false, tester is excluded from the package
	NumTcs  int    `toml:"num_tcs"`
	Seconds int    `toml:"seconds"`
}

// PackageConfig is the in-TOML representation of a single tester package.
//
// Example testers.toml entry:
//
//	[[packages]]
//	name        = "smoke"
//	description = "Fast pre-deploy sanity check"
//	testers = [
//	    { name = "tester_database", enable = true, num_tcs = 20, seconds = 60 },
//	    { name = "tester_logger", enable = false, num_tcs = 30, seconds = 120 },
//	]
//
// Note: The Enable field is parsed but ignored. Packages are always loaded
// regardless of their enable status. Only tester-level enable flags are enforced.
type PackageConfig struct {
	Name        string         `toml:"name"`
	Description string         `toml:"description"`
	Enable      bool           `toml:"enable"` // Ignored: packages are always loaded
	Testers     []TesterConfig `toml:"testers"`
}

// TOMLConfig is the top-level structure of a testers.toml file.
// It contains two independent sections:
//   - Testers: the catalog of available testers with metadata and global enable status
//   - Packages: named collections of testers to run together
type TOMLConfig struct {
	Testers  []TesterDefinition `toml:"testers"`
	Packages []PackageConfig    `toml:"packages"`
}

// LoadTOMLConfig parses a testers.toml file at the given path and returns the
// config. If the file does not exist the call succeeds with an empty config so
// that callers do not need to guard against missing files.
func LoadTOMLConfig(path string) (*TOMLConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &TOMLConfig{}, nil
		}
		return nil, fmt.Errorf("autotester: reading %s: %w", path, err)
	}

	var cfg TOMLConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("autotester: parsing %s: %w", path, err)
	}
	return &cfg, nil
}

// RegisterPackagesFromTOML loads a testers.toml file, registers the tester
// definitions from its [[testers]] section into GlobalTesterDefinitionRegistry,
// and upserts every package from its [[packages]] section into
// GlobalPackageRegistry.
//
// Processing order within a single file:
//  1. [[testers]] entries are upserted into GlobalTesterDefinitionRegistry first,
//     so that the package-filtering step below can consult the global enabled status.
//  2. [[packages]] entries are then processed. A tester is included in a package
//     only when BOTH conditions hold:
//     a. its package-level enable field is true, AND
//     b. its global definition is enabled (or it is not listed in [[testers]])
//
// A missing file is silently skipped (returns nil). A malformed file returns an
// error describing the problem. Package definitions override any programmatically
// registered package with the same name (upsert semantics).
func RegisterPackagesFromTOML(path string) error {
	cfg, err := LoadTOMLConfig(path)
	if err != nil {
		return err
	}

	// Phase 1: register tester definitions so the package filter below can use them.
	for i, td := range cfg.Testers {
		if td.Name == "" {
			return fmt.Errorf("autotester: tester definition at index %d in %s is missing a name (MID_260226100001)", i, path)
		}
		GlobalTesterDefinitionRegistry.Upsert(&td)
	}

	// Phase 2: build and upsert packages, honouring both package-level and global enabled flags.
	for i, p := range cfg.Packages {
		if p.Name == "" {
			return fmt.Errorf("autotester: package at index %d in %s is missing a name", i, path)
		}
		testerNames := make([]string, 0, len(p.Testers))
		for j, tc := range p.Testers {
			if tc.Name == "" {
				return fmt.Errorf("autotester: tester at index %d in package %q (%s) is missing a name (MID_260226100002)", j, p.Name, path)
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
			TesterNames: testerNames,
		})
	}
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
func LoadAndRegisterTOMLConfigs(paths ...string) error {
	for _, path := range paths {
		if err := RegisterPackagesFromTOML(path); err != nil {
			return err
		}
	}
	return nil
}
