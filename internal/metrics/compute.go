// compute.go turns journal records into the metrics that can be judged from
// snapshots alone (A class) plus the citation traceability probe. Metrics that
// need a model's self-report or a rerun are reported as not_computable instead
// of a fake zero.
package metrics

import (
	"fmt"
	"sort"
	"time"

	"github.com/RuiCat/Neko/internal/annotations"
	"github.com/RuiCat/Neko/internal/hardstate"
	"github.com/RuiCat/Neko/internal/journal"
)

// Compute runs the offline metrics pass without annotations.
func Compute(records []journal.TurnRecord, journalDir string, now time.Time) Report {
	return ComputeWithAnnotations(records, nil, journalDir, now)
}

// MIN_KAPPA is the agreement floor from plan 5.3: below it the B-class metrics
// are observations only and must not act as a gate.
const MIN_KAPPA = 0.6

// ComputeWithAnnotations runs the pass with optional human labels for the
// B-class metrics. Labels only affect coverage/precision; every snapshot-based
// metric is computed the same way with or without them. When inter-annotator
// agreement is below MIN_KAPPA the B-class metrics are reported as
// not_computable instead of a number that could be used as a gate.
func ComputeWithAnnotations(records []journal.TurnRecord, labels map[string][]annotations.Record, journalDir string, now time.Time) Report {
	turns := make([]journal.TurnRecord, 0, len(records))
	for _, record := range records {
		if record.Source == journal.SOURCE_TICK {
			continue
		}
		turns = append(turns, record)
	}
	agreement := annotationAgreement(labels)
	coverage := compensationCoverage(turns, labels)
	precision := compensationPrecision(turns, labels)
	switch {
	case agreement.Status == STATUS_COMPUTED && agreement.Value < MIN_KAPPA:
		reason := fmt.Sprintf("标注一致性 κ=%.2f < %.2f，不作闸门", agreement.Value, MIN_KAPPA)
		coverage = blockedMetric(coverage, reason)
		precision = blockedMetric(precision, reason)
	case len(labels) > 0 && agreement.Status != STATUS_COMPUTED:
		reason := "缺少双标（无法计算 κ），不作闸门"
		coverage = blockedMetric(coverage, reason)
		precision = blockedMetric(precision, reason)
	}
	return Report{
		GeneratedAt: now.UTC(),
		JournalDir:  journalDir,
		Records:     len(records),
		Annotated:   len(labels),
		Metrics: []Metric{
			stateConsistency(turns),
			rebuildConsistency(turns),
			citationTraceability(turns),
			coverage,
			precision,
			agreement,
		},
	}
}

// blockedMetric demotes a computed B-class metric when agreement is too low.
// The value is zeroed so a consumer reading it cannot treat it as a gate.
func blockedMetric(metric Metric, reason string) Metric {
	if metric.Status != STATUS_COMPUTED {
		return metric
	}
	metric.Status = STATUS_NOT_COMPUTABLE
	metric.Value = 0
	metric.Reason = reason
	return metric
}

// stateConsistency compares every compensation state value with the snapshot
// of the same turn; it is the hard gate that must stay at 100%.
func stateConsistency(records []journal.TurnRecord) Metric {
	const name = "cross_turn_state_consistency"
	compared, matched := 0, 0
	for _, record := range records {
		for key, value := range record.Compensation.State {
			compared++
			if hardstate.SameValue(value, record.Snapshot.State[key]) {
				matched++
			}
		}
	}
	return ratioMetric(name, matched, compared, "compensation.state 为空")
}

// rebuildConsistency checks reconstructed memory fragments: each must name a
// snapshot that exists in the journal and carry content. Field-level matching
// needs a structured rebuild summary and arrives with the teacher signal.
func rebuildConsistency(records []journal.TurnRecord) Metric {
	const name = "rebuild_consistency"
	known := map[string]bool{}
	for _, record := range records {
		known[record.Snapshot.SnapshotID] = true
	}
	compared, matched := 0, 0
	for _, record := range records {
		for _, fragment := range record.Compensation.Memory {
			if fragment.Type != "reconstructed" {
				continue
			}
			compared++
			if fragment.SourceSnapshotID != "" && known[fragment.SourceSnapshotID] && fragment.Content != "" {
				matched++
			}
		}
	}
	return ratioMetric(name, matched, compared, "没有 reconstructed 记忆片段")
}

// citationTraceability checks that every cited past event resolves to a
// snapshot present in the journal. It is the machine-checkable half of the
// compensation coverage metric.
func citationTraceability(records []journal.TurnRecord) Metric {
	const name = "citation_traceability"
	known := map[string]bool{}
	for _, record := range records {
		known[record.Snapshot.SnapshotID] = true
	}
	compared, resolved := 0, 0
	for _, record := range records {
		for _, cite := range record.Cites {
			compared++
			if known[cite.EventID] {
				resolved++
			}
		}
	}
	return ratioMetric(name, resolved, compared, "没有回答引用")
}

