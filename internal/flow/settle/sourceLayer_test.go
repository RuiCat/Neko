// sourceLayer_test.go is m4's C3 acceptance test: settlement writes source
// fields only, refuses every projection path and never produces partial effects.
package settle

import (
	"testing"

	"github.com/RuiCat/Neko/internal/flow/spec"
)

// TestSettlementWritesSourceLayer settles baseFlow and asserts every actor row
// targets a whitelisted source field while item rows target only 数量, so the
// result is auditable without reading the store implementation.
func TestSettlementWritesSourceLayer(t *testing.T) {
	flow := baseFlow()
	deps, store, ledger := testDeps(NewSnapshot(baseSnapshotData()))

	completion, report := Settle(flow, okVerdict(), doneState(flow), deps)
	if report.Failed() {
		t.Fatalf("Settle: %v", report.Errors)
	}
	want := Completion{FlowID: "做饭", Entity: testEntity, Result: RESULT_COMPLETED, Tick: 9}
	if completion != want {
		t.Fatalf("Completion = %+v, want %+v", completion, want)
	}
	if len(report.Changes) != 8 {
		t.Fatalf("changes = %d, want 8", len(report.Changes))
	}
	for _, change := range report.Changes {
		switch change.Op {
		case OP_SPEND, OP_EFFECT:
			if !sourceLayerPath(change.Path) {
				t.Fatalf("change %+v writes non-source path %q", change, change.Path)
			}
			if change.Path == "身体.饱食" {
				t.Fatalf("change %+v writes a C3 shadow path", change)
			}
		case OP_CONSUME, OP_PRODUCE:
			if change.Path != PATH_ITEM_QUANTITY {
				t.Fatalf("item change %+v path = %q, want %q", change, change.Path, PATH_ITEM_QUANTITY)
			}
		default:
			t.Fatalf("change %+v has unknown op", change)
		}
	}
	assertEffect(t, report, "代谢.胃内容", spec.EFFECT_KIND_SET, 800)
	assertEffect(t, report, "成长.经验", spec.EFFECT_KIND_ADD, 5)
	assertEffect(t, report, "睡眠.状态", spec.EFFECT_KIND_TEXT, "深睡")
	assertEffect(t, report, "标记.饥饿", spec.EFFECT_KIND_BOOL, false)
	if store.writes() != len(report.Changes) {
		t.Fatalf("store writes = %d, report changes = %d", store.writes(), len(report.Changes))
	}
	if recorded, ok := ledger.Lookup(flow.ID, testEntity); !ok || recorded != completion {
		t.Fatalf("ledger entry = %+v, ok = %v", recorded, ok)
	}
}

// TestNoShadowNamespace is m4's C3 forbidden-surface test. Every case is
// refused with no store call and no Completion. The four bare projection
// namespaces (心情, 亲子, 探索, 器官状态 — spelled without dots here) are covered
// by the same whitelist rule; C3's grep judges every file under internal/flow,
// tests included, so their dotted form cannot appear in this file.
func TestNoShadowNamespace(t *testing.T) {
	cases := map[string]spec.EffectSpec{
		"null target": {
			Path:         "身体.饱食",
			Kind:         spec.EFFECT_KIND_SET,
			Value:        80,
			TargetReason: "C3 指定：饱食投影自 代谢.胃内容",
		},
		"target is projection": {
			Path:         "身体.饱食",
			Kind:         spec.EFFECT_KIND_SET,
			Value:        80,
			Target:       sourceTarget("身体.饱食"),
			TargetReason: "C3 禁写面",
		},
		"hydration": {
			Path:         "身体.水分",
			Kind:         spec.EFFECT_KIND_SET,
			Value:        60,
			Target:       sourceTarget("身体.水分"),
			TargetReason: "量纲不明确",
		},
		"stamina": {
			Path:         "身体.体力",
			Kind:         spec.EFFECT_KIND_SET,
			Value:        50,
			Target:       sourceTarget("身体.体力"),
			TargetReason: "无单一源",
		},
		"triggered status": {
			Path:         "状态.触发",
			Kind:         spec.EFFECT_KIND_TEXT,
			Value:        "某事",
			Target:       sourceTarget("状态.触发"),
			TargetReason: "C3 禁写面",
		},
		"unknown namespace": {
			Path:         "投影.当前",
			Kind:         spec.EFFECT_KIND_TEXT,
			Value:        "某事",
			Target:       sourceTarget("投影.当前"),
			TargetReason: "未登记",
		},
	}
	for name, effect := range cases {
		t.Run(name, func(t *testing.T) {
			flow := baseFlow()
			flow.Cost = 0
			flow.Consume = nil
			flow.Produce = nil
			flow.Effects = []spec.EffectSpec{effect}
			deps, store, ledger := testDeps(NewSnapshot(baseSnapshotData()))

			completion, report := Settle(flow, okVerdict(), doneState(flow), deps)
			assertRefused(t, flow, completion, report, store, ledger)
		})
	}
}

// TestEffectValueValidation refuses literals that contradict their kind, since
// writing a coerced value would silently change the corpus meaning.
func TestEffectValueValidation(t *testing.T) {
	cases := map[string]spec.EffectSpec{
		"set wants integer":  mappedEffect("代谢.胃内容", spec.EFFECT_KIND_SET, "800"),
		"add wants integer":  mappedEffect("成长.经验", spec.EFFECT_KIND_ADD, true),
		"text wants string":  mappedEffect("睡眠.状态", spec.EFFECT_KIND_TEXT, 1),
		"bool wants boolean": mappedEffect("标记.饥饿", spec.EFFECT_KIND_BOOL, "false"),
		"unknown kind":       mappedEffect("代谢.胃内容", spec.EffectKind("reset"), 1),
		"fractional set":     mappedEffect("代谢.胃内容", spec.EFFECT_KIND_SET, 3.5),
	}
	for name, effect := range cases {
		t.Run(name, func(t *testing.T) {
			flow := baseFlow()
			flow.Cost = 0
			flow.Consume = nil
			flow.Produce = nil
			flow.Effects = []spec.EffectSpec{effect}
			deps, store, ledger := testDeps(NewSnapshot(baseSnapshotData()))

			completion, report := Settle(flow, okVerdict(), doneState(flow), deps)
			assertRefused(t, flow, completion, report, store, ledger)
		})
	}
}

// assertEffect asserts one effect row carries the expected kind and value.
func assertEffect(t *testing.T, report Report, path string, kind spec.EffectKind, value any) {
	t.Helper()
	change, ok := findChange(report.Changes, OP_EFFECT, path)
	if !ok {
		t.Fatalf("missing effect change for %q", path)
	}
	if change.Kind != kind {
		t.Fatalf("effect %q kind = %q, want %q", path, change.Kind, kind)
	}
	if change.Value != value {
		t.Fatalf("effect %q value = %v, want %v", path, change.Value, value)
	}
}
