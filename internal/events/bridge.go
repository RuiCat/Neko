package events

import (
	"strings"
	"sync"

	"github.com/ruicat/astrbox/internal/entity"
)

// Bridge attaches schema-driven change hooks to entities.
type Bridge struct {
	store       *entity.Store
	bus         *Bus
	mu          sync.RWMutex
	schemas     map[string]*entity.EntitySchema
	records     []string
	validations []map[string]any
}

// NewBridge builds a bridge and refreshes schemas from current templates.
func NewBridge(store *entity.Store, bus *Bus) *Bridge {
	bridge := &Bridge{store: store, bus: bus, schemas: map[string]*entity.EntitySchema{}}
	bridge.Refresh()
	return bridge
}

// Refresh rebuilds schemas from templates and attaches every entity.
func (b *Bridge) Refresh() {
	schemas := map[string]*entity.EntitySchema{}
	for _, typeName := range b.store.ListTemplates() {
		template := b.store.GetTemplate(typeName)
		if template == nil {
			continue
		}
		schema := entity.NewEntitySchema()
		if _, ok := template["__annotations__"]; ok {
			schema.AddFromAnnotations(template)
		} else {
			schema.AddFromData(template, "")
		}
		schemas[typeName] = schema
	}
	b.mu.Lock()
	b.schemas = schemas
	b.mu.Unlock()
	b.AttachAll()
}

// AttachAll attaches the change hook to every entity.
func (b *Bridge) AttachAll() {
	for _, ent := range b.store.ListAll() {
		b.Attach(ent)
	}
}

// Attach installs the OnChange hook on one entity.
func (b *Bridge) Attach(ent *entity.Entity) {
	if ent == nil || ent.OnChange != nil {
		return
	}
	ent.OnChange = func(changed *entity.Entity, path string, oldValue, newValue any) {
		b.HandleChange(changed, path, oldValue, newValue)
	}
}

// LastValidations returns the most recent schema validation failures.
func (b *Bridge) LastValidations(limit int) []map[string]any {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if limit <= 0 || limit > len(b.validations) {
		limit = len(b.validations)
	}
	out := make([]map[string]any, limit)
	copy(out, b.validations[len(b.validations)-limit:])
	return out
}

// LastRecords returns the most recent event dispatch records.
func (b *Bridge) LastRecords() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]string, len(b.records))
	copy(out, b.records)
	return out
}

// HandleChange validates the write and fires the bound event, if any.
func (b *Bridge) HandleChange(ent *entity.Entity, path string, oldValue, newValue any) {
	schema := b.schemaFor(ent.Type)
	if schema == nil {
		return
	}
	if err := schema.Validate(path, newValue); err != "" {
		b.mu.Lock()
		b.validations = append(b.validations, map[string]any{
			"type": "validation", "entity": ent.ID, "path": path, "error": err,
		})
		if len(b.validations) > 200 {
			b.validations = b.validations[len(b.validations)-200:]
		}
		b.mu.Unlock()
	}
	eventName := schema.GetEvent(path)
	if eventName == "" && !strings.Contains(path, ".") {
		for fieldPath := range schema.All() {
			if strings.HasSuffix(fieldPath, "."+path) {
				eventName = schema.GetEvent(fieldPath)
				break
			}
		}
	}
	if eventName == "" {
		return
	}
	ctx := Context{
		EventName: eventName, EntityID: ent.ID, FieldPath: path,
		OldValue: oldValue, NewValue: newValue, Delta: deltaOf(oldValue, newValue),
	}
	records := b.bus.Fire(eventName, ctx)
	b.mu.Lock()
	b.records = records
	b.mu.Unlock()
}

func (b *Bridge) schemaFor(typeName string) *entity.EntitySchema {
	b.mu.RLock()
	defer b.mu.RUnlock()
	merged := entity.NewEntitySchema()
	found := false
	for _, name := range entity.TypeChain(typeName) {
		schema, ok := b.schemas[name]
		if !ok {
			continue
		}
		found = true
		for path, field := range schema.All() {
			merged.Add(path, field)
		}
	}
	if !found {
		return nil
	}
	return merged
}

func deltaOf(oldValue, newValue any) any {
	oldNumber, oldOK := numeric(oldValue)
	newNumber, newOK := numeric(newValue)
	if !oldOK || !newOK {
		return nil
	}
	return newNumber - oldNumber
}

func numeric(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case int32:
		return float64(typed), true
	}
	return 0, false
}
