package diagnostics

import (
	"sort"
	"sync"
	"time"
)

// defaultCapacity bounds the in-memory ring. Older entries are evicted first.
const DEFAULT_CAPACITY = 512

// Store is a concurrency-safe bounded ring of deduplicated diagnostics.
type Store struct {
	mu         sync.Mutex
	capacity   int
	records    []Record
	index      map[string]int
	overflow   int
	generation uint64
}

// New builds a store with the given capacity; non-positive values use the default.
func New(capacity int) *Store {
	if capacity <= 0 {
		capacity = DEFAULT_CAPACITY
	}
	return &Store{capacity: capacity, index: map[string]int{}}
}

// Add inserts or updates one diagnostic and returns its stable id.
func (s *Store) Add(record Record) string {
	record = record.normalize()
	key := record.fingerprint()
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.generation++
	if position, ok := s.index[key]; ok {
		existing := &s.records[position]
		existing.Count++
		existing.LastSeen = now
		existing.Generation = s.generation
		return existing.ID
	}
	record.ID = "diag_" + key[:12]
	record.Count = 1
	record.FirstSeen = now
	record.LastSeen = now
	record.Generation = s.generation
	s.records = append(s.records, record)
	s.index[key] = len(s.records) - 1
	if len(s.records) > s.capacity {
		s.evictOldestLocked()
	}
	return record.ID
}

// evictOldestLocked drops the oldest record and rebuilds the position index.
func (s *Store) evictOldestLocked() {
	if len(s.records) == 0 {
		return
	}
	s.records = append([]Record(nil), s.records[1:]...)
	s.index = make(map[string]int, len(s.records))
	for position, record := range s.records {
		s.index[record.fingerprint()] = position
	}
	s.overflow++
}

// ClearByScript removes every diagnostic owned by one script.
func (s *Store) ClearByScript(script string) {
	s.clear(func(record Record) bool { return record.Script == script })
}

// ClearByEntity removes every diagnostic owned by one entity.
func (s *Store) ClearByEntity(entity string) {
	s.clear(func(record Record) bool { return record.Entity == entity })
}

// ClearByCodeEntity removes diagnostics matching a code and entity.
func (s *Store) ClearByCodeEntity(code, entity string) {
	s.clear(func(record Record) bool {
		return record.Code == code && record.Entity == entity
	})
}

// ClearBySource removes every diagnostic emitted by one subsystem.
func (s *Store) ClearBySource(source string) {
	s.clear(func(record Record) bool { return record.Source == source })
}

// clear drops matching records and rebuilds the position index.
func (s *Store) clear(match func(Record) bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := make([]Record, 0, len(s.records))
	for _, record := range s.records {
		if !match(record) {
			kept = append(kept, record)
		}
	}
	s.records = kept
	s.index = make(map[string]int, len(kept))
	for position, record := range kept {
		s.index[record.fingerprint()] = position
	}
	s.generation++
}

// Snapshot returns matching records ordered by severity then recency.
func (s *Store) Snapshot(filter Filter, limit int) []Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Record, 0)
	for _, record := range s.records {
		if filter.match(record) {
			out = append(out, record)
		}
	}
	sortRecords(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Summary counts matching records plus global overflow.
func (s *Store) Summary(filter Filter) Summary {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := Summary{Overflow: s.overflow}
	for _, record := range s.records {
		if !filter.match(record) {
			continue
		}
		out.Total++
		switch record.Severity {
		case SEVERITY_ERROR:
			out.Error++
		case SEVERITY_WARNING:
			out.Warning++
		default:
			out.Info++
		}
	}
	return out
}

// Generation returns the current mutation generation.
func (s *Store) Generation() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.generation
}

// Len returns the number of retained records.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.records)
}

// sortRecords orders errors first, newest first inside one severity.
func sortRecords(records []Record) {
	sort.SliceStable(records, func(i, j int) bool {
		left, right := records[i], records[j]
		if severityRank(left.Severity) != severityRank(right.Severity) {
			return severityRank(left.Severity) < severityRank(right.Severity)
		}
		if !left.LastSeen.Equal(right.LastSeen) {
			return left.LastSeen.After(right.LastSeen)
		}
		return left.ID < right.ID
	})
}
