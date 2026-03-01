package autotester

import "sync"

// TesterDefinition holds the metadata for a tester as declared in a [[testers]]
// entry in a testers.toml file. It is the authoritative record of a tester's
// identity, purpose, and global enabled/disabled status.
//
// A tester that is disabled here (Enabled = false) will not run in any package,
// regardless of package-level settings. This acts as a global kill switch.
//
// Example testers.toml entry:
//
//	[[testers]]
//	name        = "tester_database"
//	desc        = "Tests database connectivity and basic CRUD operations"
//	purpose     = "validation"
//	type        = "integration"
//	dynamic_tcs = true
//	enabled     = true
//	creator     = "AutoTester Framework"
//	created_at  = "2026-02-20T00:00:00Z"
type TesterDefinition struct {
	// Name is the unique machine-readable identifier.
	// Allowed characters: letters, digits, dashes, underscores. Max 64 chars.
	Name string `toml:"name"`

	// Desc is a short, human-readable description of what the tester tests.
	Desc string `toml:"desc"`

	// Purpose is the tester's intended purpose (e.g. "validation", "regression").
	Purpose string `toml:"purpose"`

	// Type is the tester's category (e.g. "functional", "performance",
	// "compliance", "integration"). Default: "functional".
	Type string `toml:"type"`

	// DynamicTcs indicates whether this tester can generate test cases
	// dynamically at runtime (true) or only uses static hard-coded cases (false).
	DynamicTcs bool `toml:"dynamic_tcs"`

	// Enabled is the global on/off switch for this tester.
	// A nil pointer means the field was absent in TOML — treated as true (default enabled).
	// Set to false to prevent the tester from running in any package.
	Enabled *bool `toml:"enabled"`

	// Remarks holds any additional notes about the tester.
	Remarks string `toml:"remarks"`

	// Creator is the person or team who authored this tester.
	Creator string `toml:"creator"`

	// CreatedAt is the ISO-8601 timestamp of when this tester was created.
	CreatedAt string `toml:"created_at"`
}

// IsEnabled returns the effective enabled status of this tester definition.
// Returns true if Enabled is nil (field absent in TOML → default enabled) or
// if it points to true.
func (td *TesterDefinition) IsEnabled() bool {
	if td.Enabled == nil {
		return true // default: enabled when not specified
	}
	return *td.Enabled
}

// TesterDefinitionRegistry holds TesterDefinition entries loaded from the
// [[testers]] section of testers.toml files.
//
// It is separate from TesterRegistry (which holds factory functions) so that
// metadata and enable/disable state can be managed independently of Go code.
type TesterDefinitionRegistry struct {
	definitions map[string]*TesterDefinition
	mu          sync.RWMutex
}

// GlobalTesterDefinitionRegistry is the singleton definition registry used by
// the TOML loader and the TestRunner.
var GlobalTesterDefinitionRegistry = &TesterDefinitionRegistry{
	definitions: make(map[string]*TesterDefinition),
}

// Upsert registers or replaces a TesterDefinition.
// Always uses upsert (no panic on duplicate) because TOML files can override
// each other across the shared-to-project load order.
func (r *TesterDefinitionRegistry) Upsert(def *TesterDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.definitions[def.Name] = def
}

// Get returns the TesterDefinition for the given name.
// Returns nil, false if the name is not in the registry.
func (r *TesterDefinitionRegistry) Get(name string) (*TesterDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.definitions[name]
	return def, ok
}

// IsEnabled returns the effective enabled status for a tester name:
//   - true  if the name is not in the registry (not defined → enabled by default)
//   - true  if the definition's Enabled field is nil or points to true
//   - false if the definition's Enabled field points to false
func (r *TesterDefinitionRegistry) IsEnabled(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.definitions[name]
	if !ok {
		return true // not defined → default: enabled
	}
	return def.IsEnabled()
}

// Names returns all registered tester definition names (unordered).
func (r *TesterDefinitionRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.definitions))
	for name := range r.definitions {
		names = append(names, name)
	}
	return names
}

// Clear removes all registered definitions. Useful for resetting state in tests.
func (r *TesterDefinitionRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.definitions = make(map[string]*TesterDefinition)
}
