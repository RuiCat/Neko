// fixtures_test.go holds the shared doubles and builders used by the
// settlement tests: an atomic fake store, a done K3 state, an ok K2 verdict and
// one flow covering spend, consume, produce and all four effect kinds.
package settle

import (
	"sync"

	"github.com/RuiCat/Neko/internal/flow/condition"
	"github.com/RuiCat/Neko/internal/flow/spec"
	"github.com/RuiCat/Neko/internal/flow/step"
)

// testEntity is the actor every fixture settles.
const testEntity = "猫猫"

// fakeStore applies one change set and records it. A configured failure aborts
// before recording, which mirrors the atomic Store contract settlement relies
// on: a refused Apply leaves zero writes.
type fakeStore struct {
	mu         sync.Mutex
	applyCalls int
	rows       []Change
	fail       error
}

// Apply implements Store.
func (s *fakeStore) Apply(flowID string, changes []Change) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applyCalls++
	if s.fail != nil {
		return s.fail
	}
	s.rows = append(s.rows, changes...)
	return nil
}

// calls reports how often Apply was invoked.
func (s *fakeStore) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.applyCalls
}

// writes reports how many change rows were persisted.
func (s *fakeStore) writes() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.rows)
}

// applied returns a copy of the persisted rows.
func (s *fakeStore) applied() []Change {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Change(nil), s.rows...)
}

// baseFlow is a finishing flow with a trade cost, one interaction target, one
// produced item and data-layer effects. It writes no computed field.
func baseFlow() spec.FlowSpec {
	return spec.FlowSpec{
		ID:       "做饭",
		Scene:    "S001-KT",
		Cost:     10,
		Requires: []string{"食材"},
		Consume:  []string{"食材"},
		Produce: []spec.ItemSpec{
			{Type: "素材", Name: "食物", Description: "热气腾腾的饭菜"},
		},
		Steps: []string{"洗菜切菜", "开火下锅", "装盘"},
		Effects: []spec.EffectSpec{
			mappedEffect("成长.经验", spec.EFFECT_KIND_ADD, 5),
			mappedEffect("关系.数值.好感度", spec.EFFECT_KIND_ADD, 2),
			mappedEffect("标记.饥饿", spec.EFFECT_KIND_BOOL, false),
		},
		Intents: []spec.IntentSpec{{Action: "使用", Target: "食材"}},
	}
}

// baseSnapshotData is the entity view for baseFlow. It holds two same-name
// items in different scenes so the C5 template-scene rule is observable.
func baseSnapshotData() map[string]any {
	return map[string]any{
		"位置.场景": "S001-KT",
		"素材.物品": []any{
			map[string]any{"名称": "食材", "所属场景": "S001-KT", "数量": 2},
			map[string]any{"名称": "食材", "所属场景": "S999", "数量": 5},
		},
	}
}

// okVerdict is a K2 acceptance.
func okVerdict() condition.Verdict {
	return condition.Verdict{OK: true, Reasons: []condition.Reason{}}
}

// doneState is a K3 done state for flow.
func doneState(flow spec.FlowSpec) step.State {
	return step.State{
		ExecutionID: "exec-1",
		FlowID:      flow.ID,
		StepIndex:   len(flow.Steps) - 1,
		Status:      step.StatusDone,
		StartedTick: 1,
	}
}

// testDeps wires the fake store and a fresh in-memory ledger around snap.
func testDeps(snap condition.Snapshot) (Deps, *fakeStore, *MemoryLedger) {
	store := &fakeStore{}
	ledger := NewMemoryLedger()
	return Deps{
		Snapshot: snap,
		Store:    store,
		Ledger:   ledger,
		Entity:   testEntity,
		Tick:     9,
	}, store, ledger
}

// sourceTarget returns one settlement target pointer for an EffectSpec.
func sourceTarget(path string) *string {
	return &path
}

// mappedEffect builds an effect whose D6 target is the same source path, which
// keeps the non-converting cases readable.
func mappedEffect(path string, kind spec.EffectKind, value any) spec.EffectSpec {
	return spec.EffectSpec{
		Path:         path,
		Kind:         kind,
		Value:        value,
		Target:       sourceTarget(path),
		TargetReason: "同名字段",
	}
}

// findChange returns the first change matching op and path.
func findChange(changes []Change, op ChangeOp, path string) (Change, bool) {
	for _, change := range changes {
		if change.Op == op && change.Path == path {
			return change, true
		}
	}
	return Change{}, false
}
