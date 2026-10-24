// Entity-bound path reads and writes for interpreted functions: local variables
// hold entity ids, {e} is the current entity and bare roots read relative paths.
package function

import (
	"strings"

	"github.com/ruicat/astrbox/internal/entity"
)

// getPath resolves a dotted path to a value, returning nil when it is missing.
func (r *runner) getPath(path *pathExpression) any {
	if path.current {
		return readEntityField(r.current, strings.Join(path.fields, "."))
	}
	value, isLocal := r.locals[path.root]
	if isLocal {
		if len(path.fields) == 0 {
			return value
		}
		if entityID, isText := value.(string); isText {
			if target := r.executor.lookupEntity(entityID); target != nil {
				return readEntityField(target, strings.Join(path.fields, "."))
			}
		}
		if object, isMap := value.(map[string]any); isMap {
			return lookupMapPath(object, path.fields)
		}
	}
	if target := r.executor.lookupEntity(path.root); target != nil {
		return readEntityField(target, strings.Join(path.fields, "."))
	}
	if len(path.fields) == 0 {
		return readEntityField(r.current, path.root)
	}
	return readEntityField(r.current, path.fullPath())
}

// assignPath writes value to a path; bare names stay local variables.
func (r *runner) assignPath(path *pathExpression, value any) {
	if !path.current && len(path.fields) == 0 {
		r.locals[path.root] = value
		return
	}
	target, field := r.resolveWriteTarget(path)
	if target == nil || field == "" {
		return
	}
	_ = target.Set(field, value)
}

// resolveWriteTarget maps a path to the entity and field it should write.
func (r *runner) resolveWriteTarget(path *pathExpression) (*entity.Entity, string) {
	field := strings.Join(path.fields, ".")
	if path.current {
		return r.current, field
	}
	if value, isLocal := r.locals[path.root]; isLocal {
		if entityID, isText := value.(string); isText {
			if target := r.executor.lookupEntity(entityID); target != nil {
				return target, field
			}
		}
	}
	if target := r.executor.lookupEntity(path.root); target != nil {
		return target, field
	}
	return r.current, path.fullPath()
}

// readEntityField reads one field, swallowing missing-path errors.
func readEntityField(target *entity.Entity, field string) any {
	if target == nil || field == "" {
		return nil
	}
	value, err := target.Get(field)
	if err != nil {
		return nil
	}
	return value
}

// lookupMapPath walks a nested map and returns nil when a segment is missing.
func lookupMapPath(data map[string]any, fields []string) any {
	var current any = data
	for _, field := range fields {
		object, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = object[field]
	}
	return current
}
