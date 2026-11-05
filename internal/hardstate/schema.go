// schema.go defines which entity fields count as the hard state and versions
// that definition. The hard state is the only truth source of the learning
// loop: snapshots, compensation packages and the nightly diff all resolve
// against exactly this list.
package hardstate

import (
	"fmt"
	"strings"

	"github.com/RuiCat/Neko/internal/config"
)

// SCHEMA_VERSION is bumped whenever the hard-state definition changes; old
// snapshots remain valid records of the schema they were taken with.
//
// v2 adds the field-level path list (C0): the state group alone was too narrow
// for the relationship/task/scene fields the learning loop resolves against.
const SCHEMA_VERSION = 2

// Schema is the versioned hard-state definition. Group names the top-level
// entity field that holds machine states; Paths adds field-level dotted paths
// (relative to the entity) outside that group; Entities restricts capture to
// the listed IDs (empty means every entity).
type Schema struct {
	Version  int
	Group    string
	Paths    []string
	Entities []string
}

// DefaultSchema returns the bundled schema built from the active domain
// vocabulary, so a custom state group is honoured without code changes.
func DefaultSchema() Schema {
	return Schema{
		Version: SCHEMA_VERSION,
		Group:   config.Current().StateGroup,
		Paths:   DefaultPaths(),
	}
}

// DefaultPaths returns the field-level part of the hard-state subset: the
// fields the compensation package, judge and nightly diff resolve against
// outside the configured state group. Callers get a copy.
func DefaultPaths() []string {
	return []string{
		"身体.体力",
		"心情.当前",
		"关系.亲密",
		"位置.场景",
		"睡眠.疲劳",
		"社交.互动需求",
		"任务.当前任务",
	}
}

// WithEntities returns a copy restricted to the given entity IDs. Passing no
// IDs clears the restriction.
func (s Schema) WithEntities(ids ...string) Schema {
	s.Entities = append([]string(nil), ids...)
	s.Paths = append([]string(nil), s.Paths...)
	return s
}

// Validate rejects definitions that cannot be resolved as dotted paths.
func (s Schema) Validate() error {
	if s.Version < 1 {
		return fmt.Errorf("hardstate: 版本 %d < 1", s.Version)
	}
	group := strings.TrimSpace(s.Group)
	if group == "" {
		return fmt.Errorf("hardstate: 状态组不能为空")
	}
	if strings.ContainsAny(group, ".[] ") {
		return fmt.Errorf("hardstate: 状态组 %q 不能包含 . [ ] 或空格", group)
	}
	seen := make(map[string]bool, len(s.Paths))
	for _, raw := range s.Paths {
		path := strings.TrimSpace(raw)
		if path == "" {
			return fmt.Errorf("hardstate: 硬状态路径不能为空")
		}
		if strings.ContainsAny(path, " []") || strings.HasPrefix(path, ".") || strings.HasSuffix(path, ".") || strings.Contains(path, "..") {
			return fmt.Errorf("hardstate: 硬状态路径 %q 不是合法的点分路径", raw)
		}
		if path == group || strings.HasPrefix(path, group+".") {
			return fmt.Errorf("hardstate: 路径 %q 已在状态组 %q 内，不能重复声明", path, group)
		}
		if seen[path] {
			return fmt.Errorf("hardstate: 硬状态路径 %q 重复", path)
		}
		seen[path] = true
	}
	for _, id := range s.Entities {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("hardstate: 实体 ID 不能为空白")
		}
	}
	return nil
}

// Covers reports whether path belongs to the hard-state group (the group
// itself or one of its sub-paths).
func (s Schema) Covers(path string) bool {
	if s.Version < 1 || strings.TrimSpace(s.Group) == "" {
		return false
	}
	path = strings.TrimSpace(path)
	if path == s.Group || strings.HasPrefix(path, s.Group+".") {
		return true
	}
	for _, raw := range s.Paths {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		if path == entry || strings.HasPrefix(path, entry+".") {
			return true
		}
	}
	return false
}

// includes reports whether entityID takes part in this schema.
func (s Schema) includes(entityID string) bool {
	if len(s.Entities) == 0 {
		return true
	}
	for _, id := range s.Entities {
		if id == entityID {
			return true
		}
	}
	return false
}
