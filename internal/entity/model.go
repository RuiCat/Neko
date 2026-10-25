// Package entity implements the AstrBox entity model, path resolution and storage.
package entity

import (
	"fmt"
	"strings"
	"sync"
)

// PathNotFoundError reports a missing entity or field path.
type PathNotFoundError struct{ Message string }

func (e *PathNotFoundError) Error() string { return e.Message }

// ValidationError reports an invalid entity definition or value.
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

// Entity is one independently evolving instance. Templates are only used to
// initialize it, so every write lands in Data.
type Entity struct {
	mu   sync.RWMutex
	ID   string
	Type string
	Data map[string]any
	Keys map[string]string
	Meta map[string]any

	// Templates is shared with the store and only read for fallback lookups.
	Templates map[string]map[string]any
	// OnChange is called after a successful write when non-nil.
	OnChange  func(entity *Entity, path string, oldValue, newValue any)
	listeners []func(entity *Entity, path string, oldValue, newValue any)
}

// AddChangeListener appends a per-tick listener and returns a remove function.
// Listeners are called after writes alongside OnChange (used by the engine to
// observe implicit writes made by @函数/事件).
func (e *Entity) AddChangeListener(listener func(entity *Entity, path string, oldValue, newValue any)) func() {
	e.mu.Lock()
	e.listeners = append(e.listeners, listener)
	index := len(e.listeners) - 1
	e.mu.Unlock()
	return func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		if index < len(e.listeners) {
			e.listeners[index] = nil
		}
	}
}

// DeepMerge merges override into base recursively; scalars and lists replace.
func DeepMerge(base, override map[string]any) map[string]any {
	out := deepCopyMap(base)
	for key, value := range override {
		if child, ok := value.(map[string]any); ok {
			if current, ok := out[key].(map[string]any); ok {
				out[key] = DeepMerge(current, child)
				continue
			}
		}
		out[key] = deepCopy(value)
	}
	return out
}

// TypeChain returns the inheritance chain from base to leaf.
func TypeChain(typeName string) []string {
	parts := strings.Split(typeName, ":")
	chain := make([]string, 0, len(parts))
	for i := 1; i <= len(parts); i++ {
		chain = append(chain, strings.Join(parts[:i], ":"))
	}
	return chain
}

// IsSubtypeOf reports whether sub equals parent or inherits from it.
func IsSubtypeOf(sub, parent string) bool {
	return sub == parent || strings.HasPrefix(sub, parent+":")
}

// ValidateID rejects ids containing separators or spaces.
func (e *Entity) ValidateID() error {
	if e.ID == "" {
		return &ValidationError{Message: "实体 id 不能为空"}
	}
	for _, ch := range ".[] " {
		if strings.ContainsRune(e.ID, ch) {
			return &ValidationError{Message: fmt.Sprintf("实体 id '%s' 不能包含字符: '%c'", e.ID, ch)}
		}
	}
	return nil
}

// Get reads a dotted path. Missing instance fields fall back to the template
// chain, most derived template first.
func (e *Entity) Get(path string) (any, error) {
	parts := splitPath(path)
	if len(parts) == 0 {
		return e.Snapshot(), nil
	}
	e.mu.RLock()
	current, found := lookup(e.Data, parts)
	e.mu.RUnlock()
	if found {
		return current, nil
	}
	if value, ok := lookupTemplates(e.Templates, e.Type, parts); ok {
		return value, nil
	}
	return nil, &PathNotFoundError{Message: fmt.Sprintf("实体 '%s' 路径 '%s' 不存在", e.ID, path)}
}

// GetOrNone reads a path and returns nil instead of an error when missing.
func (e *Entity) GetOrNone(path string) any {
	value, err := e.Get(path)
	if err != nil {
		return nil
	}
	return value
}

// Set writes a dotted path into the instance and fires OnChange.
func (e *Entity) Set(path string, value any) error {
	parts := splitPath(path)
	if len(parts) == 0 {
		return &PathNotFoundError{Message: "空路径"}
	}
	var oldValue any
	if current, err := e.Get(path); err == nil {
		oldValue = current
	}
	e.mu.Lock()
	current := e.Data
	for _, key := range parts[:len(parts)-1] {
		child, ok := current[key].(map[string]any)
		if !ok {
			child = map[string]any{}
			current[key] = child
		}
		current = child
	}
	current[parts[len(parts)-1]] = deepCopy(value)
	listeners := append([]func(entity *Entity, path string, oldValue, newValue any){}, e.listeners...)
	e.mu.Unlock()
	if e.OnChange != nil {
		e.OnChange(e, path, oldValue, value)
	}
	for _, listener := range listeners {
		if listener != nil {
			listener(e, path, oldValue, value)
		}
	}
	return nil
}

// Has reports whether the path exists in the instance data.
func (e *Entity) Has(path string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	_, found := lookup(e.Data, splitPath(path))
	return found
}

// CreateEntity builds an instance by deep-merging the template chain and data.
func CreateEntity(entityID, typeName string, templates map[string]map[string]any,
	data map[string]any, keys map[string]string) (*Entity, error) {
	merged := map[string]any{}
	for _, name := range TypeChain(typeName) {
		if template, ok := templates[name]; ok {
			merged = DeepMerge(merged, template)
		}
	}
	if data != nil {
		merged = DeepMerge(merged, data)
	}
	ent := &Entity{ID: entityID, Type: typeName, Data: merged, Keys: keys, Templates: templates}
	if err := ent.ValidateID(); err != nil {
		return nil, err
	}
	return ent, nil
}

// Snapshot returns a deep copy of the instance data. It holds the read lock
// while copying, so callers never read Data outside the entity lock.
func (e *Entity) Snapshot() map[string]any {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return deepCopyMap(e.Data)
}

func splitPath(path string) []string {
	if path == "" {
		return nil
	}
	return strings.Split(path, ".")
}

func lookup(data map[string]any, parts []string) (any, bool) {
	var current any = data
	for _, key := range parts {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[key]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func lookupTemplates(templates map[string]map[string]any, typeName string, parts []string) (any, bool) {
	chain := TypeChain(typeName)
	for i := len(chain) - 1; i >= 0; i-- {
		template, ok := templates[chain[i]]
		if !ok {
			continue
		}
		if value, found := lookup(template, parts); found {
			return deepCopy(value), true
		}
	}
	return nil, false
}

func deepCopy(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return deepCopyMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = deepCopy(item)
		}
		return out
	default:
		return value
	}
}

func deepCopyMap(data map[string]any) map[string]any {
	out := make(map[string]any, len(data))
	for key, value := range data {
		out[key] = deepCopy(value)
	}
	return out
}
