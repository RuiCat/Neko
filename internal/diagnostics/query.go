package diagnostics

import (
	"time"
)

// Filter selects diagnostics for queries. Zero values match everything.
type Filter struct {
	Severity Severity  `json:"severity,omitempty"`
	Source   string    `json:"source,omitempty"`
	Entity   string    `json:"entity,omitempty"`
	Script   string    `json:"script,omitempty"`
	Code     string    `json:"code,omitempty"`
	Since    time.Time `json:"since,omitempty"`
}

// match reports whether one record passes the filter.
func (f Filter) match(record Record) bool {
	if f.Severity != "" && record.Severity != f.Severity {
		return false
	}
	if f.Source != "" && record.Source != f.Source {
		return false
	}
	if f.Entity != "" && record.Entity != f.Entity {
		return false
	}
	if f.Script != "" && record.Script != f.Script {
		return false
	}
	if f.Code != "" && record.Code != f.Code {
		return false
	}
	if !f.Since.IsZero() && record.LastSeen.Before(f.Since) {
		return false
	}
	return true
}

// Summary counts diagnostics by severity.
type Summary struct {
	Total    int `json:"total"`
	Error    int `json:"error"`
	Warning  int `json:"warning"`
	Info     int `json:"info"`
	Overflow int `json:"overflow,omitempty"`
}

// Scope is the bounded diagnostic block attached to one entity response.
type Scope struct {
	Generation uint64   `json:"generation"`
	Summary    Summary  `json:"summary"`
	Items      []Record `json:"items"`
	Truncated  bool     `json:"truncated"`
}

// ForEntity returns diagnostics relevant to one entity: records addressed to it,
// records addressed to its type, and global errors. The item list is capped.
func (s *Store) ForEntity(entity, typeName string, top int) Scope {
	if top <= 0 {
		top = 3
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	matched := make([]Record, 0)
	summary := Summary{Overflow: s.overflow}
	for _, record := range s.records {
		if !scopeMatch(record, entity, typeName) {
			continue
		}
		summary.Total++
		switch record.Severity {
		case SEVERITY_ERROR:
			summary.Error++
		case SEVERITY_WARNING:
			summary.Warning++
		default:
			summary.Info++
		}
		matched = append(matched, record)
	}
	sortRecords(matched)
	truncated := len(matched) > top
	if truncated {
		matched = matched[:top]
	}
	return Scope{
		Generation: s.generation,
		Summary:    summary,
		Items:      matched,
		Truncated:  truncated,
	}
}

// scopeMatch decides whether a record is relevant to one entity query.
func scopeMatch(record Record, entity, typeName string) bool {
	if entity != "" && record.Entity == entity {
		return true
	}
	if typeName != "" && record.Type == typeName {
		return true
	}
	// Global warnings and errors are engine-wide and worth surfacing on any
	// state read; info-level noise stays out.
	return record.Entity == "" && record.Type == "" && record.Severity != SEVERITY_INFO
}
