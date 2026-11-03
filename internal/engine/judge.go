// judge.go is the read-only transition verdict (C3): a candidate state write
// is legal only when an enabled rule actually covers that path, illegal when
// the target does not exist, and uncovered otherwise. Judge never mutates
// state, so it cannot become a second writer.
package engine

import (
	"fmt"
	"strings"

	"github.com/RuiCat/Neko/internal/entity"
)

// Verdict statuses.
const (
	VERDICT_LEGAL     = "legal"
	VERDICT_ILLEGAL   = "illegal"
	VERDICT_UNCOVERED = "uncovered"
)

// Candidate is one proposed state write from a teacher signal or local expert.
type Candidate struct {
	EntityID string
	Path     string
	Proposed any
	Source   string
	Evidence []string
}

// Verdict is the read-only decision for one candidate.
type Verdict struct {
	Status   string `json:"status"`
	RuleID   string `json:"rule_id,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Snapshot string `json:"snapshot,omitempty"`
}

// Judge decides a candidate without writing anything. The returned Snapshot is
// left for the caller to fill from its own hard-state capture; Judge only
// classifies coverage so a candidate never bypasses the state machine.
func (m *StateMachine) Judge(candidate Candidate) (Verdict, error) {
	entityID := strings.TrimSpace(candidate.EntityID)
	path := normalizeCandidatePath(entityID, candidate.Path)
	if entityID == "" {
		return Verdict{Status: VERDICT_ILLEGAL, Reason: "候选缺少 entity_id"}, nil
	}
	if path == "" {
		return Verdict{Status: VERDICT_ILLEGAL, Reason: "候选缺少写入路径"}, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	target := m.store.Get(entityID)
	if target == nil {
		return Verdict{Status: VERDICT_ILLEGAL, Reason: fmt.Sprintf("实体 %q 不存在", entityID)}, nil
	}
	for _, rule := range m.rules {
		if !rule.Enabled {
			continue
		}
		selector, err := entity.ParseEntityRef(rule.Match)
		if err != nil || !m.store.EntityMatches(selector, target) {
			continue
		}
		if coversPath(rule, entityID, path) {
			return Verdict{Status: VERDICT_LEGAL, RuleID: rule.ID}, nil
		}
	}
	return Verdict{Status: VERDICT_UNCOVERED, Reason: "没有启用规则覆盖该路径"}, nil
}

// coversPath reports whether any branch op of rule writes exactly path for the
// entity. Only state-writing ops count; emit/add/append are inspected for the
// same path because they can write too.
func coversPath(rule Rule, entityID, path string) bool {
	for _, branch := range rule.Branches {
		for _, op := range branch.Then {
			if op.Path == "" {
				continue
			}
			if normalizeCandidatePath(entityID, op.Path) == path {
				return true
			}
		}
	}
	return false
}

// normalizeCandidatePath strips an entity prefix and subject placeholders so a
// rule op and a candidate path compare in the same space.
func normalizeCandidatePath(entityID, raw string) string {
	path := strings.TrimSpace(raw)
	path = strings.ReplaceAll(path, "{e}", entityID)
	if entityID != "" {
		if path == entityID {
			return ""
		}
		if strings.HasPrefix(path, entityID+".") {
			path = path[len(entityID)+1:]
		}
	}
	if strings.HasPrefix(path, "$") {
		if index := strings.Index(path, "."); index >= 0 {
			path = path[index+1:]
		}
	}
	return path
}
