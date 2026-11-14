// eventKind.go maps a tick outcome onto the gate's event vocabulary, so tick
// records carry real event kinds (relationship/emotion/self/conflict) instead
// of a constant "turn".
package mcpserver

import (
	"strings"

	"github.com/RuiCat/Neko/internal/engine"
)

// eventKindPriority orders the kinds a tick can imply; the first match wins,
// so the mapping is deterministic when one tick touches several state groups.
var eventKindPriority = []string{"relationship", "emotion", "self"}

// eventKindOf maps a tick outcome onto the gate's event vocabulary. Conflicts
// win outright; otherwise the state paths the tick wrote decide the kind.
func eventKindOf(result engine.TickResult) string {
	if len(result.Conflicts) > 0 {
		return "conflict"
	}
	seen := make(map[string]bool, len(eventKindPriority))
	for _, transition := range result.Transitions {
		for _, operation := range transition.Operations {
			path, _ := operation["path"].(string)
			if kind := kindFromOperationPath(path); kind != "" {
				seen[kind] = true
			}
		}
	}
	for _, kind := range eventKindPriority {
		if seen[kind] {
			return kind
		}
	}
	return "turn"
}

// kindFromOperationPath maps one state operation path onto a gate event kind.
func kindFromOperationPath(path string) string {
	switch {
	case strings.Contains(path, "关系"):
		return "relationship"
	case strings.Contains(path, "情绪"), strings.Contains(path, "心情"):
		return "emotion"
	case strings.Contains(path, "睡眠"), strings.Contains(path, "身体"), strings.Contains(path, "疼痛"):
		return "self"
	}
	return ""
}
