// State machine tick execution: rule selection, branch short-circuit, ops,
// transitions and conflict collection.
//
// Ported from legacy/python-astrbox/state_machine/engine.py. Tick mutates
// in-memory state only and never persists the store; callers save explicitly.
// Function calls are delegated to an optional FuncCaller (see funcCall.go).
// LLM decision logging and rule back-fill are still absent.
package engine

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ruicat/astrbox/internal/config"
	"github.com/ruicat/astrbox/internal/entity"
)

// Tick budgets, mirroring Python's MAX_RULE_EVAL / MAX_TRANSITIONS.
const (
	MAX_RULE_EVAL   = 3000
	MAX_TRANSITIONS = 2000
)

// Transition records one rule firing for one entity.
type Transition struct {
	EntityID   string           `json:"entity_id"`
	RuleID     string           `json:"rule_id"`
	FromState  map[string]any   `json:"from_state"`
	ToState    map[string]any   `json:"to_state"`
	Operations []map[string]any `json:"operations"`
}

// Conflict records a state that needs arbitration.
type Conflict struct {
	ConflictID    string         `json:"conflict_id"`
	EntityID      string         `json:"entity_id"`
	RuleID        string         `json:"rule_id"`
	Type          string         `json:"type"`
	Context       map[string]any `json:"context"`
	Candidates    []any          `json:"candidates"`
	Reason        string         `json:"reason,omitempty"`
	Resolved      bool           `json:"resolved"`
	ResolvedState map[string]any `json:"resolved_state,omitempty"`
}

// TickResult is one tick's outcome. JSON tags use snake_case to match Python.
type TickResult struct {
	TickID         string       `json:"tick_id"`
	Status         string       `json:"status"`
	Transitions    []Transition `json:"transitions"`
	Conflicts      []Conflict   `json:"conflicts"`
	ReplyHint      string       `json:"hint"`
	RulesEvaluated int          `json:"rules_evaluated"`
	Errors         []string     `json:"errors"`
}

// StateMachine owns a rule set and executes ticks against one entity store.
type StateMachine struct {
	mu               sync.Mutex
	store            *entity.Store
	rules            []Rule
	pendingConflicts map[string]Conflict
	decisionPath     string
	tickResult       *TickResult
	tickTracker      map[string]map[string]targetWrite
	currentRule      string
	funcCaller       FuncCaller
	callMu           sync.RWMutex
}

// New creates a state machine bound to a store.
func New(store *entity.Store) *StateMachine {
	return &StateMachine{store: store, pendingConflicts: map[string]Conflict{}}
}

// AddRule registers a rule. Same-id rules are replaced; fully identical enabled
// rules are merged the way Python does.
func (m *StateMachine) AddRule(rule Rule) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.addRuleLocked(rule)
}

func (m *StateMachine) addRuleLocked(rule Rule) {
	kept := m.rules[:0]
	for _, existing := range m.rules {
		if existing.ID == rule.ID {
			continue
		}
		if existing.Enabled && ruleSignature(existing) == ruleSignature(rule) {
			continue
		}
		kept = append(kept, existing)
	}
	m.rules = append(kept, rule)
}

// AddRules registers rules in order.
func (m *StateMachine) AddRules(rules []Rule) {
	for _, rule := range rules {
		m.AddRule(rule)
	}
}

// RemoveRule deletes a rule by id and reports whether it existed.
func (m *StateMachine) RemoveRule(ruleID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	before := len(m.rules)
	kept := make([]Rule, 0, len(m.rules))
	for _, rule := range m.rules {
		if rule.ID != ruleID {
			kept = append(kept, rule)
		}
	}
	m.rules = kept
	return len(m.rules) < before
}

// GetRule returns a copy of a rule, or nil.
func (m *StateMachine) GetRule(ruleID string) *Rule {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, rule := range m.rules {
		if rule.ID == ruleID {
			copied := rule
			return &copied
		}
	}
	return nil
}

// Rules returns a copy of the registered rule list.
func (m *StateMachine) Rules() []Rule {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Rule{}, m.rules...)
}

