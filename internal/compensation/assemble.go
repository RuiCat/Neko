// assemble.go builds the persona compensation package (C4): it activates
// experts, collects their fragments, projects the hard state from the snapshot
// and validates the result before anything is sent to a model.
package compensation

import (
	"context"
	"fmt"

	"github.com/RuiCat/Neko/internal/blackboard"
	"github.com/RuiCat/Neko/internal/expert"
	"github.com/RuiCat/Neko/internal/gate"
	"github.com/RuiCat/Neko/internal/journal"
)

// STATE_SOURCE is the only legal provenance for package state values.
const STATE_SOURCE = "state_machine_snapshot"

// MAX_MEMORY_FRAGMENTS bounds one package; the context budget must stay small.
const MAX_MEMORY_FRAGMENTS = 8

// Assembler turns a board plus gate output into a validated package.
type Assembler struct {
	Gate         gate.Gate
	Experts      *expert.Registry
	MaxFragments int
}

// NewAssembler wires a gate and an expert registry.
func NewAssembler(g gate.Gate, experts *expert.Registry) *Assembler {
	return &Assembler{Gate: g, Experts: experts}
}

// Input is everything one assembly needs beyond the board.
type Input struct {
	UserInput          string
	Board              blackboard.Reader
	Features           gate.Features
	PersonaConstraints []string
	Situation          string
}

// Assemble returns a validated package or an error; a package that fails
// validation is never returned (fail-closed).
func (a *Assembler) Assemble(ctx context.Context, in Input) (journal.Compensation, error) {
	pkg, _, err := a.AssembleWithActive(ctx, in)
	return pkg, err
}

// AssembleWithActive is Assemble plus the activated expert IDs, so the caller
// can record which experts ran without activating the gate twice.
func (a *Assembler) AssembleWithActive(ctx context.Context, in Input) (journal.Compensation, []string, error) {
	if a.Gate == nil || a.Experts == nil {
		return journal.Compensation{}, nil, fmt.Errorf("compensation: gate 或 experts 未配置")
	}
	if in.Board == nil {
		return journal.Compensation{}, nil, fmt.Errorf("compensation: 缺少黑板")
	}
	snapshot := in.Board.Snapshot()
	if snapshot.SnapshotID == "" {
		return journal.Compensation{}, nil, fmt.Errorf("compensation: 黑板没有硬状态快照")
	}
	active, err := a.Gate.Activate(in.Features)
	if err != nil {
		return journal.Compensation{}, nil, fmt.Errorf("compensation: 门控失败: %w", err)
	}
	fragments, err := a.Experts.Compensate(ctx, active, expert.Input{UserInput: in.UserInput, Board: in.Board})
	if err != nil {
		return journal.Compensation{}, nil, err
	}
	fragments = dedupeFragments(fragments)
	limit := a.MaxFragments
	if limit <= 0 {
		limit = MAX_MEMORY_FRAGMENTS
	}
	if len(fragments) > limit {
		fragments = fragments[:limit]
	}
	memory := make([]journal.MemoryFragment, 0, len(fragments))
	for _, fragment := range fragments {
		memory = append(memory, journal.MemoryFragment{
			Content:          fragment.Content,
			SourceSnapshotID: fragment.SourceSnapshotID,
			Type:             fragment.Type,
			Weight:           fragment.Weight,
		})
	}
	pkg := journal.Compensation{
		Memory:             memory,
		State:              copyState(snapshot.State),
		StateSource:        STATE_SOURCE,
		PersonaConstraints: append([]string(nil), in.PersonaConstraints...),
		Situation:          in.Situation,
		TurnID:             snapshot.TurnID,
		SnapshotID:         snapshot.SnapshotID,
	}
	if err := Validate(pkg, snapshot); err != nil {
		return journal.Compensation{}, nil, err
	}
	return pkg, active, nil
}

// copyState keeps package state a separate map so later board changes cannot
// mutate the package silently.
func copyState(state map[string]any) map[string]any {
	copied := make(map[string]any, len(state))
	for key, value := range state {
		copied[key] = value
	}
	return copied
}

// dedupeFragments keeps one fragment per (source, type, content) and keeps the
// highest weight. Experts have separate contexts, so the same turn can be
// recalled by several active experts; the package must not repeat it.
func dedupeFragments(fragments []expert.Fragment) []expert.Fragment {
	seen := make(map[string]int, len(fragments))
	unique := make([]expert.Fragment, 0, len(fragments))
	for _, fragment := range fragments {
		key := fragment.SourceSnapshotID + "\x00" + fragment.Type + "\x00" + fragment.Content
		if index, ok := seen[key]; ok {
			if fragment.Weight > unique[index].Weight {
				unique[index] = fragment
			}
			continue
		}
		seen[key] = len(unique)
		unique = append(unique, fragment)
	}
	return unique
}
