// sceneOps.go resolves scene targets and keeps one scene's event stream bounded.
//
// The rule language names the default scene token (`场景`); when no entity has
// that id, the token means "the actor's current scene" and resolves through the
// configured current-scene path. Scene event writes are also trimmed here so an
// event stream cannot grow without bound.
package engine

import (
	"strings"

	"github.com/RuiCat/Neko/internal/config"
	"github.com/RuiCat/Neko/internal/entity"
)

// resolveSceneTarget returns the entity a scene operation writes to. A real
// entity with that id wins; the default-scene token falls back to the actor's
// current scene; any other unknown id is nil (the op is skipped).
func (m *StateMachine) resolveSceneTarget(ent *entity.Entity, ref string) *entity.Entity {
	if strings.TrimSpace(ref) == "" {
		ref = config.Current().DefaultScene
	}
	if target := m.store.Get(ref); target != nil {
		return target
	}
	if ref != config.Current().DefaultScene {
		return nil
	}
	sceneID := currentSceneID(ent)
	if sceneID == "" {
		return nil
	}
	return m.store.Get(sceneID)
}

// currentSceneID reads the actor's current scene id; an absent or non-string
// value yields "" (no target).
func currentSceneID(ent *entity.Entity) string {
	if ent == nil {
		return ""
	}
	value, ok := ent.GetOrNone(config.Current().ScenePath()).(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

// appendSceneEvent pushes one event onto a scene's event stream and trims the
// list to the configured limit (0 = unbounded).
func (m *StateMachine) appendSceneEvent(scene *entity.Entity, eventField string, value any) error {
	events, _ := scene.GetOrNone(eventField).([]any)
	events = append(events, value)
	return scene.Set(eventField, trimSceneEvents(events, config.Current().SceneEventLimit))
}

// trimSceneEvents keeps the newest entries when the stream exceeds the limit.
func trimSceneEvents(events []any, limit int) []any {
	if limit <= 0 || len(events) <= limit {
		return events
	}
	return events[len(events)-limit:]
}
