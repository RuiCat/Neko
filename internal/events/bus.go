// Package events connects field changes to @事件 definitions and @函数 handlers.
package events

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ruicat/astrbox/internal/engine"
	"github.com/ruicat/astrbox/internal/entity"
	"github.com/ruicat/astrbox/internal/neko"
)

// Caller is the subset of the function executor the bus needs.
type Caller interface {
	Call(name string, args []any, current *entity.Entity) (any, error)
}

// Context is one field-change event payload.
type Context struct {
	EventName string
	EntityID  string
	FieldPath string
	OldValue  any
	NewValue  any
	Delta     any
}

// EventRecord is one structured entry in the bounded dispatch history.
type EventRecord struct {
	Name    string    `json:"name"`
	Entity  string    `json:"entity,omitempty"`
	Field   string    `json:"field,omitempty"`
	Status  string    `json:"status"`
	Message string    `json:"message"`
	Time    time.Time `json:"time"`
}

// Bus holds registered events and dispatches them to handlers.
type Bus struct {
	mu           sync.RWMutex
	store        *entity.Store
	caller       Caller
	defs         map[string]neko.EventDef
	depth        int
	history      []EventRecord
	historyLimit int
}

// NewBus creates an event bus bound to a store and a function caller.
func NewBus(store *entity.Store, caller Caller) *Bus {
	return &Bus{store: store, caller: caller, defs: map[string]neko.EventDef{}, historyLimit: 128}
}

// Register adds or replaces event definitions.
func (b *Bus) Register(defs []neko.EventDef) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, def := range defs {
		// A conditional definition wins over an unconditional one with the same
		// name; mirrors the Python EventSink conflict rule.
		if existing, ok := b.defs[def.Name]; ok {
			if existing.Condition != "" && def.Condition == "" {
				continue
			}
		}
		b.defs[def.Name] = def
	}
}

// Unregister removes event definitions by name.
func (b *Bus) Unregister(names []string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, name := range names {
		delete(b.defs, name)
	}
}

// Get returns one event definition.
func (b *Bus) Get(name string) (neko.EventDef, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	def, ok := b.defs[name]
	return def, ok
}

// Fire dispatches one event and returns human-readable execution records.
func (b *Bus) Fire(name string, ctx Context) []string {
	records := b.fire(name, ctx)
	b.appendHistory(name, ctx, records)
	return records
}

// History returns at most limit recent dispatch records, oldest first.
func (b *Bus) History(limit int) []EventRecord {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if limit <= 0 || limit > len(b.history) {
		limit = len(b.history)
	}
	out := make([]EventRecord, limit)
	copy(out, b.history[len(b.history)-limit:])
	return out
}

// appendHistory stores one structured dispatch record in the bounded ring.
func (b *Bus) appendHistory(name string, ctx Context, records []string) {
	if len(records) == 0 {
		return
	}
	status := "ok"
	switch {
	case strings.HasPrefix(records[0], "⚠️"):
		status = "warning"
	case strings.HasPrefix(records[0], "⏭️"):
		status = "skipped"
	case strings.HasPrefix(records[0], "ℹ️"):
		status = "info"
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.history = append(b.history, EventRecord{
		Name: name, Entity: ctx.EntityID, Field: ctx.FieldPath,
		Status: status, Message: strings.Join(records, " | "), Time: time.Now(),
	})
	if len(b.history) > b.historyLimit {
		b.history = append([]EventRecord(nil), b.history[1:]...)
	}
}

// fire is the dispatch implementation shared by Fire.
func (b *Bus) fire(name string, ctx Context) []string {
	b.mu.Lock()
	if b.depth >= 10 {
		b.mu.Unlock()
		return []string{"⚠️ 事件触发过深（> 10），疑似循环: " + name}
	}
	def, ok := b.defs[name]
	if !ok {
		b.mu.Unlock()
		return []string{fmt.Sprintf("⚠️ 事件 '%s' 未注册（字段有 @绑定但无定义）", name)}
	}
	b.depth++
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		b.depth--
		b.mu.Unlock()
	}()

	if def.Condition != "" && !b.evalCondition(def.Condition, ctx) {
		return []string{fmt.Sprintf("⏭️ 事件 '%s' 条件不满足: %s", name, def.Condition)}
	}
	handler := def.Handler
	if handler == "" {
		handler = def.Name
	}
	if b.caller == nil {
		return []string{fmt.Sprintf("ℹ️ 事件 '%s' 无执行器", name)}
	}
	current := b.store.Get(ctx.EntityID)
	result, err := b.caller.Call(handler, []any{ctx.Delta}, current)
	if err != nil {
		return []string{fmt.Sprintf("⚠️ 事件 '%s' 处理器 '%s' 失败: %v", name, handler, err)}
	}
	return []string{fmt.Sprintf("✅ 事件 '%s' → %s: %v", name, handler, result)}
}

func (b *Bus) evalCondition(condition string, ctx Context) bool {
	entityID := ctx.EntityID
	resolver := func(path string) (any, error) {
		switch path {
		case "变化量":
			return ctx.Delta, nil
		case "旧值":
			return ctx.OldValue, nil
		case "新值":
			return ctx.NewValue, nil
		case "实体":
			return entityID, nil
		case "字段":
			return ctx.FieldPath, nil
		}
		ent := b.store.Get(entityID)
		if ent == nil {
			return nil, fmt.Errorf("实体 '%s' 不存在", entityID)
		}
		return ent.Get(path)
	}
	ok, err := engine.EvaluateCondition(condition, resolver)
	if err != nil {
		return false
	}
	return ok
}