// PendingConflicts returns unresolved conflicts collected by previous ticks.
func (m *StateMachine) PendingConflicts() []Conflict {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]string, 0, len(m.pendingConflicts))
	for key := range m.pendingConflicts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]Conflict, 0, len(keys))
	for _, key := range keys {
		out = append(out, m.pendingConflicts[key])
	}
	return out
}

func ruleSignature(rule Rule) string {
	when := ""
	if rule.When != nil {
		when = *rule.When
	}
	thenParts := make([]string, 0, len(rule.Branches))
	for _, branch := range rule.Branches {
		encoded, _ := json.Marshal(branch.Then)
		thenParts = append(thenParts, string(encoded))
	}
	return fmt.Sprintf("%s|%d|%s|%s", rule.Match, rule.Priority, when, strings.Join(thenParts, ","))
}

// Tick executes one tick. scope selects the triggering entities; an empty or
// unparsable scope means all entities. State changes stay in memory: the caller
// persists via Store.Save() when desired.
func (m *StateMachine) Tick(scope string) TickResult {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := TickResult{
		TickID:      newTickID(),
		Status:      "ok",
		Transitions: []Transition{},
		Conflicts:   []Conflict{},
		Errors:      []string{},
	}
	var scopeSelector *entity.Selector
	if trimmed := strings.TrimSpace(scope); trimmed != "" {
		if selector, err := entity.ParseEntityRef(trimmed); err == nil {
			scopeSelector = &selector
		}
	}

	enabled := make([]Rule, 0, len(m.rules))
	for _, rule := range m.rules {
		if rule.Enabled {
			enabled = append(enabled, rule)
		}
	}
	sort.SliceStable(enabled, func(i, j int) bool { return enabled[i].Priority < enabled[j].Priority })

	applied := map[string]bool{}
	tracker := map[string]map[string]targetWrite{}
	m.tickResult = &result
	m.tickTracker = tracker
	defer func() {
		m.tickResult = nil
		m.tickTracker = nil
		m.currentRule = ""
	}()
	removeListeners := m.attachWriteListeners()
	defer func() {
		for _, remove := range removeListeners {
			remove()
		}
	}()
	evalCount := 0
	transitionCount := 0

	for _, rule := range enabled {
		if evalCount > MAX_RULE_EVAL {
			break
		}
		if len(rule.Subjects) > 0 {
			if m.runSubjectRule(rule, scopeSelector, applied, tracker, &result, &evalCount, &transitionCount) {
				continue
			}
			continue
		}
		selector, err := entity.ParseEntityRef(rule.Match)
		if err != nil {
			message := fmt.Sprintf("规则 '%s' match 解析失败: %v", rule.ID, err)
			result.Errors = append(result.Errors, message)
			result.Status = "error"
			continue
		}
		entities := m.store.QuerySelector(selector)
		if scopeSelector != nil {
			entities = filterEntities(m.store, *scopeSelector, entities)
		}
		if len(entities) == 0 {
			continue
		}
		evalCount++
		if evalCount > MAX_RULE_EVAL {
			break
		}
		ruleTransitions := 0
		for _, ent := range entities {
			resolver := m.newEntityResolver(ent)
			if m.runRule(ent, rule, resolver, applied, &result, nil, false, tracker) {
				transitionCount++
				ruleTransitions++
			}
			if ruleTransitions >= MAX_TRANSITIONS {
				break
			}
		}
	}

	result.RulesEvaluated = evalCount
	switch {
	case len(result.Conflicts) > 0:
		result.Status = "conflict"
		result.ReplyHint = fmt.Sprintf("发现 %d 个状态冲突，需要 LLM 仲裁", len(result.Conflicts))
	case len(result.Transitions) > 0:
		result.ReplyHint = fmt.Sprintf("执行了 %d 个状态转移", len(result.Transitions))
	default:
		result.ReplyHint = "无状态变化"
	}
	if len(result.Errors) > 0 {
		shown := result.Errors
		if len(shown) > 3 {
			shown = shown[:3]
		}
		suffix := fmt.Sprintf("⚠️ %d 条规则解析失败: %s", len(result.Errors), strings.Join(shown, "; "))
		if result.ReplyHint == "无状态变化" {
			result.ReplyHint = suffix
		} else {
			result.ReplyHint += " | " + suffix
		}
	}
	return result
}

