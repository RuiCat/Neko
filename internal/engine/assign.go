// Text assignment parsing and application ("安全等级=red", "状态.身体:疲惫").
//
// This file is the Go port of the assignment half of
// astrbox-plugin/state_machine/effect_parser.py.
package engine

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/ruicat/astrbox/internal/config"
	"github.com/ruicat/astrbox/internal/entity"
)

// Assignment is one parsed "name op value" assignment.
type Assignment struct {
	Path  string `json:"path"`
	Value any    `json:"value"`
}

// assignMap maps assignment words to entity field paths (ported verbatim, plus
// the shorthand "情绪" the Go spec calls out explicitly).
var assignMap = map[string]string{
	"安全等级": "行为.安全等级",
	"阶段":   "作息.阶段",
	"清洁状态": "清洁.状态",
	"身体状态": "状态.身体",
	"位置转移": "肉棒追踪.位置",
	"当前位置": "肉棒追踪.位置",
	"情绪":   "状态.情绪",
}

var assignPattern = regexp.MustCompile(`^([\p{Han}\w.]+)\s*[:：=]\s*([^\s,，;；]+)`)

var safetySeverity = map[string]int{"green": 0, "yellow": 1, "orange": 2, "red": 3}

// ParseAssign extracts assignments from a text.
func ParseAssign(text string) []Assignment {
	assignments := []Assignment{}
	if text == "" {
		return assignments
	}
	text = normalizeASCII(text)
	parts := strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == '，' })
	for _, part := range parts {
		part = strings.TrimSpace(part)
		match := assignPattern.FindStringSubmatch(part)
		if match == nil {
			continue
		}
		name := strings.TrimSpace(match[1])
		path, ok := assignMap[name]
		if !ok && strings.Contains(name, ".") {
			path = name
		}
		path = config.Current().NormalizePath(path)
		if path == "" {
			continue
		}
		assignments = append(assignments, Assignment{Path: path, Value: parseAssignValue(match[2])})
	}
	return assignments
}

func parseAssignValue(raw string) any {
	raw = strings.TrimSpace(raw)
	if number, err := strconv.Atoi(raw); err == nil {
		return number
	}
	if number, err := strconv.ParseFloat(raw, 64); err == nil {
		return number
	}
	return raw
}

// ApplyAssign writes assignments to an entity. Safety level is special: the
// highest severity in this batch wins, everything else writes in order.
func ApplyAssign(target *entity.Entity, text string) []Assignment {
	applied := []Assignment{}
	if target == nil {
		return applied
	}
	assignments := ParseAssign(text)
	var safetyValues []any
	for _, assignment := range assignments {
		if assignment.Path == "行为.安全等级" {
			safetyValues = append(safetyValues, assignment.Value)
		}
	}
	if len(safetyValues) > 0 {
		best := safetyValues[0]
		bestSeverity := severityOf(best)
		for _, value := range safetyValues[1:] {
			if severityOf(value) > bestSeverity {
				best, bestSeverity = value, severityOf(value)
			}
		}
		if err := target.Set("行为.安全等级", best); err == nil {
			applied = append(applied, Assignment{Path: "行为.安全等级", Value: best})
		}
		remaining := assignments[:0]
		for _, assignment := range assignments {
			if assignment.Path != "行为.安全等级" {
				remaining = append(remaining, assignment)
			}
		}
		assignments = remaining
	}
	for _, assignment := range assignments {
		if err := target.Set(assignment.Path, assignment.Value); err == nil {
			applied = append(applied, assignment)
		}
	}
	return applied
}

func severityOf(value any) int {
	if severity, ok := safetySeverity[pyString(value)]; ok {
		return severity
	}
	return 0
}
