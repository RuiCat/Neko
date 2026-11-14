// templateGuard.go validates @实体 composition before a script is committed and
// guards template removal when another registered template depends on it.
package registry

import (
	"fmt"
	"sort"
	"strings"

	"github.com/RuiCat/Neko/internal/compiler"
	"github.com/RuiCat/Neko/internal/entity"
)

// entryTemplateTypes returns the template names contributed by one script.
func (r *Registry) entryTemplateTypes(name string) map[string]bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]bool{}
	if entry := r.entries[name]; entry != nil {
		for typeName := range entry.Templates {
			out[typeName] = true
		}
	}
	return out
}

// validateCompiledTemplates enforces strict single-owner composition for @实体
// templates. skipTypes are template names this script already contributed to;
// re-applying it must not look like a duplicate against its own old fields.
func (r *Registry) validateCompiledTemplates(compiled *compiler.Result, skipTypes map[string]bool) []ApplyError {
	templates := r.store.Templates()
	parents := r.store.TemplateParentsSnapshot()
	errs := []ApplyError{}
	affected := map[string]bool{}

	for typeName, incoming := range compiled.Templates {
		incomingParents := compiled.TemplateParents[typeName]
		existingParents := parents[typeName]
		if len(incomingParents) > 0 && len(existingParents) > 0 &&
			!entity.SameParents(incomingParents, existingParents) {
			errs = append(errs, templateApplyError("模板 '%s' 的父列表与已注册声明不一致: 新 [%s]，已注册 [%s]",
				typeName, strings.Join(incomingParents, ", "), strings.Join(existingParents, ", ")))
			continue
		}
		if len(incomingParents) > 0 {
			parents[typeName] = entity.CopyParents(incomingParents)
		}
		existing := templates[typeName]
		if existing != nil && !skipTypes[typeName] &&
			(len(incomingParents) > 0 || len(parents[typeName]) > 0) {
			for _, err := range entity.CheckDuplicatePaths(typeName, existing, incoming) {
				errs = append(errs, templateApplyError("%s", err.Error()))
			}
		}
		templates[typeName] = entity.DeepMerge(existing, incoming)
		affected[typeName] = true
	}

	markAffectedDescendants(templates, parents, affected)
	for typeName := range affected {
		if len(parents[typeName]) == 0 {
			continue // legacy @定义 types keep the last-writer-wins behaviour.
		}
		for _, err := range entity.ValidateComposition(typeName, templates[typeName], templates, parents) {
			errs = append(errs, templateApplyError("%s", err.Error()))
		}
	}
	return errs
}

// markAffectedDescendants extends affected to every template whose composition
// graph reaches one of the changed templates.
func markAffectedDescendants(templates map[string]map[string]any, parents map[string][]string, affected map[string]bool) {
	names := make([]string, 0, len(templates))
	for name := range templates {
		names = append(names, name)
	}
	sort.Strings(names)
	for changed := true; changed; {
		changed = false
		for _, name := range names {
			if affected[name] {
				continue
			}
			for _, parent := range parents[name] {
				if affected[parent] {
					affected[name], changed = true, true
					break
				}
			}
			if affected[name] {
				continue
			}
			for changedName := range affected {
				if strings.HasPrefix(name, changedName+":") {
					affected[name], changed = true, true
					break
				}
			}
		}
	}
}

// applyRemovalConflicts reports dependents that would lose a parent because a
// re-applied script no longer contributes one of its old templates.
func (r *Registry) applyRemovalConflicts(name string, compiled *compiler.Result) []string {
	old := r.entries[name]
	if old == nil {
		return nil
	}
	removed := map[string]bool{}
	for typeName := range old.Templates {
		if _, stillProvided := compiled.Templates[typeName]; stillProvided {
			continue
		}
		if r.refCountLocked("template", typeName) == 1 {
			removed[typeName] = true
		}
	}
	return r.dependencyConflicts(removed)
}

// dependencyConflicts reports remaining templates that would lose a parent when
// the removed set disappears.
func (r *Registry) dependencyConflicts(removed map[string]bool) []string {
	if len(removed) == 0 {
		return nil
	}
	conflicts := []string{}
	for _, name := range r.store.ListTemplates() {
		if removed[name] {
			continue
		}
		for _, parent := range r.store.GetTemplateParents(name) {
			if removed[parent] {
				conflicts = append(conflicts, fmt.Sprintf("模板 '%s' 依赖即将删除的父模板 '%s'", name, parent))
			}
		}
		for removedName := range removed {
			if strings.HasPrefix(name, removedName+":") {
				conflicts = append(conflicts, fmt.Sprintf("模板 '%s' 依赖即将删除的类型 '%s'", name, removedName))
			}
		}
	}
	sort.Strings(conflicts)
	return conflicts
}

func templateApplyError(format string, args ...any) ApplyError {
	return ApplyError{
		Severity: "error",
		Block:    "模板组合",
		Message:  fmt.Sprintf(format, args...),
		Hint:     "组合要求严格单所有者：字段不得重复、子模板不得覆盖父模板、父模板必须存在且无环",
	}
}
