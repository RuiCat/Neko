// ledger_test.go pins the durable settlement ledger: a file-backed record
// survives a reopen and a corrupt file is refused instead of being reset.
package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/RuiCat/Neko/internal/flow/settle"
)

// TestLedgerMemoryAndFileRecords exercises the memory ledger, the file ledger and
// the reopen path.
func TestLedgerMemoryAndFileRecords(t *testing.T) {
	completion := settle.Completion{
		FlowID: "做饭", Entity: testActorID, Result: settle.RESULT_COMPLETED, Tick: 7,
	}

	memory := NewLedger()
	if _, ok := memory.SettlementLedger().Lookup("做饭", testActorID); ok {
		t.Fatal("empty memory ledger returned a settlement")
	}
	memory.SettlementLedger().Record(completion)
	if err := memory.PersistError(); err != nil {
		t.Fatalf("memory ledger persistence: %v", err)
	}
	if got, ok := memory.SettlementLedger().Lookup("做饭", testActorID); !ok || got != completion {
		t.Fatalf("memory settlement = %+v, ok = %v", got, ok)
	}

	path := filepath.Join(t.TempDir(), "nested", "ledger.json")
	file, err := OpenLedger(path)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	file.SettlementLedger().Record(completion)
	if err := file.PersistError(); err != nil {
		t.Fatalf("file ledger persistence: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("ledger file: %v", err)
	}
	reopened, err := OpenLedger(path)
	if err != nil {
		t.Fatalf("reopen ledger: %v", err)
	}
	if got, ok := reopened.SettlementLedger().Lookup("做饭", testActorID); !ok || got != completion {
		t.Fatalf("reopened settlement = %+v, ok = %v", got, ok)
	}
	if _, ok := reopened.SettlementLedger().Lookup("做饭", "另一只猫"); ok {
		t.Fatal("ledger keyed a record to the wrong entity")
	}
	if _, err := OpenLedger(""); err == nil {
		t.Fatal("OpenLedger accepted an empty path")
	}
	corrupt := filepath.Join(t.TempDir(), "corrupt.json")
	if err := os.WriteFile(corrupt, []byte("not json"), 0o644); err != nil {
		t.Fatalf("write corrupt ledger: %v", err)
	}
	if _, err := OpenLedger(corrupt); err == nil {
		t.Fatal("OpenLedger accepted a corrupt ledger")
	}
}
