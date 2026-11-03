// Package triage is the P3 nightly scan: it turns one day of journal records
// into findings and classifies them into the fact / consistency / preference
// queues. Preference findings never reach the repair queue (constraint #9).
package triage

// Class is the routing bucket of one finding. Only fact and consistency are
// repairable; preference samples are evidence for trend metrics only.
type Class string

// The three triage queues.
const (
	// ClassFact marks a wrong answer or a citation that cannot be resolved.
	ClassFact Class = "fact"
	// ClassConsistency marks a structural drift between a record and its snapshot.
	ClassConsistency Class = "consistency"
	// ClassPreference marks a subjective sample that must not be auto-repaired.
	ClassPreference Class = "preference"
)

// Finding is one triage result. Every finding that can be repaired carries the
// snapshot ID it was observed against (plan: no snapshot, no repair).
type Finding struct {
	TurnID     string `json:"turn_id"`
	SnapshotID string `json:"snapshot_id,omitempty"`
	Class      Class  `json:"class"`
	Kind       string `json:"kind"`
	Severity   string `json:"severity"`
	Detail     string `json:"detail"`
	// Source is "A" for the full structural scan, "B" for the annotated sample.
	Source string `json:"source,omitempty"`
}
