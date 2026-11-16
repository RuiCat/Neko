// start_test.go covers Start's K3 guard rails and its idempotent pending state.
package step

import (
	"errors"
	"reflect"
	"testing"

	"github.com/RuiCat/Neko/internal/flow/spec"
)

func TestStartPendingState(t *testing.T) {
	flow := newFlowFixture()
	state, err := Start(flow, "exec-1", 42)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if state.ExecutionID != "exec-1" {
		t.Fatalf("ExecutionID = %q, want exec-1", state.ExecutionID)
	}
	if state.FlowID != flow.ID {
		t.Fatalf("FlowID = %q, want %q", state.FlowID, flow.ID)
	}
	if state.StepIndex != 0 {
		t.Fatalf("StepIndex = %d, want 0", state.StepIndex)
	}
	if state.Status != StatusPending {
		t.Fatalf("Status = %s, want %s", state.Status, StatusPending)
	}
	if state.MinutesSpent != 0 {
		t.Fatalf("MinutesSpent = %d, want 0", state.MinutesSpent)
	}
	if state.StartedTick != 42 {
		t.Fatalf("StartedTick = %d, want 42", state.StartedTick)
	}
}

func TestStartRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name        string
		flow        *spec.FlowSpec
		executionID string
		want        error
	}{
		{"nil flow", nil, "exec-1", ErrInvalidFlow},
		{"empty flow id", &spec.FlowSpec{Steps: []string{"一步"}}, "exec-1", ErrInvalidFlow},
		{"empty execution id", newFlowFixture(), "", ErrEmptyExecutionID},
		{"flow without steps", &spec.FlowSpec{ID: "空流程"}, "exec-1", ErrStepOutOfRange},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Start(tc.flow, tc.executionID, 1); !errors.Is(err, tc.want) {
				t.Fatalf("Start error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestStartIsIdempotent(t *testing.T) {
	flow := newFlowFixture()
	first, err := Start(flow, "exec-1", 42)
	if err != nil {
		t.Fatalf("first Start: %v", err)
	}
	second, err := Start(flow, "exec-1", 42)
	if err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated Start differs: %+v != %+v", first, second)
	}
}

func TestCheckExecution(t *testing.T) {
	flow := newFlowFixture()
	state := mustStart(t, flow)
	if err := state.CheckExecution(state.ExecutionID); err != nil {
		t.Fatalf("CheckExecution(same) = %v, want nil", err)
	}
	if err := state.CheckExecution("other"); !errors.Is(err, ErrStaleExecution) {
		t.Fatalf("CheckExecution(other) = %v, want ErrStaleExecution", err)
	}
}
