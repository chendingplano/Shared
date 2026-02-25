package autotester

import (
	"sync"
)

// TesterFactory is a function that constructs a Tester.
// Using a factory allows lazy construction and avoids import cycles.
type TesterFactory func() Tester

// TesterRegistry holds the set of known Testers.
type TesterRegistry struct {
	factories map[string]TesterFactory
	mu        sync.RWMutex
}

// GlobalRegistry is the default global tester registry.
// Applications can use this directly or create their own registry.
var GlobalRegistry = &TesterRegistry{
	factories: make(map[string]TesterFactory),
}

// Register adds a Tester factory to the registry.
// Panics on duplicate name to catch configuration errors at startup.
func (r *TesterRegistry) Register(name string, factory TesterFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.factories[name]; exists {
		panic("duplicate tester name: " + name)
	}

	r.factories[name] = factory
}

// Build instantiates all registered Testers and returns them as a slice.
// Testers are returned in an unspecified order - do not rely on ordering.
func (r *TesterRegistry) Build() []Tester {
	r.mu.RLock()
	defer r.mu.RUnlock()

	testers := make([]Tester, 0, len(r.factories))
	for _, factory := range r.factories {
		testers = append(testers, factory())
	}
	return testers
}

// GetFactory returns the factory for a specific tester by name.
// Returns nil if the tester is not registered.
func (r *TesterRegistry) GetFactory(name string) TesterFactory {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.factories[name]
}

// Has returns true if a tester with the given name is registered.
func (r *TesterRegistry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.factories[name]
	return exists
}

// Count returns the number of registered testers.
func (r *TesterRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.factories)
}

// Names returns a sorted list of all registered tester names.
func (r *TesterRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	return names
}

// Clear removes all registered testers.
// Useful for testing the registry itself or resetting state.
func (r *TesterRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.factories = make(map[string]TesterFactory)
}

// Register wraps GlobalRegistry.Register for convenience.
func Register(name string, factory TesterFactory) {
	GlobalRegistry.Register(name, factory)
}

// Build wraps GlobalRegistry.Build for convenience.
func Build() []Tester {
	return GlobalRegistry.Build()
}
