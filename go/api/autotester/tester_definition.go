package autotester

import (
	"sync"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// TesterDefinitionRegistry holds TesterDefinition entries loaded from the
// [[testers]] section of testers.toml files.
//
// It is separate from TesterRegistry (which holds factory functions) so that
// metadata and enable/disable state can be managed independently of Go code.
type TesterDefinitionRegistry struct {
	definitions map[string]*ApiTypes.TesterDefinition
	mu          sync.RWMutex
}

// GlobalTesterDefinitionRegistry is the singleton definition registry used by
// the TOML loader and the TestRunner.
var GlobalTesterDefinitionRegistry = &TesterDefinitionRegistry{
	definitions: make(map[string]*ApiTypes.TesterDefinition),
}

// Upsert registers or replaces a TesterDefinition.
// Always uses upsert (no panic on duplicate) because TOML files can override
// each other across the shared-to-project load order.
func (r *TesterDefinitionRegistry) Upsert(def *ApiTypes.TesterDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.definitions[def.Name] = def
}

// Get returns the TesterDefinition for the given name.
// Returns nil, false if the name is not in the registry.
func (r *TesterDefinitionRegistry) Get(name string) (*ApiTypes.TesterDefinition, bool) {
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

// AllEnabled returns all tester names that are globally enabled.
// A tester is considered enabled if:
//   - it is not in the registry (not defined → enabled by default), OR
//   - its Enabled field is nil or points to true
//
// This is used by the "complete" package to run all enabled testers.
func (r *TesterDefinitionRegistry) AllEnabled() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.definitions))
	for name, def := range r.definitions {
		if def.IsEnabled() {
			names = append(names, name)
		}
	}
	return names
}

// Clear removes all registered definitions. Useful for resetting state in tests.
func (r *TesterDefinitionRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.definitions = make(map[string]*ApiTypes.TesterDefinition)
}
