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
const SCHEMA_VERSION = 1

// Schema is the versioned hard-state definition. Group names the top-level
// entity field that holds machine states; Entities restricts capture to the
// listed IDs (empty means every entity).
type Schema struct {
	Version  int
	Group    string
	Entities []string
}

// DefaultSchema returns the bundled schema built from the active domain
// vocabulary, so a custom state group is honoured without code changes.
func DefaultSchema() Schema {
	return Schema{Version: SCHEMA_VERSION, Group: config.Current().StateGroup}
}

// WithEntities returns a copy restricted to the given entity IDs. Passing no
// IDs clears the restriction.
func (s Schema) WithEntities(ids ...string) Schema {
	copied := append([]string(nil), ids...)
	s.Entities = copied
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
	return path == s.Group || strings.HasPrefix(path, s.Group+".")
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
