package autotester

import (
	"fmt"
	"sync"
)

// TesterPackage is a named, ordered collection of tester names.
// It represents a predefined selection of testers to run together,
// such as a "smoke" test suite, a "complete" regression suite, or any
// application-specific grouping.
//
// Example:
//
//	autotester.GlobalPackageRegistry.Register(&autotester.TesterPackage{
//	    Name:        "smoke",
//	    Description: "Quick sanity check: database and logger only",
//	    TesterNames: []string{"tester_database", "tester_logger"},
//	})
type TesterPackage struct {
	// Name is the unique, machine-readable identifier for this package.
	// e.g. "smoke", "complete", "regression", "nightly".
	Name string

	// Description is a human-readable explanation of what this package covers.
	Description string

	// TesterNames is the ordered list of tester names included in this package.
	// Each name must match a name registered in TesterRegistry.
	// The order determines sequential execution order when Parallel=false.
	TesterNames []string
}

// TesterPackageRegistry holds named tester packages.
// It is separate from TesterRegistry so that the same individual testers
// can appear in multiple packages without duplicating factory logic.
type TesterPackageRegistry struct {
	packages map[string]*TesterPackage
	mu       sync.RWMutex
}

// GlobalPackageRegistry is the default singleton package registry.
// Applications register their packages here at startup.
var GlobalPackageRegistry = &TesterPackageRegistry{
	packages: make(map[string]*TesterPackage),
}

// Register adds a TesterPackage to the registry.
// Panics on duplicate name to catch configuration errors at startup.
func (pr *TesterPackageRegistry) Register(pkg *TesterPackage) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	if _, exists := pr.packages[pkg.Name]; exists {
		panic("duplicate package name: " + pkg.Name)
	}
	pr.packages[pkg.Name] = pkg
}

// Get returns the package with the given name.
// Returns nil, false if the package is not registered.
func (pr *TesterPackageRegistry) Get(name string) (*TesterPackage, bool) {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	pkg, ok := pr.packages[name]
	return pkg, ok
}

// Has returns true if a package with the given name is registered.
func (pr *TesterPackageRegistry) Has(name string) bool {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	_, exists := pr.packages[name]
	return exists
}

// Names returns all registered package names (unordered).
func (pr *TesterPackageRegistry) Names() []string {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	names := make([]string, 0, len(pr.packages))
	for name := range pr.packages {
		names = append(names, name)
	}
	return names
}

// Build instantiates testers for the named package using the provided TesterRegistry.
// Testers are returned in the order defined by the package's TesterNames.
// Returns an error if the package is not found or a tester name is not registered.
func (pr *TesterPackageRegistry) Build(packageName string, registry *TesterRegistry) ([]Tester, error) {
	pkg, ok := pr.Get(packageName)
	if !ok {
		return nil, fmt.Errorf("package %q not found in registry (MID_240226100002)", packageName)
	}

	testers := make([]Tester, 0, len(pkg.TesterNames))
	for _, name := range pkg.TesterNames {
		factory := registry.GetFactory(name)
		if factory == nil {
			return nil, fmt.Errorf("tester %q (required by package %q) not found in registry (MID_240226100003)", name, packageName)
		}
		testers = append(testers, factory())
	}
	return testers, nil
}

// Upsert registers a TesterPackage, replacing any existing package with the same name.
// Unlike Register, it does not panic on duplicate names.
// Use this when loading packages from config files that should override programmatic defaults.
func (pr *TesterPackageRegistry) Upsert(pkg *TesterPackage) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	pr.packages[pkg.Name] = pkg
}

// Clear removes all registered packages.
// Useful for resetting state in tests of the registry itself.
func (pr *TesterPackageRegistry) Clear() {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	pr.packages = make(map[string]*TesterPackage)
}

// RegisterPackage adds a TesterPackage to GlobalPackageRegistry.
// This is a package-level convenience wrapper.
func RegisterPackage(pkg *TesterPackage) {
	GlobalPackageRegistry.Register(pkg)
}

// BuildPackage builds testers for the named package using GlobalRegistry and GlobalPackageRegistry.
// This is the preferred call site when using the default global registries.
//
// Example:
//
//	testers, err := autotester.BuildPackage("smoke")
//	if err != nil { log.Fatal(err) }
//	runner := autotester.NewTestRunner(testers, config, logger)
func BuildPackage(packageName string) ([]Tester, error) {
	return GlobalPackageRegistry.Build(packageName, GlobalRegistry)
}
