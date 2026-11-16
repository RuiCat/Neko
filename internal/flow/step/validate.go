// validate.go centralizes the K3 input guards shared by Start and Next.
package step

import (
	"fmt"

	"github.com/RuiCat/Neko/internal/flow/spec"
)

// validateFlow rejects a nil or unidentified flow before any state is built.
func validateFlow(flow *spec.FlowSpec) error {
	if flow == nil {
		return fmt.Errorf("%w: nil flow spec", ErrInvalidFlow)
	}
	if flow.ID == "" {
		return fmt.Errorf("%w: empty flow id", ErrInvalidFlow)
	}
	return nil
}

// validateStepIndex rejects a K3 out-of-range step number. A step index is
// valid only while it names one of the flow's steps.
func validateStepIndex(state State, stepCount int) error {
	if state.StepIndex < 0 || state.StepIndex >= stepCount {
		return fmt.Errorf("%w: step %d of %d", ErrStepOutOfRange, state.StepIndex, stepCount)
	}
	return nil
}
