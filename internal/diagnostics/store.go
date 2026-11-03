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
	// notifyMu guards notify; notify callbacks always run outside mu.
	notifyMu sync.RWMutex
	notify   func(record Record, op string)
}

// New builds a store with the given capacity; non-positive values use the default.
func New(capacity int) *Store {
	if capacity <= 0 {
		capacity = DEFAULT_CAPACITY
	}
	return &Store{capacity: capacity, index: map[string]int{}}
}

// SetNotify registers a mutation observer. added/updated fire after one Add
// (updated when the fingerprint already existed), removed fires for a cleared
// or evicted record. The callback runs after Store.mu is released, so it may
// call Generation/Snapshot/Summary/Len; it must not call Add.
func (s *Store) SetNotify(notify func(record Record, op string)) {
	s.notifyMu.Lock()
	s.notify = notify
	s.notifyMu.Unlock()
}

// Add inserts or updates one diagnostic and returns its stable id.
func (s *Store) Add(record Record) string {
	record = record.normalize()
	key := record.fingerprint()
	now := time.Now()

	s.mu.Lock()
	s.generation++
	op := "added"
	var out Record
	var evicted *Record
	if position, ok := s.index[key]; ok {
		existing := &s.records[position]
		existing.Count++
		existing.LastSeen = now
		existing.Generation = s.generation
		out = *existing
		op = "updated"
	} else {
		record.ID = "diag_" + key[:12]
		record.Count = 1
		record.FirstSeen = now
		record.LastSeen = now
		record.Generation = s.generation
		s.records = append(s.records, record)
		s.index[key] = len(s.records) - 1
		if len(s.records) > s.capacity {
			if dropped, ok := s.evictOldestLocked(); ok {
				evicted = &dropped
			}
		}
		out = record
	}
	s.mu.Unlock()

	if evicted != nil {
		s.notifyRecord(*evicted, "removed")
	}
	s.notifyRecord(out, op)
	return out.ID
}

// evictOldestLocked drops the oldest record and rebuilds the position index,
// returning the evicted record for the out-of-lock notification.
func (s *Store) evictOldestLocked() (Record, bool) {
	if len(s.records) == 0 {
		return Record{}, false
	}
	dropped := s.records[0]
	s.records = append([]Record(nil), s.records[1:]...)
	s.index = make(map[string]int, len(s.records))
	for position, record := range s.records {
		s.index[record.fingerprint()] = position
	}
	s.overflow++
	return dropped, true
}

// notifyRecord runs one notify callback after the store lock was released.
func (s *Store) notifyRecord(record Record, op string) {
	s.notifyMu.RLock()
	callback := s.notify
	s.notifyMu.RUnlock()
	if callback == nil {
		return
	}
	callback(record, op)
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

// clear drops matching records and rebuilds the position index. Removed
// records are reported to the notify callback after the lock is released.
func (s *Store) clear(match func(Record) bool) {
	s.mu.Lock()
	kept := make([]Record, 0, len(s.records))
	removed := make([]Record, 0)
	for _, record := range s.records {
		if match(record) {
			removed = append(removed, record)
			continue
		}
		kept = append(kept, record)
	}
	s.records = kept
	s.index = make(map[string]int, len(kept))
	for position, record := range kept {
		s.index[record.fingerprint()] = position
	}
	s.generation++
	s.mu.Unlock()

	for _, record := range removed {
		s.notifyRecord(record, "removed")
	}
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
