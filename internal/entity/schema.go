package entity

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// FieldSchema is the constraint attached to one annotated field.
type FieldSchema struct {
	FieldName string
	Type      string
	Event     string
	Enum      []any
	Min       *float64
	Max       *float64
	Readonly  bool
	Nullable  bool
}

// Validate checks a value against the schema and returns a message when invalid.
func (s *FieldSchema) Validate(value any) string {
	if value == nil {
		if s.Nullable {
			return ""
		}
		return fmt.Sprintf("字段 %s 不允许 null", s.FieldName)
	}
	switch s.Type {
	case "int":
		if !isInteger(value) {
			return fmt.Sprintf("字段 %s 需要 int", s.FieldName)
		}
	case "float":
		if !isNumber(value) {
			return fmt.Sprintf("字段 %s 需要 float", s.FieldName)
		}
	case "str":
		if _, ok := value.(string); !ok {
			return fmt.Sprintf("字段 %s 需要 str", s.FieldName)
		}
	case "bool":
		if _, ok := value.(bool); !ok {
			return fmt.Sprintf("字段 %s 需要 bool", s.FieldName)
		}
	}
	if number, ok := toFloat(value); ok {
		if s.Min != nil && number < *s.Min {
			return fmt.Sprintf("字段 %s 低于最小值 %v", s.FieldName, *s.Min)
		}
		if s.Max != nil && number > *s.Max {
			return fmt.Sprintf("字段 %s 超过最大值 %v", s.FieldName, *s.Max)
		}
	}
	if s.Enum != nil {
		for _, allowed := range s.Enum {
			if fmt.Sprint(allowed) == fmt.Sprint(value) {
				return ""
			}
		}
		return fmt.Sprintf("字段 %s 值 '%v' 不在枚举中", s.FieldName, value)
	}
	return ""
}

var annotPattern = regexp.MustCompile(`^([^\[\]]+?)\s*\[([^\]]*)\]\s*(?::\s*(.*))?$`)

// ParseAnnotatedField parses "体力[int,@事件,min:0]: 75".
func ParseAnnotatedField(text string) (string, *FieldSchema, any, bool) {
	match := annotPattern.FindStringSubmatch(strings.TrimSpace(text))
	if match == nil {
		return "", nil, nil, false
	}
	name := strings.TrimSpace(match[1])
	schema := &FieldSchema{FieldName: name}
	for _, annotation := range strings.Split(match[2], ",") {
		annotation = strings.TrimSpace(annotation)
		switch {
		case annotation == "":
		case strings.HasPrefix(annotation, "@"):
			schema.Event = annotation[1:]
		case annotation == "int", annotation == "float", annotation == "str", annotation == "bool":
			schema.Type = annotation
		case annotation == "readonly":
			schema.Readonly = true
		case annotation == "nullable":
			schema.Nullable = true
		case strings.HasPrefix(annotation, "enum:"):
			for _, item := range strings.Split(annotation[5:], ";") {
				schema.Enum = append(schema.Enum, strings.TrimSpace(item))
			}
		case strings.HasPrefix(annotation, "min:"):
			if value, err := strconv.ParseFloat(annotation[4:], 64); err == nil {
				schema.Min = &value
			}
		case strings.HasPrefix(annotation, "max:"):
			if value, err := strconv.ParseFloat(annotation[4:], 64); err == nil {
				schema.Max = &value
			}
		}
	}
	return name, schema, parseDefault(match[3]), true
}

func parseDefault(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if len(raw) >= 2 && ((raw[0] == '"' && raw[len(raw)-1] == '"') ||
		(raw[0] == '\'' && raw[len(raw)-1] == '\'')) {
		return raw[1 : len(raw)-1]
	}
	if raw == "true" {
		return true
	}
	if raw == "false" {
		return false
	}
	if value, err := strconv.Atoi(raw); err == nil {
		return value
	}
	if value, err := strconv.ParseFloat(raw, 64); err == nil {
		return value
	}
	return raw
}

// EntitySchema maps full field paths to their schema.
type EntitySchema struct {
	fields map[string]*FieldSchema
}

