package entity

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/ruicat/astrbox/internal/config"
)

// Store is the single owner of templates and entity state.
type Store struct {
	mu        sync.RWMutex
	dataDir   string
	templates map[string]map[string]any
	entities  map[string]*Entity
	loaded    bool
	// OnCreate is called after a new entity is stored (used by the event bridge).
	OnCreate func(entity *Entity)
}

// NewStore creates a store rooted at dataDir.
func NewStore(dataDir string) *Store {
	return &Store{dataDir: dataDir, templates: map[string]map[string]any{}, entities: map[string]*Entity{}}
}

// Load reads templates/*.json and entities.json.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.templates = map[string]map[string]any{}
	s.entities = map[string]*Entity{}
	if err := s.loadTemplatesLocked(); err != nil {
		return err
	}
	if err := s.loadEntitiesLocked(); err != nil {
		return err
	}
	s.loaded = true
	return nil
}

func (s *Store) loadTemplatesLocked() error {
	dir := filepath.Join(s.dataDir, "templates")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var data map[string]any
		if err := json.Unmarshal(raw, &data); err != nil {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".json")
		s.templates[strings.ReplaceAll(name, "_", ":")] = data
	}
	return nil
}

func (s *Store) loadEntitiesLocked() error {
	if err := s.loadEntitiesFileLocked(filepath.Join(s.dataDir, "entities.json")); err != nil {
		return err
	}
	return s.loadEntitiesFileLocked(s.userEntitiesPath())
}

// userEntitiesPath is the local-only NPC entity store.
func (s *Store) userEntitiesPath() string {
	return filepath.Join(s.dataDir, "user", "entities.json")
}

