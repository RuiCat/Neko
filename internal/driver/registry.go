// Context runtime registry persisted to data/contexts.json (plan C0).
package driver

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Context status values used by the registry and the invoke policy.
const (
	// STATUS_CLOSED means the context has no session in this process.
	STATUS_CLOSED = "closed"
	// STATUS_IDLE means the session exists and no turn is running.
	STATUS_IDLE = "idle"
	// STATUS_RUNNING means a prompt was accepted and its turn may be running.
	STATUS_RUNNING = "running"
)

// Record is one context's durable runtime state.
type Record struct {
	ContextID     string    `json:"contextId"`
	CharacterID   string    `json:"characterId"`
	SessionID     string    `json:"sessionId"`
	PresetID      string    `json:"presetId"`
	Cwd           string    `json:"cwd"`
	Entity        string    `json:"entity"`
	Status        string    `json:"status"`
	LastTurn      int       `json:"lastTurn"`
	LastRequestID string    `json:"lastRequestId,omitempty"`
	OpenedAt      time.Time `json:"openedAt,omitempty"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// registryFile is the on-disk document.
type registryFile struct {
	Version   int               `json:"version"`
	UpdatedAt time.Time         `json:"updatedAt"`
	Contexts  map[string]Record `json:"contexts"`
}

// Registry owns the in-memory context records and their persisted copy.
type Registry struct {
	path    string
	mu      sync.Mutex
	records map[string]Record
}

// LoadRegistry loads the persisted registry and merges the declared specs onto
// it. Identity fields come from the configuration; status and last turn come
// from the persisted record.
func LoadRegistry(path string, specs []Spec) (*Registry, error) {
	registry := &Registry{path: path, records: map[string]Record{}}
	raw, err := os.ReadFile(path)
	switch {
	case err == nil:
		file := registryFile{}
		if err := json.Unmarshal(raw, &file); err != nil {
			return nil, fmt.Errorf("解析上下文注册表 %s 失败: %w", path, err)
		}
		for key, record := range file.Contexts {
			registry.records[key] = record
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		return nil, err
	}
	now := time.Now()
	declared := map[string]bool{}
	for _, spec := range specs {
		record, ok := registry.records[spec.ContextID]
		if !ok || record.SessionID == "" {
			record = Record{ContextID: spec.ContextID, Status: STATUS_CLOSED, OpenedAt: now}
		}
		record.CharacterID = spec.CharacterID
		record.PresetID = spec.PresetID
		record.Cwd = spec.Cwd
		record.Entity = spec.entityID()
		if record.Status == "" {
			record.Status = STATUS_CLOSED
		}
		registry.records[spec.ContextID] = record
		declared[spec.ContextID] = true
	}
	for key := range registry.records {
		if !declared[key] {
			delete(registry.records, key)
		}
	}
	return registry, nil
}

// Get returns one record.
func (r *Registry) Get(contextID string) (Record, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	record, ok := r.records[contextID]
	return record, ok
}

// List returns every record ordered by context id.
func (r *Registry) List() []Record {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Record, 0, len(r.records))
	for _, record := range r.records {
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ContextID < out[j].ContextID })
	return out
}

// FindBySession returns the record owning a DSH session id.
func (r *Registry) FindBySession(sessionID string) (Record, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, record := range r.records {
		if record.SessionID == sessionID {
			return record, true
		}
	}
	return Record{}, false
}

// Update applies mutate to one record and persists the registry.
func (r *Registry) Update(contextID string, mutate func(*Record)) (Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	record, ok := r.records[contextID]
	if !ok {
		return Record{}, fmt.Errorf("未知上下文 %q", contextID)
	}
	mutate(&record)
	record.UpdatedAt = time.Now()
	r.records[contextID] = record
	return record, r.saveLocked()
}

// saveLocked writes the registry through a temporary file and rename.
func (r *Registry) saveLocked() error {
	file := registryFile{Version: 1, UpdatedAt: time.Now(), Contexts: r.records}
	raw, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	temporary := r.path + ".tmp"
	if err := os.WriteFile(temporary, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, r.path)
}
