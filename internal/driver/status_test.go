// Tests for session/list status reconciliation.
package driver

import (
	"context"
	"testing"
)

// summaryWithTurns builds one list row with a session turn count.
func summaryWithTurns(sessionID string, running bool, turns int) SessionSummary {
	summary := SessionSummary{SessionID: sessionID, Running: running}
	summary.Projections = &SessionProjectionSet{}
	summary.Projections.Values.SessionStats.Turns = turns
	return summary
}

func TestRefreshStatusReconcilesRunning(t *testing.T) {
	fake := newFakeDSH(t)
	contextDriver, _ := newTestDriver(t, fake)
	if _, err := contextDriver.Invoke(context.Background(), InvokeRequest{
		ContextID: "ctx-cat", EventID: "e1", Text: "a",
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	record, _ := contextDriver.registry.Get("ctx-cat")
	if record.Status != STATUS_RUNNING {
		t.Fatalf("status after invoke = %q", record.Status)
	}
	fake.mu.Lock()
	fake.listItems = []SessionSummary{summaryWithTurns("dsh-session", false, 3)}
	fake.mu.Unlock()
	if err := contextDriver.RefreshStatus(context.Background()); err != nil {
		t.Fatalf("RefreshStatus: %v", err)
	}
	record, _ = contextDriver.registry.Get("ctx-cat")
	if record.Status != STATUS_IDLE {
		t.Fatalf("status after refresh = %q, want idle", record.Status)
	}
	if record.LastTurn != 3 {
		t.Fatalf("lastTurn = %d, want 3", record.LastTurn)
	}
}

func TestRefreshStatusKeepsUnknownSession(t *testing.T) {
	fake := newFakeDSH(t)
	contextDriver, _ := newTestDriver(t, fake)
	if _, err := contextDriver.Invoke(context.Background(), InvokeRequest{
		ContextID: "ctx-cat", EventID: "e1", Text: "a",
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	fake.mu.Lock()
	fake.listItems = []SessionSummary{{SessionID: "another-session", Running: false}}
	fake.mu.Unlock()
	if err := contextDriver.RefreshStatus(context.Background()); err != nil {
		t.Fatalf("RefreshStatus: %v", err)
	}
	record, _ := contextDriver.registry.Get("ctx-cat")
	if record.Status != STATUS_RUNNING {
		t.Fatalf("status = %q, want running (missing session must not rewrite)", record.Status)
	}
}

func TestRefreshStatusRunningTrue(t *testing.T) {
	fake := newFakeDSH(t)
	contextDriver, _ := newTestDriver(t, fake)
	if _, err := contextDriver.Invoke(context.Background(), InvokeRequest{
		ContextID: "ctx-cat", EventID: "e1", Text: "a",
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if err := contextDriver.HandleTurnEnd("dsh-session", 1); err != nil {
		t.Fatalf("HandleTurnEnd: %v", err)
	}
	fake.mu.Lock()
	fake.listItems = []SessionSummary{{SessionID: "dsh-session", Running: true}}
	fake.mu.Unlock()
	if err := contextDriver.RefreshStatus(context.Background()); err != nil {
		t.Fatalf("RefreshStatus: %v", err)
	}
	record, _ := contextDriver.registry.Get("ctx-cat")
	if record.Status != STATUS_RUNNING {
		t.Fatalf("status = %q, want running", record.Status)
	}
}
