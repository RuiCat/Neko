// compute_test.go pins the A-class metric formulas on hand-built records.
package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/RuiCat/Neko/internal/annotations"
	"github.com/RuiCat/Neko/internal/hardstate"
	"github.com/RuiCat/Neko/internal/journal"
)

func recordWith(turnID string, state map[string]any, compensation map[string]any, cites []journal.Citation, memory []journal.MemoryFragment) journal.TurnRecord {
	return journal.TurnRecord{
		TurnID: turnID,
		Snapshot: hardstate.Snapshot{
			SnapshotID: turnID,
			State:      state,
		},
		Compensation: journal.Compensation{State: compensation, Memory: memory},
		Cites:        cites,
	}
}

func metricByName(t *testing.T, report Report, name string) Metric {
	t.Helper()
	for _, metric := range report.Metrics {
		if metric.Name == name {
			return metric
		}
	}
	t.Fatalf("缺少指标 %q", name)
	return Metric{}
}

func TestStateConsistencyCountsMismatches(t *testing.T) {
	records := []journal.TurnRecord{
		recordWith("evt_20261101_0001",
			map[string]any{"猫猫.状态.情绪": "平静", "猫猫.状态.安全": float64(3)},
			map[string]any{"猫猫.状态.情绪": "平静", "猫猫.状态.安全": float64(3)}, nil, nil),
		recordWith("evt_20261101_0002",
			map[string]any{"猫猫.状态.情绪": "平静"},
			map[string]any{"猫猫.状态.情绪": "烦躁"}, nil, nil),
	}
	metric := metricByName(t, Compute(records, "x", time.Unix(0, 0)), "cross_turn_state_consistency")
	if metric.Status != STATUS_COMPUTED {
		t.Fatalf("状态 %q：%s", metric.Status, metric.Reason)
	}
	if metric.Numerator != 2 || metric.Denominator != 3 {
		t.Fatalf("计数 %d/%d，期望 2/3", metric.Numerator, metric.Denominator)
	}
}

func TestCitationTraceability(t *testing.T) {
	records := []journal.TurnRecord{
		recordWith("evt_20261101_0001", nil, nil, []journal.Citation{
			{EventID: "evt_20261031_0004", Kind: "event"},
			{EventID: "evt_missing", Kind: "event"},
		}, nil),
		recordWith("evt_20261031_0004", nil, nil, nil, nil),
	}
	metric := metricByName(t, Compute(records, "x", time.Unix(0, 0)), "citation_traceability")
	if metric.Numerator != 1 || metric.Denominator != 2 {
		t.Fatalf("计数 %d/%d，期望 1/2", metric.Numerator, metric.Denominator)
	}
}

func TestRebuildAndEmptySetsAreNotComputable(t *testing.T) {
	report := Compute([]journal.TurnRecord{
		recordWith("evt_20261101_0001", nil, nil, nil, nil),
	}, "x", time.Unix(0, 0))
	for _, name := range []string{"cross_turn_state_consistency", "rebuild_consistency", "citation_traceability"} {
		metric := metricByName(t, report, name)
		if metric.Status != STATUS_NOT_COMPUTABLE {
			t.Fatalf("%s 应为 not_computable，得到 %q", name, metric.Status)
		}
	}
	if report.Records != 1 {
		t.Fatalf("记录数 %d", report.Records)
	}
}

func TestRebuildConsistencyResolvesSources(t *testing.T) {
	records := []journal.TurnRecord{
		recordWith("evt_20261101_0001", nil, nil, nil, nil),
		recordWith("evt_20261101_0002", nil, nil, nil, []journal.MemoryFragment{
			{Content: "昨晚的约定", SourceSnapshotID: "evt_20261101_0001", Type: "reconstructed", Weight: 0.4},
			{Content: "编造的来源", SourceSnapshotID: "evt_missing", Type: "reconstructed", Weight: 0.4},
		}),
	}
	metric := metricByName(t, Compute(records, "x", time.Unix(0, 0)), "rebuild_consistency")
	if metric.Numerator != 1 || metric.Denominator != 2 {
		t.Fatalf("计数 %d/%d，期望 1/2", metric.Numerator, metric.Denominator)
	}
}