// compensationCoverage is the B-class coverage metric: among annotated cites
// from the primary annotator, how many name a past event that the package
// provided AND that resolves to a journaled snapshot.
func compensationCoverage(records []journal.TurnRecord, labels map[string][]annotations.Record) Metric {
	const name = "compensation_coverage"
	known := snapshotIndex(records)
	byTurn := turnIndex(records)
	compared, covered := 0, 0
	for turnID, candidates := range labels {
		label, hasLabel := primaryLabel(candidates)
		if !hasLabel {
			continue
		}
		record, ok := byTurn[turnID]
		if !ok {
			continue
		}
		provided := map[string]bool{}
		for _, fragment := range record.Compensation.Memory {
			provided[fragment.SourceSnapshotID] = true
		}
		for _, cite := range label.Cites {
			compared++
			if known[cite.EventID] && provided[cite.EventID] {
				covered++
			}
		}
	}
	return ratioMetric(name, covered, compared, "标注里没有引用")
}

// compensationPrecision is the B-class precision metric: among fragments the
// primary annotator marked as used, how many carry a resolvable source and
// non-empty content. Semantic correctness remains a human judgement.
func compensationPrecision(records []journal.TurnRecord, labels map[string][]annotations.Record) Metric {
	const name = "compensation_precision"
	known := snapshotIndex(records)
	byTurn := turnIndex(records)
	compared, matched := 0, 0
	for turnID, candidates := range labels {
		label, hasLabel := primaryLabel(candidates)
		if !hasLabel {
			continue
		}
		record, ok := byTurn[turnID]
		if !ok {
			continue
		}
		for _, index := range label.UsedFragments {
			compared++
			if index < 0 || index >= len(record.Compensation.Memory) {
				continue
			}
			fragment := record.Compensation.Memory[index]
			if fragment.SourceSnapshotID != "" && known[fragment.SourceSnapshotID] && fragment.Content != "" {
				matched++
			}
		}
	}
	return ratioMetric(name, matched, compared, "标注里没有 used_fragments")
}

// annotationAgreement is Cohen's κ over paired `correct` labels. It is the
// gate plan 5.3 requires: κ below 0.6 must not be used as a gate. Numerator is
// the agreement count and denominator the paired count.
func annotationAgreement(labels map[string][]annotations.Record) Metric {
	const name = "annotation_agreement"
	var bothTrue, firstTrue, secondTrue, bothFalse int
	for _, candidates := range labels {
		if len(candidates) < 2 {
			continue
		}
		sorted := sortedLabels(candidates)
		first, second := sorted[0], sorted[1]
		if first.Correct == nil || second.Correct == nil {
			continue
		}
		switch {
		case *first.Correct && *second.Correct:
			bothTrue++
		case *first.Correct && !*second.Correct:
			firstTrue++
		case !*first.Correct && *second.Correct:
			secondTrue++
		default:
			bothFalse++
		}
	}
	pairs := bothTrue + firstTrue + secondTrue + bothFalse
	if pairs == 0 {
		return Metric{Name: name, Status: STATUS_NOT_COMPUTABLE, Reason: "没有双标的 correct 标签"}
	}
	po := float64(bothTrue+bothFalse) / float64(pairs)
	firstYes := float64(bothTrue+firstTrue) / float64(pairs)
	secondYes := float64(bothTrue+secondTrue) / float64(pairs)
	pe := firstYes*secondYes + (1-firstYes)*(1-secondYes)
	if pe >= 1 {
		return Metric{Name: name, Status: STATUS_NOT_COMPUTABLE, Reason: "κ 未定义（期望一致率=1）"}
	}
	return Metric{
		Name: name, Status: STATUS_COMPUTED,
		Value:     (po - pe) / (1 - pe),
		Numerator: bothTrue + bothFalse, Denominator: pairs,
	}
}

// primaryLabel picks the deterministic annotation used by the B-class
// metrics: the lexicographically first annotator for that turn.
func primaryLabel(candidates []annotations.Record) (annotations.Record, bool) {
	if len(candidates) == 0 {
		return annotations.Record{}, false
	}
	return sortedLabels(candidates)[0], true
}

func sortedLabels(candidates []annotations.Record) []annotations.Record {
	sorted := append([]annotations.Record(nil), candidates...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Annotator < sorted[j].Annotator })
	return sorted
}

// snapshotIndex collects every journaled snapshot ID.
func snapshotIndex(records []journal.TurnRecord) map[string]bool {
	known := map[string]bool{}
	for _, record := range records {
		known[record.Snapshot.SnapshotID] = true
	}
	return known
}

// turnIndex indexes records by turn ID.
func turnIndex(records []journal.TurnRecord) map[string]journal.TurnRecord {
	index := make(map[string]journal.TurnRecord, len(records))
	for _, record := range records {
		index[record.TurnID] = record
	}
	return index
}

// ratioMetric builds a computed metric, or a not_computable one when nothing
// could be compared.
func ratioMetric(name string, numerator, denominator int, emptyReason string) Metric {
	if denominator == 0 {
		return Metric{Name: name, Status: STATUS_NOT_COMPUTABLE, Reason: emptyReason}
	}
	return Metric{
		Name:        name,
		Status:      STATUS_COMPUTED,
		Value:       float64(numerator) / float64(denominator),
		Numerator:   numerator,
		Denominator: denominator,
	}
}