// runSubjectRule executes a multi-subject rule across the cartesian product of
// its selectors. It returns false when the rule cannot be evaluated.
func (m *StateMachine) runSubjectRule(rule Rule, scopeSelector *entity.Selector, applied map[string]bool,
	tracker map[string]map[string]targetWrite, result *TickResult, evalCount *int, transitionCount *int) bool {
	lists := make([][]*entity.Entity, 0, len(rule.Subjects))
	for index, subject := range rule.Subjects {
		selector, err := entity.ParseEntityRef(subject)
		if err != nil {
			message := fmt.Sprintf("规则 '%s' subjects 解析失败: %v", rule.ID, err)
			result.Errors = append(result.Errors, message)
			result.Status = "error"
			return false
		}
		entities := m.store.QuerySelector(selector)
		if scopeSelector != nil && index == 0 {
			entities = filterEntities(m.store, *scopeSelector, entities)
		}
		if len(entities) == 0 {
			return false
		}
		lists = append(lists, entities)
	}
	if len(lists) == 0 {
		return false
	}
	(*evalCount)++
	if *evalCount > MAX_RULE_EVAL {
		return false
	}
	ruleTransitions := 0
	var walk func(index int, chosen []*entity.Entity)
	walk = func(index int, chosen []*entity.Entity) {
		if ruleTransitions >= MAX_TRANSITIONS {
			return
		}
		if index == len(lists) {
			seen := map[*entity.Entity]bool{}
			for _, ent := range chosen {
				if seen[ent] {
					return
				}
				seen[ent] = true
			}
			bindings := map[int]*entity.Entity{}
			for i, ent := range chosen {
				bindings[i+1] = ent
			}
			resolver := m.newMultiResolver(bindings)
			if m.runRule(chosen[0], rule, resolver, applied, result, bindings, true, tracker) {
				(*transitionCount)++
				ruleTransitions++
			}
			return
		}
		for _, ent := range lists[index] {
			walk(index+1, append(chosen, ent))
		}
	}
	walk(0, nil)
	return true
}

func filterEntities(store *entity.Store, selector entity.Selector, entities []*entity.Entity) []*entity.Entity {
	out := make([]*entity.Entity, 0, len(entities))
	for _, ent := range entities {
		if store.EntityMatches(selector, ent) {
			out = append(out, ent)
		}
	}
	return out
}

// runRule applies one rule to one entity. It returns whether a transition was
// recorded.
func (m *StateMachine) runRule(ent *entity.Entity, rule Rule, resolver ValueResolver,
	applied map[string]bool, result *TickResult, bindings map[int]*entity.Entity,
	allowNoMatch bool, tracker map[string]map[string]targetWrite) bool {
	if rule.When != nil && strings.TrimSpace(*rule.When) != "" {
		condition, err := CompileCondition(*rule.When)
		if err != nil || !condition(resolver) {
			return false
		}
	}

	var matched *Branch
	for index := range rule.Branches {
		branch := &rule.Branches[index]
		if branch.Else {
			matched = branch
			break
		}
		if branch.If != nil && strings.TrimSpace(*branch.If) != "" {
			condition, err := CompileCondition(*branch.If)
			if err != nil {
				continue
			}
			if condition(resolver) {
				matched = branch
				break
			}
			continue
		}
		matched = branch
		break
	}

	if matched == nil {
		if allowNoMatch {
			return false
		}
		conflict := m.makeConflict(ent, rule, "no_branch_match",
			"规则 '"+rule.ID+"' 无匹配分支（无 else 兜底）")
		result.Conflicts = append(result.Conflicts, conflict)
		m.pendingConflicts[conflict.ConflictID] = conflict
		result.Status = "conflict"
		return false
	}

	fromState := stateSnapshot(ent)
	m.currentRule = rule.ID
	defer func() { m.currentRule = "" }()
	var operations []map[string]any
	for _, op := range matched.Then {
		operation := m.applyOp(ent, op, resolver, bindings)
		if operation == nil {
			continue
		}
		key := operationKey(ent.ID, operation)
		if applied[key] {
			continue
		}
		applied[key] = true
		operations = append(operations, operation)
	}
	if matched.State != nil {
		groups := make([]string, 0, len(matched.State))
		for group := range matched.State {
			groups = append(groups, group)
		}
		sort.Strings(groups)
		for _, group := range groups {
			value := matched.State[group]
			path := config.Current().StatePath(group)
			if err := ent.Set(path, value); err != nil {
				continue
			}
			operation := map[string]any{"op": "set", "path": path, "value": value}
			key := operationKey(ent.ID, operation)
			if applied[key] {
				continue
			}
			applied[key] = true
			operations = append(operations, operation)
		}
	}
	toState := stateSnapshot(ent)
	if len(operations) == 0 && equalValues(fromState, toState) {
		return false
	}
	result.Transitions = append(result.Transitions, Transition{
		EntityID:   ent.ID,
		RuleID:     rule.ID,
		FromState:  fromState,
		ToState:    toState,
		Operations: operations,
	})
	return true
}

