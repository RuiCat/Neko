// bus.go keeps the append-only observation streams in memory: one ring per
// scope, one monotonically increasing seq per scope and no de-duplication by
// path (folding happens at read time).
package observation

import (
	"sort"
	"sync"
	"time"
)

const (
	// STREAM_CAPACITY bounds how many events one scope keeps (newest win).
	STREAM_CAPACITY = 512
	// MAX_SCOPES bounds how many scopes stay resident; the least recently
	// written scope is evicted first.
	MAX_SCOPES = 256
)

// stream is one scope's ring of events plus its current version.
type stream struct {
	version int64
	events  []Event
}

// Bus holds every observation stream. It is safe for concurrent use.
type Bus struct {
	mu       sync.Mutex
	streams  map[string]*stream
	order    []string
	capacity int
	maxScope int
	now      func() time.Time
}

// NewBus builds an empty bus with the default capacities.
func NewBus() *Bus {
	return &Bus{
		streams:  map[string]*stream{},
		order:    []string{},
		capacity: STREAM_CAPACITY,
		maxScope: MAX_SCOPES,
		now:      time.Now,
	}
}

// Append stores one event and returns it with its assigned seq and timestamp.
// The scope's version becomes the new seq.
func (b *Bus) Append(event Event) Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	event = b.appendLocked(event)
	return event
}

// AppendAll stores a batch under one lock. Events keep their given order.
func (b *Bus) AppendAll(events []Event) []Event {
	if len(events) == 0 {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	stored := make([]Event, 0, len(events))
	for _, event := range events {
		stored = append(stored, b.appendLocked(event))
	}
	return stored
}

// appendLocked assigns the seq/timestamp and writes the event into its stream.
// Callers must hold b.mu.
func (b *Bus) appendLocked(event Event) Event {
	if event.At == "" {
		event.At = b.now().UTC().Format(time.RFC3339)
	}
	current, exists := b.streams[event.Scope]
	if !exists {
		current = &stream{events: make([]Event, 0, b.capacity)}
		b.streams[event.Scope] = current
		b.order = append(b.order, event.Scope)
		b.evictScopesLocked()
	}
	current.version++
	event.Seq = current.version
	// The tier is derived at read time from the current policy.
	event.Tier = ""
	current.events = append(current.events, event)
	if len(current.events) > b.capacity {
		trimmed := make([]Event, b.capacity)
		copy(trimmed, current.events[len(current.events)-b.capacity:])
		current.events = trimmed
	}
	b.touchLocked(event.Scope)
	return event
}

// evictScopesLocked drops the least recently written scopes past the cap.
// Callers must hold b.mu.
func (b *Bus) evictScopesLocked() {
	for len(b.order) > b.maxScope {
		oldest := b.order[0]
		b.order = b.order[1:]
		delete(b.streams, oldest)
	}
}

// touchLocked moves one scope to the most-recently-written position.
func (b *Bus) touchLocked(scope string) {
	for index, candidate := range b.order {
		if candidate != scope {
			continue
		}
		copy(b.order[index:], b.order[index+1:])
		b.order[len(b.order)-1] = scope
		return
	}
	b.order = append(b.order, scope)
}

// Stream returns the stored events with seq > since, oldest first. A limit of
// zero or less means "no limit".
func (b *Bus) Stream(scope string, since int64, limit int) []Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	current := b.streams[scope]
	if current == nil {
		return nil
	}
	result := make([]Event, 0, len(current.events))
	for _, event := range current.events {
		if event.Seq <= since {
			continue
		}
		result = append(result, event)
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result
}

// Version returns the current seq of one scope (0 for an unknown scope).
func (b *Bus) Version(scope string) int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if current := b.streams[scope]; current != nil {
		return current.version
	}
	return 0
}

// Scopes lists every scope that has events, sorted.
func (b *Bus) Scopes() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	result := make([]string, 0, len(b.streams))
	for scope := range b.streams {
		result = append(result, scope)
	}
	sort.Strings(result)
	return result
}

// ScopesForEntity lists the scopes that carry one entity's events.
func (b *Bus) ScopesForEntity(entityID string) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	scope := ScopeForEntity(entityID)
	if _, exists := b.streams[scope]; !exists {
		return []string{scope}
	}
	return []string{scope}
}

// Delete drops one scope's whole history (an entity is gone).
func (b *Bus) Delete(scope string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, exists := b.streams[scope]; !exists {
		return
	}
	delete(b.streams, scope)
	for index, candidate := range b.order {
		if candidate == scope {
			b.order = append(b.order[:index], b.order[index+1:]...)
			return
		}
	}
}
