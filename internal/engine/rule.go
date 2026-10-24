// Package engine implements the AstrBox state machine, condition evaluator and
// effect applier.
package engine

import (
	"encoding/json"
	"fmt"
)

// Rule is one compiled state-machine rule.
type Rule struct {
	ID       string   `json:"id"`
	Desc     string   `json:"desc,omitempty"`
	Match    string   `json:"match"`
	When     *string  `json:"when,omitempty"`
	Priority int      `json:"priority,omitempty"`
	Source   string   `json:"source,omitempty"`
	Enabled  bool     `json:"enabled"`
	Subjects []string `json:"subjects,omitempty"`
	Branches []Branch `json:"branches"`
}

// Branch is one short-circuit branch of a rule.
type Branch struct {
	If    *string        `json:"if,omitempty"`
	Else  bool           `json:"else,omitempty"`
	Then  []Op           `json:"then,omitempty"`
	State map[string]any `json:"state,omitempty"`
	// ThenPresent keeps Python's explicit "then": [] on auto-generated else
	// branches, which participates in the rule-id hash.
	ThenPresent bool `json:"-"`
}

// Op is one compiled action.
type Op struct {
	Op      string `json:"op"`
	Path    string `json:"path,omitempty"`
	Value   any    `json:"value,omitempty"`
	Entity  string `json:"entity,omitempty"`
	Subject int    `json:"subject,omitempty"`
}

// UnmarshalJSON defaults enabled to true when the field is omitted, matching the
// Python rule loader (a missing flag must not silently disable a rule).
func (r *Rule) UnmarshalJSON(data []byte) error {
	type ruleAlias Rule
	aux := &struct {
		Enabled *bool `json:"enabled"`
		*ruleAlias
	}{ruleAlias: (*ruleAlias)(r)}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if aux.Enabled == nil {
		r.Enabled = true
	} else {
		r.Enabled = *aux.Enabled
	}
	return nil
}

// JSONMap renders a Rule as a generic map for persistence and tool output.
func (r Rule) JSONMap() map[string]any {
	return map[string]any{
		"id": r.ID, "desc": r.Desc, "match": r.Match, "when": r.When,
		"priority": r.Priority, "source": r.Source, "enabled": r.Enabled,
		"subjects": r.Subjects, "branches": r.Branches,
	}
}

// Validate reports structural problems with a rule.
func (r *Rule) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("规则 id 不能为空")
	}
	if r.Match == "" {
		return fmt.Errorf("规则 '%s' 缺少 match", r.ID)
	}
	if len(r.Branches) == 0 {
		return fmt.Errorf("规则 '%s' 缺少 branches", r.ID)
	}
	return nil
}

// ParseRules decodes a rules.json payload ({"version":1,"rules":[...]}) or a
// bare rule array.
func ParseRules(raw []byte) ([]Rule, error) {
	var wrapper struct {
		Rules []Rule `json:"rules"`
	}
	if err := json.Unmarshal(raw, &wrapper); err == nil && wrapper.Rules != nil {
		return normalizeRules(wrapper.Rules)
	}
	var list []Rule
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("规则 JSON 解析失败: %w", err)
	}
	return normalizeRules(list)
}

func normalizeRules(rules []Rule) ([]Rule, error) {
	for i := range rules {
		if rules[i].Source == "" {
			rules[i].Source = "user"
		}
		if err := rules[i].Validate(); err != nil {
			return nil, err
		}
	}
	return rules, nil
}

// RulesToJSON serializes rules in the canonical rules.json shape.
func RulesToJSON(rules []Rule) ([]byte, error) {
	payload := map[string]any{"version": 1, "rules": rules}
	return json.MarshalIndent(payload, "", "  ")
}
