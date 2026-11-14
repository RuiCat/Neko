// composition.go resolves template composition graphs and validates that
// composed templates stay disjoint: no field may be owned by two templates.
package entity

import (
	"fmt"
	"sort"
	"strings"
)

// EffectiveParents returns the templates merged before typeName: the implicit
// colon chain (`A:B:C` inherits `A` and `A:B`) followed by the explicit parents
// declared by an @实体 block, in declaration order.
func EffectiveParents(typeName string, parents map[string][]string) []string {
	chain := TypeChain(typeName)
	out := make([]string, 0, len(chain)+len(parents[typeName]))
	if len(chain) > 1 {
		out = append(out, chain[:len(chain)-1]...)
	}
	for _, parent := range parents[typeName] {
		if parent == "" || parent == typeName {
			continue
		}
		out = append(out, parent)
	}
	return out
}

// ResolveTemplateOrder returns every template that contributes to typeName in
// merge order (ancestors first, typeName last). Explicitly declared parents must
// exist; a missing implicit colon ancestor is tolerated for legacy types.
func ResolveTemplateOrder(typeName string, templates map[string]map[string]any, parents map[string][]string) ([]string, error) {
	order := []string{}
	state := map[string]int{} // 1 = visiting, 2 = done
	var visit func(name string, stack []string) error
	visit = func(name string, stack []string) error {
		switch state[name] {
		case 2:
			return nil
		case 1:
			return fmt.Errorf("模板组合存在循环依赖: %s", strings.Join(append(stack, name), " -> "))
		}
		state[name] = 1
		for _, parent := range EffectiveParents(name, parents) {
			if parent == name {
				return fmt.Errorf("模板 '%s' 不能把自己声明为父模板", name)
			}
			if _, ok := templates[parent]; !ok {
				if isImplicitPrefix(name, parent) {
					continue
				}
				return fmt.Errorf("组合模板 '%s' 的父模板 '%s' 不存在", name, parent)
			}
			if err := visit(parent, append(stack, name)); err != nil {
				return err
			}
		}
		state[name] = 2
		order = append(order, name)
		return nil
	}
	if err := visit(typeName, nil); err != nil {
		return nil, err
	}
	return order, nil
}

// TemplateOrder is ResolveTemplateOrder with a legacy fallback on error. It is
// used by read paths that must not fail, such as template fallback lookups.
func TemplateOrder(typeName string, templates map[string]map[string]any, parents map[string][]string) []string {
	if order, err := ResolveTemplateOrder(typeName, templates, parents); err == nil {
		return order
	}
	return TypeChain(typeName)
}

func isImplicitPrefix(typeName, parent string) bool {
	return parent != typeName && strings.HasPrefix(typeName, parent+":")
}

// TemplateLeafPaths flattens one template into leaf dotted paths, skipping the
// metadata tables that are not entity data.
func TemplateLeafPaths(data map[string]any) map[string]any {
	out := map[string]any{}
	collectTemplatePaths(data, "", out)
	return out
}

func collectTemplatePaths(data map[string]any, prefix string, out map[string]any) {
	for key, value := range data {
		if key == "__annotations__" || key == "__parents__" {
			continue
		}
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		if child, ok := value.(map[string]any); ok {
			collectTemplatePaths(child, path, out)
			continue
		}
		out[path] = value
	}
}

// OverlappingPaths returns paths that collide, including one path being a
// prefix of another (a scalar vs a nested object).
func OverlappingPaths(left, right map[string]any) []string {
	conflicts := []string{}
	for leftPath := range left {
		for rightPath := range right {
			if pathsOverlap(leftPath, rightPath) {
				conflicts = append(conflicts, leftPath+" vs "+rightPath)
			}
		}
	}
	sort.Strings(conflicts)
	return conflicts
}

func pathsOverlap(left, right string) bool {
	if left == right {
		return true
	}
	return strings.HasPrefix(left, right+".") || strings.HasPrefix(right, left+".")
}

// ValidateComposition enforces the strict single-owner rule for one composed
// type: its own fields may not repeat any ancestor field, and two ancestors may
// not contribute the same path.
func ValidateComposition(typeName string, own map[string]any, templates map[string]map[string]any, parents map[string][]string) []error {
	order, err := ResolveTemplateOrder(typeName, templates, parents)
	if err != nil {
		return []error{err}
	}
	errs := []error{}
	ownPaths := TemplateLeafPaths(own)
	merged := map[string]map[string]any{}
	for _, ancestor := range order {
		if ancestor == typeName {
			continue
		}
		paths, ok := templates[ancestor]
		if !ok {
			continue
		}
		ancestorPaths := TemplateLeafPaths(paths)
		if conflicts := OverlappingPaths(ownPaths, ancestorPaths); len(conflicts) > 0 {
			errs = append(errs, fmt.Errorf("模板 '%s' 不得重定义父模板 '%s' 的字段: %s",
				typeName, ancestor, strings.Join(conflicts, "; ")))
		}
		for other, otherPaths := range merged {
			if conflicts := OverlappingPaths(ancestorPaths, otherPaths); len(conflicts) > 0 {
				errs = append(errs, fmt.Errorf("父模板 '%s' 与 '%s' 的字段冲突: %s",
					ancestor, other, strings.Join(conflicts, "; ")))
			}
		}
		merged[ancestor] = ancestorPaths
	}
	return errs
}

// CheckDuplicatePaths rejects a contribution that repeats a path already owned
// by the same template. Repeating a path is an error even when the value is
// identical, because strict composition requires exactly one owner.
func CheckDuplicatePaths(typeName string, existing, incoming map[string]any) []error {
	if existing == nil || incoming == nil {
		return nil
	}
	conflicts := OverlappingPaths(TemplateLeafPaths(existing), TemplateLeafPaths(incoming))
	if len(conflicts) == 0 {
		return nil
	}
	return []error{fmt.Errorf("模板 '%s' 的字段被重复定义: %s", typeName, strings.Join(conflicts, "; "))}
}

// CopyParents returns a defensive copy of a parent list.
func CopyParents(parents []string) []string {
	if len(parents) == 0 {
		return nil
	}
	return append([]string(nil), parents...)
}

// SameParents reports whether two parent lists are equal.
func SameParents(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
