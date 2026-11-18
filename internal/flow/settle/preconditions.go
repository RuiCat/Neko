// preconditions.go checks the K4 inputs before any planning or write. A flow is
// settled only when K2 accepted it, K3 reached done for the same flow, and the
// dependency seams are present.
package settle

import (
	"fmt"
	"strings"

	"github.com/RuiCat/Neko/internal/flow/condition"
	"github.com/RuiCat/Neko/internal/flow/spec"
	"github.com/RuiCat/Neko/internal/flow/step"
)

// checkPreconditions enforces K4's input contract and returns one refusal
// error for the first broken precondition. The caller never plans or writes
// when this returns an error.
func checkPreconditions(flow spec.FlowSpec, verdict condition.Verdict, state step.State, deps Deps) error {
	switch {
	case strings.TrimSpace(flow.ID) == "":
		return fmt.Errorf("flow id is empty")
	case !verdict.OK:
		return fmt.Errorf("flow %q: verdict is not ok: %s", flow.ID, reasonsText(verdict.Reasons))
	case state.Status != step.StatusDone:
		return fmt.Errorf("flow %q: execution is %s, want %s", flow.ID, state.Status, step.StatusDone)
	case state.FlowID != flow.ID:
		return fmt.Errorf("flow %q: execution belongs to flow %q", flow.ID, state.FlowID)
	case deps.Entity == "":
		return fmt.Errorf("flow %q: entity is empty", flow.ID)
	case deps.Snapshot == nil:
		return fmt.Errorf("flow %q: nil snapshot", flow.ID)
	case deps.Store == nil:
		return fmt.Errorf("flow %q: nil store", flow.ID)
	case deps.Ledger == nil:
		return fmt.Errorf("flow %q: nil ledger", flow.ID)
	}
	return nil
}

// reasonsText renders K2 reasons compactly for one report error.
func reasonsText(reasons []condition.Reason) string {
	if len(reasons) == 0 {
		return "no reasons"
	}
	parts := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		parts = append(parts, fmt.Sprintf("%s@%s want=%s got=%s", reason.Code, reason.Path, reason.Want, reason.Got))
	}
	return strings.Join(parts, "; ")
}