func (s *Store) loadEntitiesFileLocked(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var payload struct {
		Entities map[string]struct {
			Type string            `json:"type"`
			Data map[string]any    `json:"data"`
			Keys map[string]string `json:"keys"`
			Meta map[string]any    `json:"_meta"`
		} `json:"entities"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		_ = os.Rename(path, path+".broken.bak")
		return nil
	}
	for id, item := range payload.Entities {
		s.entities[id] = &Entity{
			ID: id, Type: item.Type, Data: item.Data, Keys: item.Keys,
			Meta: item.Meta, Templates: s.templates,
		}
	}
	return nil
}

// Save atomically writes public entities and local-only NPC entities.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	public := map[string]any{}
	user := map[string]any{}
	for id, ent := range s.entities {
		record := map[string]any{
			"id": ent.ID, "type": ent.Type, "keys": ent.Keys,
			"data": ent.Data, "_meta": ent.Meta,
		}
		if strings.HasPrefix(ent.Type, "NPC") {
			user[id] = record
		} else {
			public[id] = record
		}
	}
	if err := writeEntitiesFile(filepath.Join(s.dataDir, "entities.json"), public); err != nil {
		return err
	}
	return writeEntitiesFile(s.userEntitiesPath(), user)
}

func writeEntitiesFile(path string, entities map[string]any) error {
	payload := map[string]any{"version": 1, "entities": entities}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, raw)
}

// MigrateUserEntities splits a legacy combined entities.json when needed.
func (s *Store) MigrateUserEntities() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := os.Stat(s.userEntitiesPath()); err == nil {
		return nil
	}
	hasNPC := false
	for _, ent := range s.entities {
		if strings.HasPrefix(ent.Type, "NPC") {
			hasNPC = true
			break
		}
	}
	if !hasNPC {
		return nil
	}
	return s.saveLocked()
}

func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// GetTemplate returns a deep copy of a template.
func (s *Store) GetTemplate(typeName string) map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	template, ok := s.templates[typeName]
	if !ok {
		return nil
	}
	return deepCopyMap(template)
}

// AddTemplate stores or deep-merges a template. Merging (instead of replacing)
// matches the Python store: later scripts may redefine a type without repeating
// every field or annotation.
func (s *Store) AddTemplate(typeName string, data map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.templates[typeName]; ok {
		s.templates[typeName] = DeepMerge(existing, data)
		return
	}
	s.templates[typeName] = deepCopyMap(data)
}

// SaveTemplateFile persists one template to templates/<name>.json.
func (s *Store) SaveTemplateFile(typeName string) error {
	template := s.GetTemplate(typeName)
	if template == nil {
		return fmt.Errorf("模板 '%s' 不存在", typeName)
	}
	raw, err := json.MarshalIndent(template, "", "  ")
	if err != nil {
		return err
	}
	name := strings.ReplaceAll(typeName, ":", "_")
	return atomicWrite(filepath.Join(s.dataDir, "templates", name+".json"), raw)
}

// DeleteTemplateFile removes templates/<name>.json when present.
func (s *Store) DeleteTemplateFile(typeName string) error {
	name := strings.ReplaceAll(typeName, ":", "_")
	err := os.Remove(filepath.Join(s.dataDir, "templates", name+".json"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// RemoveTemplate deletes a template and reports whether it existed.
func (s *Store) RemoveTemplate(typeName string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.templates[typeName]
	delete(s.templates, typeName)
	return ok
}

// HasTemplate reports whether a template exists.
func (s *Store) HasTemplate(typeName string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.templates[typeName]
	return ok
}

// ListTemplates returns sorted template names.
func (s *Store) ListTemplates() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.templates))
	for name := range s.templates {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Create builds and stores a new entity.
func (s *Store) Create(entityID, typeName string, data map[string]any, keys map[string]string) (*Entity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.entities[entityID]; exists {
		return nil, &ValidationError{Message: fmt.Sprintf("实体 '%s' 已存在", entityID)}
	}
	ent, err := CreateEntity(entityID, typeName, s.templates, data, keys)
	if err != nil {
		return nil, err
	}
	s.entities[entityID] = ent
	if s.OnCreate != nil {
		s.OnCreate(ent)
	}
	return ent, nil
}

// Get returns an entity by id.
func (s *Store) Get(entityID string) *Entity {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.entities[entityID]
}

// GetOrRaise returns an entity or an error.
func (s *Store) GetOrRaise(entityID string) (*Entity, error) {
	ent := s.Get(entityID)
	if ent == nil {
		return nil, &PathNotFoundError{Message: fmt.Sprintf("实体 '%s' 不存在", entityID)}
	}
	return ent, nil
}

// Delete removes an entity.
func (s *Store) Delete(entityID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.entities[entityID]; !ok {
		return false
	}
	delete(s.entities, entityID)
	return true
}

// ListAll returns all entities sorted by id.
func (s *Store) ListAll() []*Entity {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Entity, 0, len(s.entities))
	for _, ent := range s.entities {
		out = append(out, ent)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// EntityMatches reports whether an entity matches a selector.
func (s *Store) EntityMatches(sel Selector, ent *Entity) bool {
	var classifications []string
	if raw, ok := ent.GetOrNone(config.Current().ClassificationField).([]any); ok {
		for _, item := range raw {
			if label, ok := item.(string); ok {
				classifications = append(classifications, label)
			}
		}
	}
	return sel.Matches(ent.Type, ent.ID, classifications)
}

// QuerySelector returns all matching entities.
func (s *Store) QuerySelector(sel Selector) []*Entity {
	var out []*Entity
	for _, ent := range s.ListAll() {
		if s.EntityMatches(sel, ent) {
			out = append(out, ent)
		}
	}
	return out
}

// Query supports id / type wildcard / key: / field=value queries.
func (s *Store) Query(query string) ([]*Entity, error) {
	query = strings.TrimSpace(query)
	if strings.HasPrefix(query, "key:") {
		spec := query[4:]
		if index := strings.Index(spec, "="); index >= 0 {
			return s.QueryByKey(strings.TrimSpace(spec[:index]), strings.TrimSpace(spec[index+1:])), nil
		}
		return s.QueryByKey(strings.TrimSpace(spec), ""), nil
	}
	if index := strings.Index(query, "="); index >= 0 && strings.Contains(query[:index], ".") {
		path := strings.TrimSpace(query[:index])
		value := strings.Trim(strings.TrimSpace(query[index+1:]), `"'`)
		var out []*Entity
		for _, ent := range s.ListAll() {
			if fmt.Sprint(ent.GetOrNone(path)) == value {
				out = append(out, ent)
			}
		}
		return out, nil
	}
	sel, err := ParseEntityRef(query)
	if err != nil {
		return nil, err
	}
	return s.QuerySelector(sel), nil
}

// QueryByKey returns entities carrying a key (and matching value when given).
func (s *Store) QueryByKey(key, value string) []*Entity {
	var out []*Entity
	for _, ent := range s.ListAll() {
		actual, ok := ent.Keys[key]
		if !ok {
			continue
		}
		if value == "" || actual == value {
			out = append(out, ent)
		}
	}
	return out
}

// Resolve reads a path from an entity, falling back to templates.
func (s *Store) Resolve(entityID, path string) (any, error) {
	ent, err := s.GetOrRaise(entityID)
	if err != nil {
		return nil, err
	}
	return ent.Get(path)
}

// SetPath writes a path and persists the store.
func (s *Store) SetPath(entityID, path string, value any) error {
	ent, err := s.GetOrRaise(entityID)
	if err != nil {
		return err
	}
	if err := ent.Set(path, value); err != nil {
		return err
	}
	return s.Save()
}

// Count returns the number of entities.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entities)
}

// Loaded reports whether Load completed.
func (s *Store) Loaded() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loaded
}

// Templates returns the template map (read-only use).
func (s *Store) Templates() map[string]map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.templates
}
