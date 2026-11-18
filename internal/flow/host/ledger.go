// ledger.go is the durable single-settlement ledger. It stores one record kind
// keyed by (flowID, entity) and exposes the settle.Ledger seam. With a path the
// ledger reloads across process restarts; without one it lives in memory only.
package host

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/RuiCat/Neko/internal/flow/settle"
)

// LEDGER_VERSION is the on-disk ledger format version. Adding record kinds is
// compatible; re-typing or removing one is not.
const LEDGER_VERSION = 1

// ledgerKey is the K4 settlement identity: one flow on one entity.
type ledgerKey struct {
	flowID string
	entity string
}

// Ledger records the single settlement of every (flowID, entity) pair. It is safe for concurrent lookups and records; the
// engine's tick gate still serializes the Lookup+apply+Record window.
//
// Ownership: a file-backed ledger belongs to one process. The file is rewritten
// atomically but not locked, so two processes sharing one path would clobber each
// other's records; a multi-process host must put the ledger behind its own lock
// or a real database.
type Ledger struct {
	mu          sync.Mutex
	path        string
	settlements map[ledgerKey]settle.Completion
	persistErr  error
}

// ledgerFile is the on-disk shape. Records are sorted on write so two hosts with
// the same records produce the same bytes.
type ledgerFile struct {
	Version     int                 `json:"version"`
	Settlements []settle.Completion `json:"settlements"`
}

// NewLedger returns an empty in-memory ledger. A host that must survive a
// restart opens a file-backed one with OpenLedger instead.
func NewLedger() *Ledger {
	return &Ledger{settlements: make(map[ledgerKey]settle.Completion)}
}

// OpenLedger opens the ledger stored at path, creating an empty one when the
// file does not exist yet. Every Record is persisted before it returns, so a
// reopened ledger answers the same lookups.
func OpenLedger(path string) (*Ledger, error) {
	if path == "" {
		return nil, fmt.Errorf("ledger path is empty")
	}
	ledger := NewLedger()
	ledger.path = path
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ledger, nil
		}
		return nil, fmt.Errorf("read ledger %s: %w", path, err)
	}
	var file ledgerFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("decode ledger %s: %w", path, err)
	}
	if file.Version != LEDGER_VERSION {
		return nil, fmt.Errorf("ledger %s has version %d, want %d", path, file.Version, LEDGER_VERSION)
	}
	for _, completion := range file.Settlements {
		ledger.settlements[ledgerKey{flowID: completion.FlowID, entity: completion.Entity}] = completion
	}
	return ledger, nil
}

// SettlementLedger returns the settle.Ledger seam of this ledger.
func (l *Ledger) SettlementLedger() settle.Ledger {
	return settlementSeam{ledger: l}
}

// PersistError reports the first persistence failure of a Record. Record itself
// cannot return an error because both seam interfaces return nothing, so a host
// must check this after settling or advancing; a non-nil result means the record
// is only in memory.
func (l *Ledger) PersistError() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.persistErr
}

// lookupSettlement returns the recorded settlement of one flow on one entity.
func (l *Ledger) lookupSettlement(flowID, entity string) (settle.Completion, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	completion, ok := l.settlements[ledgerKey{flowID: flowID, entity: entity}]
	return completion, ok
}

// recordSettlement stores one completion and persists the ledger.
func (l *Ledger) recordSettlement(completion settle.Completion) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.settlements[ledgerKey{flowID: completion.FlowID, entity: completion.Entity}] = completion
	l.notePersistLocked()
}

// notePersistLocked keeps the first persistence failure. Callers must hold mu.
func (l *Ledger) notePersistLocked() {
	if l.path == "" {
		return
	}
	if err := l.persistLocked(); err != nil && l.persistErr == nil {
		l.persistErr = err
	}
}

// persistLocked writes the whole ledger through a temp file and a rename, so a
// crash cannot leave a half-written ledger. Callers must hold mu.
func (l *Ledger) persistLocked() error {
	file := l.fileLocked()
	raw, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("encode ledger: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return fmt.Errorf("create ledger directory: %w", err)
	}
	tmp := l.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("write ledger: %w", err)
	}
	if err := os.Rename(tmp, l.path); err != nil {
		return fmt.Errorf("replace ledger: %w", err)
	}
	return nil
}

// fileLocked renders the sorted on-disk view. Callers must hold mu.
func (l *Ledger) fileLocked() ledgerFile {
	file := ledgerFile{
		Version:     LEDGER_VERSION,
		Settlements: make([]settle.Completion, 0, len(l.settlements)),
	}
	for _, completion := range l.settlements {
		file.Settlements = append(file.Settlements, completion)
	}
	sort.Slice(file.Settlements, func(left, right int) bool {
		if file.Settlements[left].FlowID != file.Settlements[right].FlowID {
			return file.Settlements[left].FlowID < file.Settlements[right].FlowID
		}
		return file.Settlements[left].Entity < file.Settlements[right].Entity
	})
	return file
}

// settlementSeam adapts the durable ledger to settle.Ledger.
type settlementSeam struct {
	ledger *Ledger
}

// Lookup returns the recorded completion of one (flowID, entity).
func (s settlementSeam) Lookup(flowID, entity string) (settle.Completion, bool) {
	return s.ledger.lookupSettlement(flowID, entity)
}

// Record stores one completion.
func (s settlementSeam) Record(completion settle.Completion) {
	s.ledger.recordSettlement(completion)
}

// Settlement seam conformance.
var _ settle.Ledger = settlementSeam{}