func TestTickRecordsExcludedFromMetrics(t *testing.T) {
	tick := recordWith("evt_tick", map[string]any{"猫猫.状态.情绪": "平静"},
		map[string]any{"猫猫.状态.情绪": "烦躁"}, nil, nil)
	tick.Source = journal.SOURCE_TICK
	turn := recordWith("evt_turn", map[string]any{"猫猫.状态.情绪": "平静"},
		map[string]any{"猫猫.状态.情绪": "平静"}, nil, nil)
	turn.Source = journal.SOURCE_CHAT
	report := Compute([]journal.TurnRecord{tick, turn}, "x", time.Unix(0, 0))
	metric := metricByName(t, report, "cross_turn_state_consistency")
	if metric.Numerator != 1 || metric.Denominator != 1 {
		t.Fatalf("tick 记录不应计入指标：%d/%d", metric.Numerator, metric.Denominator)
	}
	if report.Records != 2 {
		t.Fatalf("记录总数 %d", report.Records)
	}
}

func annotationFixture() ([]journal.TurnRecord, map[string][]annotations.Record) {
	previous := recordWith("evt_0", map[string]any{"猫猫.状态.情绪": "平静"}, nil, nil, nil)
	previous.Source = journal.SOURCE_CHAT
	turn := recordWith("evt_1", map[string]any{"猫猫.状态.情绪": "平静"},
		map[string]any{"猫猫.状态.情绪": "平静"}, nil,
		[]journal.MemoryFragment{{Content: "昨晚的约定", SourceSnapshotID: "evt_0", Type: "retrieved", Weight: 0.8}})
	turn.Source = journal.SOURCE_CHAT
	labels := map[string][]annotations.Record{
		"evt_1": {{
			TurnID: "evt_1", UsedFragments: []int{0},
			Cites:     []annotations.Citation{{EventID: "evt_0", Kind: "event"}, {EventID: "evt_missing", Kind: "event"}},
			Annotator: "lead", CreatedAt: "2026-11-03",
		}},
	}
	return []journal.TurnRecord{previous, turn}, labels
}

func TestSingleAnnotatorBlocksBClass(t *testing.T) {
	records, labels := annotationFixture()
	report := ComputeWithAnnotations(records, labels, "x", time.Unix(0, 0))
	for _, name := range []string{"compensation_coverage", "compensation_precision"} {
		metric := metricByName(t, report, name)
		if metric.Status != STATUS_NOT_COMPUTABLE {
			t.Fatalf("单标注不应作为闸门：%s %+v", name, metric)
		}
		if !strings.Contains(metric.Reason, "双标") {
			t.Fatalf("降级原因未说明缺双标：%q", metric.Reason)
		}
	}
	if report.Annotated != 1 {
		t.Fatalf("标注数 %d", report.Annotated)
	}
}

func TestWithoutAnnotationsBClassIsNotComputable(t *testing.T) {
	report := Compute([]journal.TurnRecord{recordWith("evt_1", nil, nil, nil, nil)}, "x", time.Unix(0, 0))
	for _, name := range []string{"compensation_coverage", "compensation_precision", "annotation_agreement"} {
		if metric := metricByName(t, report, name); metric.Status != STATUS_NOT_COMPUTABLE {
			t.Fatalf("%s 应为 not_computable：%+v", name, metric)
		}
	}
}

func boolPointer(value bool) *bool { return &value }

func TestAnnotationAgreementKappa(t *testing.T) {
	labels := map[string][]annotations.Record{
		"t1": {{TurnID: "t1", Annotator: "a", CreatedAt: "2026-11-03", Correct: boolPointer(true)},
			{TurnID: "t1", Annotator: "b", CreatedAt: "2026-11-03", Correct: boolPointer(true)}},
		"t2": {{TurnID: "t2", Annotator: "a", CreatedAt: "2026-11-03", Correct: boolPointer(true)},
			{TurnID: "t2", Annotator: "b", CreatedAt: "2026-11-03", Correct: boolPointer(true)}},
		"t3": {{TurnID: "t3", Annotator: "a", CreatedAt: "2026-11-03", Correct: boolPointer(true)},
			{TurnID: "t3", Annotator: "b", CreatedAt: "2026-11-03", Correct: boolPointer(false)}},
		"t4": {{TurnID: "t4", Annotator: "a", CreatedAt: "2026-11-03", Correct: boolPointer(false)},
			{TurnID: "t4", Annotator: "b", CreatedAt: "2026-11-03", Correct: boolPointer(false)}},
	}
	report := ComputeWithAnnotations(nil, labels, "x", time.Unix(0, 0))
	metric := metricByName(t, report, "annotation_agreement")
	if metric.Status != STATUS_COMPUTED {
		t.Fatalf("κ 应可计算：%+v", metric)
	}
	if metric.Value != 0.5 {
		t.Fatalf("κ = %v，期望 0.5", metric.Value)
	}
	if metric.Numerator != 3 || metric.Denominator != 4 {
		t.Fatalf("一致计数 %d/%d，期望 3/4", metric.Numerator, metric.Denominator)
	}
}

