// settle.go is the K4 entry point: it refuses rejected/unfinished executions
// and applies a fully validated settlement as exactly one atomic write set.
package settle

import (
	"fmt"

	"github.com/RuiCat/Neko/internal/flow/condition"
	"github.com/RuiCat/Neko/internal/flow/spec"
	"github.com/RuiCat/Neko/internal/flow/step"
)

// Settle turns one finished execution into a K4 Completion and applies the
// flow's trade request, consumed/produced items and effects. Settlement is
// refused with a Report error when the K2 verdict is not ok, the K3 state is
// not done, or any planned write targets a non-source path. A replay of an
// already settled (flowID, entity) returns the recorded Completion, an empty
// change list and no write call. All writes are validated first and applied
// through one Store.Apply call, so a refusal leaves zero writes behind.
func Settle(flow spec.FlowSpec, verdict condition.Verdict, state step.State, deps Deps) (Completion, Report) {
	if err := checkPreconditions(flow, verdict, state, deps); err != nil {
		return Completion{}, refusal(err)
	}
	if previous, settled := deps.Ledger.Lookup(flow.ID, deps.Entity); settled {
		return previous, Report{Changes: []Change{}, Errors: []error{}}
	}
	changes, errs := planChanges(flow, deps)
	if len(errs) > 0 {
		return Completion{}, Report{Changes: []Change{}, Errors: errs}
	}
	if err := deps.Store.Apply(flow.ID, changes); err != nil {
		return Completion{}, refusal(fmt.Errorf("flow %q: store: %w", flow.ID, err))
	}
	completion := Completion{
		FlowID: flow.ID,
		Entity: deps.Entity,
		Result: RESULT_COMPLETED,
		Tick:   deps.Tick,
	}
	deps.Ledger.Record(completion)
	return completion, Report{Changes: changes, Errors: []error{}}
}

// planChanges builds and validates the whole write set. Errors are collected
// across every category so one report lists every offending row; the returned
// changes are only applied when errs is empty.
func planChanges(flow spec.FlowSpec, deps Deps) ([]Change, []error) {
	var changes []Change
	var errs []error
	spend, err := planSpend(flow, deps.Entity)
	if err != nil {
		errs = append(errs, err)
	}
	changes = append(changes, spend...)
	consumed, consumeErrs := planConsume(flow, deps.Snapshot)
	changes = append(changes, consumed...)
	errs = append(errs, consumeErrs...)
	produced, produceErrs := planProduce(flow)
	changes = append(changes, produced...)
	errs = append(errs, produceErrs...)
	effects, effectErrs := planEffects(deps.Entity, flow.Effects)
	changes = append(changes, effects...)
	errs = append(errs, effectErrs...)
	return changes, errs
}

// planSpend converts a positive flow cost into trade-kernel request rows. The
// currency field is deliberately never assigned here (C5): the trade tick owns
// the actual balance change.
func planSpend(flow spec.FlowSpec, entity string) ([]Change, error) {
	if flow.Cost < 0 {
		return nil, fmt.Errorf("flow %q: negative cost %d", flow.ID, flow.Cost)
	}
	if flow.Cost == 0 {
		return nil, nil
	}
	return []Change{
		{Op: OP_SPEND, Entity: entity, Path: PATH_TRADE_AMOUNT, Kind: spec.EFFECT_KIND_SET, Value: flow.Cost},
		{Op: OP_SPEND, Entity: entity, Path: PATH_TRADE_RESULT, Kind: spec.EFFECT_KIND_TEXT, Value: RESULT_SPENT},
	}, nil
}

// refusal wraps one refusal error into a K4 report.
func refusal(err error) Report {
	return Report{Changes: []Change{}, Errors: []error{err}}
}
