// context.go is the bounded, decaying private context every expert owns.
package expert

import "sync"

// DEFAULT_CONTEXT_CAPACITY is the per-expert working-memory bound.
const DEFAULT_CONTEXT_CAPACITY = 8

// MIN_WEIGHT drops entries that decayed below usefulness.
const MIN_WEIGHT = 0.05

// Context is one expert's isolated working memory. Oldest entries are evicted
// first, so a context can never grow into a second vector store.
type Context struct {
	mu       sync.Mutex
	capacity int
	entries  []Entry
}

// NewContext returns a context with the given capacity (≤0 uses the default).
func NewContext(capacity int) *Context {
	if capacity <= 0 {
		capacity = DEFAULT_CONTEXT_CAPACITY
	}
	return &Context{capacity: capacity}
}

// Remember appends one entry and evicts the oldest when full.
func (c *Context) Remember(entry Entry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, entry)
	if len(c.entries) > c.capacity {
		c.entries = c.entries[len(c.entries)-c.capacity:]
	}
}

// Recent returns up to limit newest entries (oldest first). limit ≤ 0 returns
// everything currently held.
func (c *Context) Recent(limit int) []Entry {
	c.mu.Lock()
	defer c.mu.Unlock()
	start := 0
	if limit > 0 && len(c.entries) > limit {
		start = len(c.entries) - limit
	}
	return append([]Entry(nil), c.entries[start:]...)
}

// Decay multiplies every weight by factor and drops entries below MIN_WEIGHT.
// It is called by the nightly pass; the online path never decays.
func (c *Context) Decay(factor float64) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	kept := make([]Entry, 0, len(c.entries))
	for _, entry := range c.entries {
		entry.Weight *= factor
		if entry.Weight >= MIN_WEIGHT {
			kept = append(kept, entry)
		}
	}
	removed := len(c.entries) - len(kept)
	c.entries = kept
	return removed
}

// Len reports how many entries are held.
func (c *Context) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// Replace swaps the whole content (used by the persistence loader) and keeps
// the capacity bound by dropping the oldest entries.
func (c *Context) Replace(entries []Entry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(entries) > c.capacity {
		entries = entries[len(entries)-c.capacity:]
	}
	c.entries = append([]Entry(nil), entries...)
}