func labelPair(turnID, annotator string, correct bool) annotations.Record {
	return annotations.Record{
		TurnID: turnID, Annotator: annotator, CreatedAt: "2026-11-03", Correct: boolPointer(correct),
	}
}

func TestLowKappaBlocksBClass(t *testing.T) {
	records, labels := annotationFixture()
	labels["evt_1"] = []annotations.Record{
		{TurnID: "evt_1", Annotator: "a", CreatedAt: "2026-11-03", Correct: boolPointer(true),
			UsedFragments: []int{0}, Cites: []annotations.Citation{{EventID: "evt_0", Kind: "event"}}},
		{TurnID: "evt_1", Annotator: "b", CreatedAt: "2026-11-03", Correct: boolPointer(false),
			UsedFragments: []int{0}, Cites: []annotations.Citation{{EventID: "evt_0", Kind: "event"}}},
	}
	report := ComputeWithAnnotations(records, labels, "x", time.Unix(0, 0))
	agreement := metricByName(t, report, "annotation_agreement")
	if agreement.Status != STATUS_COMPUTED || agreement.Value >= MIN_KAPPA {
		t.Fatalf("夹具 κ 应低于阈值：%+v", agreement)
	}
	for _, name := range []string{"compensation_coverage", "compensation_precision"} {
		metric := metricByName(t, report, name)
		if metric.Status != STATUS_NOT_COMPUTABLE {
			t.Fatalf("κ 不足时 %s 不应作闸门：%+v", name, metric)
		}
		if !strings.Contains(metric.Reason, "κ") {
			t.Fatalf("降级原因未说明 κ：%q", metric.Reason)
		}
	}
}

func TestHighKappaKeepsBClass(t *testing.T) {
	records, labels := annotationFixture()
	labels["evt_1"] = []annotations.Record{
		{TurnID: "evt_1", Annotator: "a", CreatedAt: "2026-11-03", Correct: boolPointer(true),
			UsedFragments: []int{0}, Cites: []annotations.Citation{{EventID: "evt_0", Kind: "event"}}},
		{TurnID: "evt_1", Annotator: "b", CreatedAt: "2026-11-03", Correct: boolPointer(true),
			UsedFragments: []int{0}, Cites: []annotations.Citation{{EventID: "evt_0", Kind: "event"}}},
	}
	for index := 0; index < 4; index++ {
		turnID := "same" + string(rune('a'+index))
		labels[turnID] = []annotations.Record{labelPair(turnID, "a", true), labelPair(turnID, "b", true)}
	}
	for index := 0; index < 4; index++ {
		turnID := "no" + string(rune('a'+index))
		labels[turnID] = []annotations.Record{labelPair(turnID, "a", false), labelPair(turnID, "b", false)}
	}
	labels["split"] = []annotations.Record{labelPair("split", "a", true), labelPair("split", "b", false)}
	report := ComputeWithAnnotations(records, labels, "x", time.Unix(0, 0))
	agreement := metricByName(t, report, "annotation_agreement")
	if agreement.Status != STATUS_COMPUTED || agreement.Value < MIN_KAPPA {
		t.Fatalf("夹具 κ 应高于阈值：%+v", agreement)
	}
	coverage := metricByName(t, report, "compensation_coverage")
	if coverage.Status != STATUS_COMPUTED || coverage.Numerator != 1 || coverage.Denominator != 1 {
		t.Fatalf("高 κ 下覆盖率应可算：%+v", coverage)
	}
}
