// templateParents.go owns the explicit parent table for @实体 composition and
// its templates/<name>.parents.json sidecars.
package entity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// SetTemplateParents stores or clears the explicit parents of one template.
func (s *Store) SetTemplateParents(typeName string, parents []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.order = map[string][]string{}
	if len(parents) == 0 {
		delete(s.parents, typeName)
		return
	}
	s.parents[typeName] = CopyParents(parents)
}

// GetTemplateParents returns a copy of one template's explicit parents.
func (s *Store) GetTemplateParents(typeName string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return CopyParents(s.parents[typeName])
}

// TemplateParentsSnapshot returns a deep copy of the whole parent table.
func (s *Store) TemplateParentsSnapshot() map[string][]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string][]string, len(s.parents))
	for name, parents := range s.parents {
		out[name] = CopyParents(parents)
	}
	return out
}

// ResolveOrder returns the merge order used for one type (ancestors first,
// typeName last).
func (s *Store) ResolveOrder(typeName string) []string {
	s.mu.RLock()
	cached, ok := s.order[typeName]
	s.mu.RUnlock()
	if ok {
		return cached
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if cached, ok := s.order[typeName]; ok {
		return cached
	}
	order := TemplateOrder(typeName, s.templates, s.parents)
	if s.order == nil {
		s.order = map[string][]string{}
	}
	s.order[typeName] = order
	return order
}

// Ancestors returns every template merged into typeName, excluding typeName.
func (s *Store) Ancestors(typeName string) []string {
	order := s.ResolveOrder(typeName)
	if len(order) == 0 || order[len(order)-1] != typeName {
		return order
	}
	return order[:len(order)-1]
}

// saveParentsFile writes the parents sidecar, removing it when the template has
// no explicit parents. Callers must not hold s.mu.
func (s *Store) saveParentsFile(typeName string) error {
	parents := s.GetTemplateParents(typeName)
	name := strings.ReplaceAll(typeName, ":", "_")
	path := filepath.Join(s.dataDir, "templates", name+".parents.json")
	if len(parents) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	raw, err := json.MarshalIndent(parents, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, raw)
}
