package compiler

import (
	"fmt"
	"strings"

	"github.com/ruicat/astrbox/internal/config"
	"github.com/ruicat/astrbox/internal/engine"
	"github.com/ruicat/astrbox/internal/neko"
)

// DeriveMatch derives the match selector, extra conditions and subject list.
func DeriveMatch(subjects []neko.Subject) (string, []string, bool, []string) {
	extraWhen := []string{}
	multi := false
	var subjectsList []string
	var globals []string
	for _, subject := range subjects {
		if subject.Kind == "global" {
			globals = append(globals, subject.EntityID+subject.Var)
		}
	}
	hasPrimary := false
	for _, subject := range subjects {
		if subject.Kind == "primary" || subject.Kind == "multi" {
			hasPrimary = true
		}
	}
	for _, subject := range subjects {
		if subject.Kind == "global" && !hasPrimary {
			entityID := subject.EntityID
			if entityID == "" {
				entityID = "[*]"
			}
			return entityID, extraWhen, multi, subjectsList
		}
		if subject.Kind == "multi" {
			multi = true
			sel := "[*]"
			if subject.TypeName != "" {
				sel = subject.TypeName + "[*]"
			}
			subjectsList = append(subjectsList, sel)
			if subject.Condition != "" {
				if subject.Var != "" {
					extraWhen = append(extraWhen, subject.Var+"."+subject.Condition)
				} else {
					extraWhen = append(extraWhen, sel)
				}
			}
		}
	}
	for _, subject := range subjects {
		if subject.Kind != "primary" {
			continue
		}
		if subject.TypeName != "" {
			match := subject.TypeName + "[*]"
			if subject.Condition != "" {
				extraWhen = append(extraWhen, "{e}."+subject.Condition)
			}
			return match, extraWhen, multi, subjectsList
		}
		return "[*]", extraWhen, multi, subjectsList
	}
	return "[*]", extraWhen, multi, subjectsList
}

// CompileOps converts parsed actions into engine operations.
func CompileOps(actions []neko.ActionOp) []engine.Op {
	ops := make([]engine.Op, 0, len(actions))
	for _, action := range actions {
		if action.Op == "emit" {
			scene := action.Entity
			if scene == "" {
				scene = config.Current().DefaultScene
			}
			ops = append(ops, engine.Op{Op: "emit", Entity: scene, Value: action.Value})
			continue
		}
		op := engine.Op{Op: action.Op}
		switch {
		case action.Entity != "":
			op.Entity = action.Entity
			op.Path = action.Target
			if op.Path == "" {
				op.Path = config.Current().SceneEventField
			}
		case action.Subject != 0:
			op.Subject = action.Subject
			op.Path = action.Target
		default:
			if action.Target != "" {
				op.Path = "{e}." + action.Target
			} else {
				op.Path = "{e}"
			}
		}
		op.Value = MaybeRefValue(NormalizeTypeRefs(action.Value))
		ops = append(ops, op)
	}
	return ops
}

// NormalizeTypeRefs replaces $[TYPE]. references with {e}.
func NormalizeTypeRefs(value any) any {
	text, ok := value.(string)
	if !ok || !strings.Contains(text, "$[") {
		return value
	}
	text = typedRefPattern.ReplaceAllString(text, "{e}.")
	text = typedBarePattern.ReplaceAllString(text, "{e}")
	return text
}

// MaybeRefValue turns a bare dotted path into a $ reference.
func MaybeRefValue(value any) any {
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return value
	}
	trimmed := strings.TrimSpace(text)
	if strings.Contains(trimmed, "(") && strings.HasSuffix(trimmed, ")") {
		return value
	}
	if strings.HasPrefix(trimmed, "{e}") {
		return "$" + trimmed
	}
	if strings.HasPrefix(trimmed, "$") {
		return value
	}
	if pathRefPattern.MatchString(trimmed) {
		return "$" + trimmed
	}
	return value
}

// CompileBranches converts parsed branches into engine branches.
func CompileBranches(branches []neko.BranchDef, subjects []neko.Subject) []engine.Branch {
	result := make([]engine.Branch, 0, len(branches))
	for _, branch := range branches {
		out := engine.Branch{}
		if branch.Condition == nil {
			out.Else = true
		} else {
			rewritten, _ := RewriteCondition(*branch.Condition, subjects)
			out.If = &rewritten
		}
		out.Then = CompileOps(branch.Ops)
		if len(branch.State) > 0 {
			out.State = branch.State
		}
		result = append(result, out)
	}
	return result
}

// CompileRule compiles one rule definition.
func CompileRule(def neko.RuleDef) (*engine.Rule, error) {
	match, extraWhen, isMulti, subjectsList := DeriveMatch(def.Subjects)

	var rewrittenConds []string
	for _, cond := range def.Conditions {
		rewritten, extra := RewriteCondition(cond, def.Subjects)
		rewrittenConds = append(rewrittenConds, rewritten)
		extraWhen = append(extraWhen, extra...)
	}

	var whenParts []string
	if stateWhen := RewriteStateWhen(def.StateWhen); stateWhen != nil {
		whenParts = append(whenParts, *stateWhen)
	}
	whenParts = append(whenParts, rewrittenConds...)
	whenParts = append(whenParts, extraWhen...)
	var when *string
	if len(whenParts) > 0 {
		joined := strings.Join(whenParts, " ∧ ")
		when = &joined
	}

	actions := CompileOps(def.Actions)
	branches := CompileBranches(def.Branches, def.Subjects)
	if len(branches) == 0 {
		if len(actions) > 0 {
			branches = []engine.Branch{{Then: actions}}
			empty := ""
			branches[0].If = &empty
		} else {
			branches = []engine.Branch{{Else: true, ThenPresent: true}}
		}
	}
	if when == nil {
		hasCond := false
		for _, branch := range branches {
			if branch.If != nil || branch.Else {
				hasCond = true
			}
		}
		if !hasCond {
			return nil, fmt.Errorf("规则缺少条件")
		}
	}

	priority := 0
	if def.Priority != nil {
		priority = *def.Priority
	}
	desc := def.HeaderRaw
	if runes := []rune(desc); len(runes) > 80 {
		desc = string(runes[:80])
	}
	rule := &engine.Rule{
		ID: RuleID(def, match, subjectsList, when, branches), Match: match,
		When: when, Branches: branches, Priority: priority, Desc: desc,
		Source: "script", Enabled: true,
	}
	if isMulti && len(subjectsList) > 0 {
		rule.Subjects = subjectsList
	}
	if err := rule.Validate(); err != nil {
		return nil, err
	}
	return rule, nil
}

// RuleID returns the explicit id or a content-derived id. The hash input is
// normalized (not byte-identical to the Python repr), so regenerated ids stay
// stable for a given Go build.
func RuleID(def neko.RuleDef, match string, subjects []string, when *string, branches []engine.Branch) string {
	if def.RuleID != "" {
		return def.RuleID
	}
	return "r_" + shortHash(canonicalRule(match, subjects, when, branches))
}
