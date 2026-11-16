// Text assignment parsing and application ("字段=值", "路径:值").
//
// This file is the Go port of the assignment half of
// astrbox-plugin/state_machine/effect_parser.py.
package engine

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/RuiCat/Neko/internal/config"
	"github.com/RuiCat/Neko/internal/entity"
)

// Assignment is one parsed "name op value" assignment.
type Assignment struct {
	Path  string `json:"path"`
	Value any    `json:"value"`
}

var assignPattern = regexp.MustCompile(`^([\p{Han}\w.]+)\s*[:：=]\s*([^\s,，;；]+)`)

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
		// 词 → 路径的映射是数据（config.assign_map），源码不写死旧字段。
		path, ok := config.Current().AssignMap[name]
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

// ApplyAssign writes assignments to an entity in order. Severity merging is
// business policy and belongs to scripts, not to the engine.
func ApplyAssign(target *entity.Entity, text string) []Assignment {
	applied := []Assignment{}
	if target == nil {
		return applied
	}
	for _, assignment := range ParseAssign(text) {
		if err := target.Set(assignment.Path, assignment.Value); err == nil {
			applied = append(applied, assignment)
		}
	}
	return applied
}
