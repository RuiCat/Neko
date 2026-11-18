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
// flow's trade intent, interaction intent, produced items and data-layer
// effects. Settlement writes requests and data only; every computed field is
// left to the .neko tick. It is refused with a Report error when the K2 verdict
// is not ok, the K3 state is not done, or any planned write targets a computed
// or unknown path. A replay of an
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
	trade, err := planTradeIntent(flow, deps.Entity)
	if err != nil {
		errs = append(errs, err)
	}
	changes = append(changes, trade...)
	interaction, err := planInteraction(flow, deps.Entity)
	if err != nil {
		errs = append(errs, err)
	}
	changes = append(changes, interaction...)
	produced, produceErrs := planProduce(flow)
	changes = append(changes, produced...)
	errs = append(errs, produceErrs...)
	effects, effectErrs := planEffects(deps.Entity, flow.Effects)
	changes = append(changes, effects...)
	errs = append(errs, effectErrs...)
	return changes, errs
}

// planTradeIntent converts a positive flow cost into trade-kernel request
// rows. It deliberately writes no 成交额/结果: the .neko trade tick owns the
// actual balance change and result.
func planTradeIntent(flow spec.FlowSpec, entity string) ([]Change, error) {
	if flow.Cost < 0 {
		return nil, fmt.Errorf("flow %q: negative cost %d", flow.ID, flow.Cost)
	}
	if flow.Cost == 0 {
		return nil, nil
	}
	object := tradeObject(flow)
	if object == "" {
		return nil, fmt.Errorf("flow %q: cost %d has no trade object", flow.ID, flow.Cost)
	}
	return []Change{
		{Op: OP_SPEND, Entity: entity, Path: PATH_TRADE_INTENT, Kind: spec.EFFECT_KIND_TEXT, Value: TRADE_SIDE_BUY},
		{Op: OP_SPEND, Entity: entity, Path: PATH_TRADE_OBJECT, Kind: spec.EFFECT_KIND_TEXT, Value: object},
		{Op: OP_SPEND, Entity: entity, Path: PATH_TRADE_PRICE, Kind: spec.EFFECT_KIND_SET, Value: flow.Cost},
	}, nil
}

// tradeObject returns the first trade intent subject, falling back to the first
// consume item and then the first produced item.
func tradeObject(flow spec.FlowSpec) string {
	for _, intent := range flow.Intents {
		if intent.TradeSide != "" && intent.Object != "" {
			return intent.Object
		}
	}
	if len(flow.Consume) > 0 {
		return flow.Consume[0]
	}
	if len(flow.Produce) > 0 {
		return flow.Produce[0].Name
	}
	return ""
}

// planInteraction writes the interaction request rows for a consume flow: the
// first derived interaction verb (or 使用) and the first consumed item. It never
// touches the material entity; the interaction tick consumes it.
func planInteraction(flow spec.FlowSpec, entity string) ([]Change, error) {
	if len(flow.Consume) == 0 {
		return nil, nil
	}
	if flow.Consume[0] == "" {
		return nil, fmt.Errorf("flow %q: first consume item is empty", flow.ID)
	}
	action := interactionAction(flow)
	for _, row := range []struct{ path, value string }{
		{PATH_INTERACT_ACTION, action},
		{PATH_INTERACT_TARGET, flow.Consume[0]},
	} {
		if err := validateTargetPath(row.path); err != nil {
			return nil, fmt.Errorf("flow %q: %w", flow.ID, err)
		}
	}
	return []Change{
		{Op: OP_INTENT, Entity: entity, Path: PATH_INTERACT_ACTION, Kind: spec.EFFECT_KIND_TEXT, Value: action},
		{Op: OP_INTENT, Entity: entity, Path: PATH_INTERACT_TARGET, Kind: spec.EFFECT_KIND_TEXT, Value: flow.Consume[0]},
	}, nil
}

// interactionAction returns the first non-trade intent verb, defaulting to 使用.
func interactionAction(flow spec.FlowSpec) string {
	for _, intent := range flow.Intents {
		if intent.TradeSide == "" && intent.Action != "" {
			return intent.Action
		}
	}
	return DEFAULT_INTERACT_ACTION
}

// refusal wraps one refusal error into a K4 report.
func refusal(err error) Report {
	return Report{Changes: []Change{}, Errors: []error{err}}
}
