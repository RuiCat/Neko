// budget.go caps how many tokens one process may spend on remote calls.
package llm

import "sync"

// Budget tracks token usage; a limit ≤ 0 means unlimited.
type Budget struct {
	mu    sync.Mutex
	limit int64
	used  int64
}

// NewBudget returns a budget with the given token limit.
func NewBudget(limit int64) *Budget {
	return &Budget{limit: limit}
}

// Check rejects a new call when the budget is already exhausted.
func (b *Budget) Check() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.limit > 0 && b.used >= b.limit {
		return ErrBudgetExceeded
	}
	return nil
}

// Add records consumed tokens.
func (b *Budget) Add(tokens int64) {
	if b == nil || tokens <= 0 {
		return
	}
	b.mu.Lock()
	b.used += tokens
	b.mu.Unlock()
}

// Used reports consumed tokens.
func (b *Budget) Used() int64 {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used
}