// ResolveConflict applies an arbitrator decision to a pending conflict.
// ResolveConflict applies an arbitration decision without side effects.
func (m *StateMachine) ResolveConflict(conflictID string, targetState map[string]any, reason string) bool {
	return m.ResolveConflictWithEffects(conflictID, targetState, reason, nil)
}

// ResolveConflictWithEffects applies a decision, logs it and (when effects are
// provided) back-fills a learned rule.
func (m *StateMachine) ResolveConflictWithEffects(conflictID string, targetState map[string]any, reason string, effects []Op) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	conflict, ok := m.pendingConflicts[conflictID]
	if !ok {
		return false
	}
	ent := m.store.Get(conflict.EntityID)
	if ent == nil {
		return false
	}
	groups := make([]string, 0, len(targetState))
	for group := range targetState {
		groups = append(groups, group)
	}
	sort.Strings(groups)
	for _, group := range groups {
		if err := ent.Set(config.Current().StatePath(group), targetState[group]); err != nil {
			continue
		}
	}
	conflict.Resolved = true
	if targetState != nil {
		conflict.ResolvedState = deepCopyAny(targetState).(map[string]any)
	}
	conflict.Reason = reason
	if m.decisionPath != "" {
		_ = m.logDecision(conflict, targetState, reason)
	}
	if len(effects) > 0 {
		m.backfillRuleLocked(conflict, targetState, effects)
	}
	m.pendingConflicts[conflictID] = conflict
	delete(m.pendingConflicts, conflictID)
	return true
}

func operationKey(entityID string, operation map[string]any) string {
	path, _ := operation["path"].(string)
	return entityID + "\x00" + path + "\x00" + pyString(operation["value"])
}

func stateSnapshot(ent *entity.Entity) map[string]any {
	value, err := ent.Get(config.Current().StateGroup)
	if err != nil {
		return map[string]any{}
	}
	state, ok := value.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return deepCopyAny(state).(map[string]any)
}

// deepCopyAny copies maps and slices so transitions are stable snapshots.
func deepCopyAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = deepCopyAny(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for index, item := range typed {
			out[index] = deepCopyAny(item)
		}
		return out
	default:
		return value
	}
}

func newTickID() string {
	buffer := make([]byte, 4)
	if _, err := rand.Read(buffer); err == nil {
		return fmt.Sprintf("%x", buffer)
	}
	return fmt.Sprintf("%08x", time.Now().UnixNano()&0xffffffff)
}

// attachWriteListeners registers a per-tick listener on every entity so writes
// performed by @函数/事件 (not just rule ops) participate in branch_race checks.
func (m *StateMachine) attachWriteListeners() []func() {
	entities := m.store.ListAll()
	removers := make([]func(), 0, len(entities))
	for _, ent := range entities {
		removers = append(removers, ent.AddChangeListener(func(changed *entity.Entity, path string, _, newValue any) {
			m.noteStateWrite(changed, path, newValue)
		}))
	}
	return removers
}
