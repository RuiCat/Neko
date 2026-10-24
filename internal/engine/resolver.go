// Entity-bound path resolvers, field aliases and numeric clamping.
//
// Ported from the resolver and _clamp_value helpers in
// astrbox-plugin/state_machine/engine.py.
package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ruicat/astrbox/internal/config"
	"github.com/ruicat/astrbox/internal/entity"
)

// fieldAliases normalises multi-role wording back to the legacy "主人" names.
var fieldAliases = [][2]string{
	{"行为.对方行为", "行为.主人行为"},
	{"行为.对方同场景", "行为.主人同场景"},
	{"对方行为", "主人行为"},
	{"对方同场景", "主人同场景"},
}

func normalizeFieldAlias(path string) string {
	if path == "" {
		return path
	}
	for _, alias := range fieldAliases {
		path = strings.ReplaceAll(path, alias[0], alias[1])
	}
	return path
}

// deepSearchField finds a bare field anywhere in the entity data, preferring
// nested non-身体 matches, then top-level matches (Python's _deep_search_field).
func deepSearchField(data map[string]any, fieldName string) (any, bool) {
	type match struct {
		path  string
		value any
	}
	var matches []match
	var walk func(current map[string]any, prefix string)
	walk = func(current map[string]any, prefix string) {
		keys := make([]string, 0, len(current))
		for key := range current {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			value := current[key]
			if key == fieldName {
				matches = append(matches, match{path: prefix + key, value: value})
			}
			if child, ok := value.(map[string]any); ok {
				walk(child, prefix+key+".")
			}
		}
	}
	walk(data, "")
	if len(matches) == 0 {
		return nil, false
	}
	for _, item := range matches {
		if strings.Contains(item.path, ".") && !strings.HasPrefix(item.path, config.Current().DefaultStateSubgroup+".") {
			return item.value, true
		}
	}
	for _, item := range matches {
		if !strings.Contains(item.path, ".") {
			return item.value, true
		}
	}
	return matches[0].value, true
}

// getEntityField reads a field path, treating an empty path as the whole data
// snapshot.
func getEntityField(ent *entity.Entity, fields []string) (any, error) {
	if len(fields) == 0 || (len(fields) == 1 && fields[0] == "") {
		return ent.Get("")
	}
	return ent.Get(strings.Join(fields, "."))
}

// newEntityResolver builds the single-subject resolver: current entity id,
// {e} placeholders, then global entity references.
func (m *StateMachine) newEntityResolver(ent *entity.Entity) ValueResolver {
	var resolver ValueResolver
	resolver = func(path string) (any, error) {
		if value, handled := m.evaluateFuncCall(path, resolver, ent); handled {
			return value, nil
		}
		path = normalizeFieldAlias(path)
		if path == ent.ID || strings.HasPrefix(path, ent.ID+".") {
			return ent.Get(strings.TrimPrefix(strings.TrimPrefix(path, ent.ID), "."))
		}
		if strings.HasPrefix(path, "{e}") {
			fieldPath := strings.TrimPrefix(strings.TrimPrefix(path, "{e}"), ".")
			value, err := ent.Get(fieldPath)
			if err == nil {
				return value, nil
			}
			if strings.HasPrefix(fieldPath, config.Current().DefaultStateSubgroup+".") {
				data, _ := ent.Get("")
				if snapshot, ok := data.(map[string]any); ok {
					if found, ok := deepSearchField(snapshot, strings.TrimPrefix(fieldPath, config.Current().DefaultStateSubgroup+".")); ok {
						return found, nil
					}
				}
			}
			return nil, err
		}
		selector, fields, err := entity.ParseEntityPath(path)
		if err != nil {
			return nil, fmt.Errorf("无法解析: %s", path)
		}
		// Prefer the entity this rule is executing for when it matches the
		// selector; this keeps wildcard paths ("NPC[*].x") per-entity instead of
		// silently reading whichever entity sorts first.
		if m.store.EntityMatches(selector, ent) {
			if value, err := getEntityField(ent, fields); err == nil {
				return value, nil
			}
		}
		candidates := m.store.QuerySelector(selector)
		if len(candidates) > 0 {
			return getEntityField(candidates[0], fields)
		}
		return nil, fmt.Errorf("无法解析: %s", path)
	}
	return resolver
}

// newMultiResolver builds the multi-subject resolver: $1/$2 bindings, {e} as
// the first subject, then global entity references.
func (m *StateMachine) newMultiResolver(bindings map[int]*entity.Entity) ValueResolver {
	var resolver ValueResolver
	resolver = func(path string) (any, error) {
		if len(bindings) == 0 {
			return nil, fmt.Errorf("多主体绑定为空")
		}
		keys := make([]int, 0, len(bindings))
		for key := range bindings {
			keys = append(keys, key)
		}
		sort.Ints(keys)
		if value, handled := m.evaluateFuncCall(path, resolver, bindings[keys[0]]); handled {
			return value, nil
		}
		for _, key := range keys {
			varName := fmt.Sprintf("$%d", key)
			if path == varName {
				return bindings[key].Get("")
			}
			if strings.HasPrefix(path, varName+".") {
				return bindings[key].Get(strings.TrimPrefix(path, varName+"."))
			}
		}
		if strings.HasPrefix(path, "{e}") {
			first := bindings[keys[0]]
			fieldPath := strings.TrimPrefix(strings.TrimPrefix(path, "{e}"), ".")
			return first.Get(fieldPath)
		}
		selector, fields, err := entity.ParseEntityPath(path)
		if err != nil {
			return nil, fmt.Errorf("无法解析: %s", path)
		}
		candidates := m.store.QuerySelector(selector)
		if len(candidates) == 0 {
			return nil, fmt.Errorf("无法解析: %s", path)
		}
		return getEntityField(candidates[0], fields)
	}
	return resolver
}

// clampValue clamps numeric values using template __annotations__ min/max,
// falling back to suffix matching when the path is not exact.
func (m *StateMachine) clampValue(target *entity.Entity, path string, value any) any {
	number, numeric := toFloatValue(value)
	if !numeric {
		return value
	}
	for _, typeName := range entity.TypeChain(target.Type) {
		template := m.store.GetTemplate(typeName)
		if template == nil {
			continue
		}
		annotations, ok := template["__annotations__"].(map[string]any)
		if !ok {
			continue
		}
		rawSchema, found := annotations[path]
		if !found && strings.Contains(path, ".") {
			tail := "." + path[strings.LastIndex(path, ".")+1:]
			keys := make([]string, 0, len(annotations))
			for key := range annotations {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				if strings.HasSuffix(key, tail) {
					rawSchema, found = annotations[key], true
					break
				}
			}
		}
		if !found {
			continue
		}
		schema, ok := rawSchema.(map[string]any)
		if !ok {
			continue
		}
		if low, ok := toFloatValue(schema["min"]); ok && number < low {
			return low
		}
		if high, ok := toFloatValue(schema["max"]); ok && number > high {
			return high
		}
		return value
	}
	return value
}
