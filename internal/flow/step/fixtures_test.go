// fixtures_test.go provides the shared flow and drivers used by the step tests.
package step

import (
	"testing"

	"github.com/RuiCat/Neko/internal/flow/spec"
)

// newFlowFixture returns a three-step flow with a template cost. The cost is
// only observed by the interrupt/resume ledger test; this package never settles.
func newFlowFixture() *spec.FlowSpec {
	return &spec.FlowSpec{
		ID:    "做饭",
		Steps: []string{"洗菜", "切菜", "下锅"},
		Cost:  20,
	}
}

// mustStart starts a fixture execution at tick 1 or fails the test.
func mustStart(t *testing.T, flow *spec.FlowSpec) State {
	t.Helper()
	state, err := Start(flow, "exec-fixture", 1)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	return state
}

// mustNext advances one step or fails the test.
func mustNext(t *testing.T, flow *spec.FlowSpec, state State, minutes int) State {
	t.Helper()
	next, err := Next(flow, state, minutes)
	if err != nil {
		t.Fatalf("Next(%s, %d): %v", state.Status, minutes, err)
	}
	return next
}

// mustInterrupt parks an execution after its first Next or fails the test.
func mustInterrupt(t *testing.T, flow *spec.FlowSpec) State {
	t.Helper()
	state, err := Interrupt(mustNext(t, flow, mustStart(t, flow), 10))
	if err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	return state
}

// mustFinish drives a fresh execution to done or fails the test.
func mustFinish(t *testing.T, flow *spec.FlowSpec) State {
	t.Helper()
	state := mustStart(t, flow)
	for steps := 0; state.Status != StatusDone; steps++ {
		if steps > len(flow.Steps)+1 {
			t.Fatalf("execution did not finish after %d steps", steps)
		}
		state = mustNext(t, flow, state, 1)
	}
	return state
}
