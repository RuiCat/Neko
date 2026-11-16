// requirement.go models the precondition kinds the judge understands. Scene,
// item and money requirements derive from K1 FlowSpec; affinity and time are
// additive K2-side kinds because K1 carries no affinity or time field.
package condition

import "github.com/RuiCat/Neko/internal/flow/spec"

// Kind names one precondition category.
type Kind string

// Requirement kinds understood by JudgeRequirements.
const (
	// KIND_SCENE requires PATH_SCENE to equal Scene; empty Scene means no
	// constraint.
	KIND_SCENE Kind = "scene"
	// KIND_ITEM requires the PATH_ITEMS view to hold Quantity of Item in
	// Scene; Quantity <= 0 defaults to 1, empty Scene counts every scene.
	KIND_ITEM Kind = "item"
	// KIND_MONEY requires PATH_MONEY to be at least Threshold; non-positive
	// Threshold means no cost.
	KIND_MONEY Kind = "money"
	// KIND_AFFINITY requires PATH_AFFINITY (or Path when set) to be at least
	// Threshold; non-positive Threshold means no floor.
	KIND_AFFINITY Kind = "affinity"
	// KIND_TIME is a placeholder: no snapshot source exists yet, so it always
	// yields REASON_NOT_READY.
	KIND_TIME Kind = "time"
)

// Requirement is one precondition evaluated against a snapshot. Fields apply
// per kind: Scene for KIND_SCENE; Item/Scene/Quantity for KIND_ITEM; Threshold
// for KIND_MONEY, KIND_AFFINITY and KIND_TIME. Path overrides the snapshot path
// for KIND_AFFINITY and KIND_TIME.
type Requirement struct {
	Kind      Kind
	Scene     string
	Item      string
	Quantity  int
	Threshold int
	Path      string
}

// Requirements derives the K1 carve-out: an empty scene is unconstrained, each
// required item defaults to quantity 1 (attack log A4-3), and a non-positive
// cost is free.
func Requirements(flow spec.FlowSpec) []Requirement {
	reqs := make([]Requirement, 0, 2+len(flow.Requires))
	if flow.Scene != "" {
		reqs = append(reqs, Requirement{Kind: KIND_SCENE, Scene: flow.Scene})
	}
	for _, name := range flow.Requires {
		reqs = append(reqs, Requirement{
			Kind:     KIND_ITEM,
			Item:     name,
			Scene:    flow.Scene,
			Quantity: 1,
		})
	}
	if flow.Cost > 0 {
		reqs = append(reqs, Requirement{Kind: KIND_MONEY, Threshold: flow.Cost})
	}
	return reqs
}
