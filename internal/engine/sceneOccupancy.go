// sceneOccupancy.go keeps every scene's occupant list derived from the actors'
// current-scene field, so one NPC moving is a scene change the other agents can
// observe on the board (multi-agent scene state sync).
package engine

import (
	"sort"
	"strings"

	"github.com/RuiCat/Neko/internal/config"
	"github.com/RuiCat/Neko/internal/entity"
)

// sceneTypeName is the entity type of scene entities.
const sceneTypeName = "场景"

// reconcileSceneOccupancy derives each scene's occupant ids from every actor's
// current-scene field and writes the lists that changed. It runs at the end of a
// tick: the tick's write listeners observe these writes, so the event board
// reports 在场 next to the actor's own movement. A scene with no occupant and no
// field yet is left untouched (no empty field is invented).
func (m *StateMachine) reconcileSceneOccupancy() {
	cfg := config.Current()
	field := strings.TrimSpace(cfg.SceneOccupancyField)
	if field == "" {
		return
	}
	occupants := make(map[string][]string)
	for _, actor := range m.store.ListAll() {
		sceneID := actorSceneID(actor, cfg.ScenePath())
		if sceneID == "" {
			continue
		}
		occupants[sceneID] = append(occupants[sceneID], actor.ID)
	}
	for sceneID := range occupants {
		sort.Strings(occupants[sceneID])
	}
	for _, scene := range m.store.ListAll() {
		if scene.Type != sceneTypeName {
			continue
		}
		ids := occupants[scene.ID]
		current := sceneOccupantIDs(scene, field)
		if equalStringSlices(current, ids) {
			continue
		}
		values := make([]any, len(ids))
		for index, id := range ids {
			values[index] = id
		}
		_ = scene.Set(field, values)
	}
}

// actorSceneID reads one actor's current scene id ("" when it has none).
func actorSceneID(actor *entity.Entity, scenePath string) string {
	sceneID, ok := actor.GetOrNone(scenePath).(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(sceneID)
}

// sceneOccupantIDs reads one scene's occupant list; a missing field yields nil
// and a non-list value yields an empty list (it carries no occupants).
func sceneOccupantIDs(scene *entity.Entity, field string) []string {
	raw, ok := scene.GetOrNone(field).([]any)
	if !ok || raw == nil {
		return nil
	}
	ids := make([]string, 0, len(raw))
	for _, item := range raw {
		if id, isString := item.(string); isString {
			ids = append(ids, id)
		}
	}
	return ids
}

// equalStringSlices reports whether two id lists carry the same ids in order
// (nil and an empty slice both mean "no occupants").
func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
