// set.go holds the per-entity expert pools: every entity owns its own expert
// registry, so one NPC never recalls another NPC's private memory. All sessions
// of the same entity share one pool by design (the main session must see what a
// sub-session left behind).
package expert

import (
	"context"
	"sort"
	"sync"
)

// Set keeps one Registry per entity. Sessions of one entity write to the same
// pool; reads separate entities by key and skip entries the current session
// produced (Entry.SessionID).
type Set struct {
	mu       sync.RWMutex
	capacity int
	// newRegistry builds the registry of a lazily created entity pool.
	newRegistry func(capacity int) *Registry
	registries  map[string]*Registry
}

// NewSet returns an empty set whose lazily created entity pools start without
// experts.
func NewSet(capacity int) *Set {
	return &Set{
		capacity:    capacity,
		newRegistry: func(int) *Registry { return NewRegistry() },
		registries:  map[string]*Registry{},
	}
}

// NewDefaultSet returns a set whose lazily created entity pools hold the five
// bundled experts.
func NewDefaultSet(capacity int) *Set {
	return &Set{
		capacity:    capacity,
		newRegistry: NewDefaultRegistry,
		registries:  map[string]*Registry{},
	}
}

// RegistryOf returns the entity's pool, creating it on first use.
func (s *Set) RegistryOf(entity string) *Registry {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if registry, ok := s.registries[entity]; ok {
		return registry
	}
	registry := s.newRegistry(s.capacity)
	s.registries[entity] = registry
	return registry
}

// Remember stores one entry in the entity's pool and reports whether a
// recall-capable expert accepted it.
func (s *Set) Remember(entity, id string, entry Entry) bool {
	registry := s.RegistryOf(entity)
	if registry == nil {
		return false
	}
	return registry.Remember(id, entry)
}

// Compensate runs the active experts of one entity's pool; a different entity
// never sees these fragments. An entity without a pool contributes no
// fragments and gets no empty pool just from being read.
func (s *Set) Compensate(ctx context.Context, entity string, active []string, in Input) ([]Fragment, error) {
	registry := s.registry(entity)
	if registry == nil {
		return nil, nil
	}
	return registry.Compensate(ctx, active, in)
}

// DecayAll runs the nightly decay on every entity pool and returns
// entity -> expert -> how many entries each one dropped.
func (s *Set) DecayAll(factor float64) map[string]map[string]int {
	if s == nil {
		return map[string]map[string]int{}
	}
	registries := s.registryList()
	removed := make(map[string]map[string]int, len(registries))
	for _, pair := range registries {
		removed[pair.entity] = pair.registry.DecayAll(factor)
	}
	return removed
}

// Snapshot returns a copy of every entity pool's contexts (for persistence).
func (s *Set) Snapshot() map[string]map[string][]Entry {
	if s == nil {
		return map[string]map[string][]Entry{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	snapshot := make(map[string]map[string][]Entry, len(s.registries))
	for entity, registry := range s.registries {
		snapshot[entity] = registry.Snapshot()
	}
	return snapshot
}

// Load restores contexts saved by Snapshot; each entity gets its own pool.
func (s *Set) Load(snapshot map[string]map[string][]Entry) {
	if s == nil {
		return
	}
	for entity, experts := range snapshot {
		s.RegistryOf(entity).Load(experts)
	}
}

// DropInjectedEchoes removes injected-push echoes from every entity pool and
// reports how many entries were dropped.
func (s *Set) DropInjectedEchoes() int {
	if s == nil {
		return 0
	}
	dropped := 0
	for _, pair := range s.registryList() {
		dropped += DropInjectedEchoes(pair.registry)
	}
	return dropped
}

// Entities returns the entity keys that own a pool, sorted for stable output.
func (s *Set) Entities() []string {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	entities := make([]string, 0, len(s.registries))
	for entity := range s.registries {
		entities = append(entities, entity)
	}
	sort.Strings(entities)
	return entities
}

// registry returns the existing pool of one entity without creating it.
func (s *Set) registry(entity string) *Registry {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.registries[entity]
}

// entityRegistry pairs one pool with its entity key for lock-free iteration.
type entityRegistry struct {
	entity   string
	registry *Registry
}

// registryList returns a copy of every pool so callers can iterate without
// holding the set lock.
func (s *Set) registryList() []entityRegistry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	registries := make([]entityRegistry, 0, len(s.registries))
	for entity, registry := range s.registries {
		registries = append(registries, entityRegistry{entity: entity, registry: registry})
	}
	return registries
}
