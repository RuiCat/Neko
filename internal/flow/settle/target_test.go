// target_test.go covers D6: settlement writes EffectSpec.Target only, a nil
// target refuses the flow with its audit reason, the legacy Path is never a
// write target, and the satiety projection converts into 代谢.胃内容 units.
package settle

import (
	"strings"
	"testing"

	"github.com/RuiCat/Neko/internal/flow/spec"
)

// TestNullTargetRefusesFlow proves an unmapped effect refuses the whole flow:
// the report names both the effect path and the recorded reason, and nothing is
// written. The writable-path case proves settlement never falls back to Path
// when Target is nil.
func TestNullTargetRefusesFlow(t *testing.T) {
	reason := "D6 未映射：拒绝猜测"
	cases := map[string]string{
		"legacy path":   PATH_SATIETY,
		"writable path": PATH_STOMACH_CONTENT,
	}
	for name, path := range cases {
		t.Run(name, func(t *testing.T) {
			flow := minimalEffectsFlow([]spec.EffectSpec{{
				Path:         path,
				Kind:         spec.EFFECT_KIND_SET,
				Value:        80,
				TargetReason: reason,
			}})
			deps, store, ledger := testDeps(NewSnapshot(nil))

			completion, report := Settle(flow, okVerdict(), doneState(flow), deps)
			assertRefused(t, flow, completion, report, store, ledger)
			message := report.Errors[0].Error()
			if !strings.Contains(message, path) {
				t.Fatalf("error %q does not name the effect path", message)
			}
			if !strings.Contains(message, reason) {
				t.Fatalf("error %q does not carry targetReason", message)
			}
		})
	}
}

// TestMappedTargetWritesOnlyTarget is the D6 happy path. The 吃饭 chain maps
// 身体.饱食 onto 代谢.胃内容 and keeps 标记.饥饿 on its same-name source field;
// the legacy path must never appear in the change set.
func TestMappedTargetWritesOnlyTarget(t *testing.T) {
	flow := minimalEffectsFlow([]spec.EffectSpec{
		{
			Path:         PATH_SATIETY,
			Kind:         spec.EFFECT_KIND_SET,
			Value:        80,
			Target:       sourceTarget(PATH_STOMACH_CONTENT),
			TargetReason: "C3 指定：饱食投影自 代谢.胃内容",
		},
		{
			Path:         "标记.饥饿",
			Kind:         spec.EFFECT_KIND_BOOL,
			Value:        false,
			Target:       sourceTarget("标记.饥饿"),
			TargetReason: "状态内核 标记.饥饿 同名字段",
		},
	})
	deps, store, _ := testDeps(NewSnapshot(nil))

	completion, report := Settle(flow, okVerdict(), doneState(flow), deps)
	if report.Failed() {
		t.Fatalf("Settle: %v", report.Errors)
	}
	if completion.FlowID != flow.ID || completion.Entity != testEntity {
		t.Fatalf("Completion = %+v", completion)
	}
	if len(report.Changes) != 2 {
		t.Fatalf("changes = %+v, want 2 effects", report.Changes)
	}
	stomach, ok := findChange(report.Changes, OP_EFFECT, PATH_STOMACH_CONTENT)
	if !ok {
		t.Fatalf("missing %q change: %+v", PATH_STOMACH_CONTENT, report.Changes)
	}
	if stomach.Kind != spec.EFFECT_KIND_SET || stomach.Value != 1200 {
		t.Fatalf("stomach change = %+v, want set 1200", stomach)
	}
	hunger, ok := findChange(report.Changes, OP_EFFECT, "标记.饥饿")
	if !ok || hunger.Value != false {
		t.Fatalf("hunger change = %+v, ok = %v", hunger, ok)
	}
	for _, change := range report.Changes {
		if change.Path == PATH_SATIETY {
			t.Fatalf("legacy path reached the write set: %+v", change)
		}
	}
	if store.writes() != len(report.Changes) {
		t.Fatalf("store writes = %d, report changes = %d", store.writes(), len(report.Changes))
	}
}

// TestSatietyConversion pins the D6 unit rule from m1's targetReason:
// satiety = stomach content / 1500 * 100, so stomach content = satiety * 15.
// set stays absolute and clamps to the kernel range; add stays an unclamped
// delta. Both directions are tested so a future change to the scale is caught.
func TestSatietyConversion(t *testing.T) {
	cases := []struct {
		name  string
		kind  spec.EffectKind
		value any
		want  int
	}{
		{name: "set 80 percent", kind: spec.EFFECT_KIND_SET, value: 80, want: 1200},
		{name: "set zero", kind: spec.EFFECT_KIND_SET, value: 0, want: 0},
		{name: "set full", kind: spec.EFFECT_KIND_SET, value: 100, want: 1500},
		{name: "set malformed legacy clamps", kind: spec.EFFECT_KIND_SET, value: 400, want: 5000},
		{name: "add ten points", kind: spec.EFFECT_KIND_ADD, value: 10, want: 150},
		{name: "add negative points", kind: spec.EFFECT_KIND_ADD, value: -5, want: -75},
		{name: "add is not clamped", kind: spec.EFFECT_KIND_ADD, value: 400, want: 6000},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			flow := minimalEffectsFlow([]spec.EffectSpec{{
				Path:         PATH_SATIETY,
				Kind:         testCase.kind,
				Value:        testCase.value,
				Target:       sourceTarget(PATH_STOMACH_CONTENT),
				TargetReason: "C3 指定：饱食投影自 代谢.胃内容",
			}})
			deps, _, _ := testDeps(NewSnapshot(nil))

			_, report := Settle(flow, okVerdict(), doneState(flow), deps)
			if report.Failed() {
				t.Fatalf("Settle: %v", report.Errors)
			}
			change, ok := findChange(report.Changes, OP_EFFECT, PATH_STOMACH_CONTENT)
			if !ok {
				t.Fatalf("missing %q change: %+v", PATH_STOMACH_CONTENT, report.Changes)
			}
			if change.Value != testCase.want {
				t.Fatalf("value = %v, want %d", change.Value, testCase.want)
			}
		})
	}
}

// TestTargetConversionIsPairScoped proves a same-name effect on an unrelated
// source field is not scaled by the satiety rule.
func TestTargetConversionIsPairScoped(t *testing.T) {
	flow := minimalEffectsFlow([]spec.EffectSpec{
		mappedEffect("成长.经验", spec.EFFECT_KIND_ADD, 5),
	})
	deps, _, _ := testDeps(NewSnapshot(nil))

	_, report := Settle(flow, okVerdict(), doneState(flow), deps)
	if report.Failed() {
		t.Fatalf("Settle: %v", report.Errors)
	}
	change, ok := findChange(report.Changes, OP_EFFECT, "成长.经验")
	if !ok || change.Value != 5 {
		t.Fatalf("experience change = %+v, ok = %v", change, ok)
	}
}

// minimalEffectsFlow returns baseFlow without cost, items or the fixture
// effects, so one test can drive an exact effect list.
func minimalEffectsFlow(effects []spec.EffectSpec) spec.FlowSpec {
	flow := baseFlow()
	flow.Cost = 0
	flow.Consume = nil
	flow.Produce = nil
	flow.Effects = effects
	return flow
}