// NewEntitySchema returns an empty schema.
func NewEntitySchema() *EntitySchema { return &EntitySchema{fields: map[string]*FieldSchema{}} }

// Add stores one field schema.
func (s *EntitySchema) Add(path string, schema *FieldSchema) { s.fields[path] = schema }

// AddFromAnnotations builds the schema from a template's __annotations__ table.
func (s *EntitySchema) AddFromAnnotations(data map[string]any) {
	raw, ok := data["__annotations__"].(map[string]any)
	if !ok {
		return
	}
	for path, meta := range raw {
		entry, ok := meta.(map[string]any)
		if !ok {
			continue
		}
		parts := strings.Split(path, ".")
		schema := &FieldSchema{FieldName: parts[len(parts)-1]}
		if value, ok := entry["type"].(string); ok {
			schema.Type = value
		}
		if value, ok := entry["event"].(string); ok {
			schema.Event = value
		}
		if value, ok := entry["enum"].([]any); ok {
			schema.Enum = value
		}
		if value, ok := toFloat(entry["min"]); ok {
			schema.Min = &value
		}
		if value, ok := toFloat(entry["max"]); ok {
			schema.Max = &value
		}
		schema.Readonly, _ = entry["readonly"].(bool)
		schema.Nullable, _ = entry["nullable"].(bool)
		s.fields[path] = schema
	}
}

// AddFromData walks annotated keys and records their constraints.
func (s *EntitySchema) AddFromData(data map[string]any, prefix string) {
	for key, value := range data {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		if name, schema, _, ok := ParseAnnotatedField(key); ok {
			fieldPath := name
			if prefix != "" {
				fieldPath = prefix + "." + name
			}
			s.fields[fieldPath] = schema
			if child, ok := value.(map[string]any); ok {
				s.AddFromData(child, fieldPath)
			}
			continue
		}
		if child, ok := value.(map[string]any); ok {
			s.AddFromData(child, path)
		}
	}
}

// Get returns the closest schema for a path (exact, then nearest parent).
func (s *EntitySchema) Get(path string) *FieldSchema {
	if schema, ok := s.fields[path]; ok {
		return schema
	}
	parts := strings.Split(path, ".")
	for i := len(parts) - 1; i > 0; i-- {
		if schema, ok := s.fields[strings.Join(parts[:i], ".")]; ok {
			return schema
		}
	}
	return nil
}

// GetEvent returns the event bound to a path, if any.
func (s *EntitySchema) GetEvent(path string) string {
	if schema := s.Get(path); schema != nil {
		return schema.Event
	}
	return ""
}

// Validate checks a value for the given path.
func (s *EntitySchema) Validate(path string, value any) string {
	if schema := s.Get(path); schema != nil {
		return schema.Validate(value)
	}
	return ""
}

// All returns a copy of the field map.
func (s *EntitySchema) All() map[string]*FieldSchema {
	out := make(map[string]*FieldSchema, len(s.fields))
	for key, value := range s.fields {
		out[key] = value
	}
	return out
}

// MergeInherited fills unset child constraints from the parent schema.
func (s *EntitySchema) MergeInherited(parent *EntitySchema) {
	for path, parentSchema := range parent.All() {
		child, ok := s.fields[path]
		if !ok {
			s.fields[path] = parentSchema
			continue
		}
		if child.Type == "" {
			child.Type = parentSchema.Type
		}
		if child.Event == "" {
			child.Event = parentSchema.Event
		}
		if child.Enum == nil {
			child.Enum = parentSchema.Enum
		}
		if child.Min == nil {
			child.Min = parentSchema.Min
		}
		if child.Max == nil {
			child.Max = parentSchema.Max
		}
		if !child.Nullable {
			child.Nullable = parentSchema.Nullable
		}
	}
}

func isNumber(value any) bool {
	switch value.(type) {
	case int, int64, float64, float32, int32:
		return true
	}
	return false
}

func isInteger(value any) bool {
	switch typed := value.(type) {
	case int, int64, int32:
		return true
	case float64:
		return typed == float64(int64(typed))
	case float32:
		return typed == float32(int64(typed))
	}
	return false
}

func toFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case int32:
		return float64(typed), true
	}
	return 0, false
}
