// sceneEventProjection.go folds a scene's event-stream change (an unbounded
// list) into one bounded board change carrying only the newest entry.
//
// The board stores from -> to per path, so projecting the stream onto
// `<event field>.最新` keeps the board readable and the push small no matter how
// long the scene's event list grows.
package eventboard

import (
	"encoding/json"
	"strings"

	"github.com/RuiCat/Neko/internal/config"
)

const (
	// sceneEventPreviewSuffix names the projected path: `事件.最新`.
	sceneEventPreviewSuffix = ".最新"
	// sceneEventPreviewMaxRunes bounds one previewed event.
	sceneEventPreviewMaxRunes = 120
)

// projectSceneEventChanges rewrites event-stream changes into their bounded
// projection and drops changes whose lists carry nothing to preview.
func projectSceneEventChanges(changes []Change) []Change {
	eventField := config.Current().SceneEventField
	if eventField == "" || len(changes) == 0 {
		return changes
	}
	projected := make([]Change, 0, len(changes))
	for _, change := range changes {
		if change.Path != eventField {
			projected = append(projected, change)
			continue
		}
		if next, ok := projectSceneEventChange(change); ok {
			projected = append(projected, next)
		}
	}
	return projected
}

// projectSceneEventChange turns one event-stream change into `事件.最新`.
// FromKnown is false when the old list was empty, so the board reuses that
// path's previous To as from (a new event still shows old -> new).
func projectSceneEventChange(change Change) (Change, bool) {
	from, fromOK := newestSceneEvent(change.From)
	to, toOK := newestSceneEvent(change.To)
	if !fromOK && !toOK {
		return Change{}, false
	}
	return Change{
		Path:      change.Path + sceneEventPreviewSuffix,
		From:      from,
		To:        to,
		RuleIDs:   change.RuleIDs,
		FromKnown: fromOK,
	}, true
}

// newestSceneEvent previews the last entry of an event-list value.
func newestSceneEvent(value any) (any, bool) {
	events, ok := value.([]any)
	if !ok || len(events) == 0 {
		return nil, false
	}
	return sceneEventPreview(events[len(events)-1]), true
}

// sceneEventPreview renders one event value as a bounded preview: strings stay
// readable, structured values fall back to their JSON text.
func sceneEventPreview(value any) any {
	if value == nil {
		return nil
	}
	if text, ok := value.(string); ok {
		return truncateScenePreview(text)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return truncateScenePreview(string(raw))
}

// truncateScenePreview bounds a preview to sceneEventPreviewMaxRunes runes.
func truncateScenePreview(text string) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= sceneEventPreviewMaxRunes {
		return string(runes)
	}
	return string(runes[:sceneEventPreviewMaxRunes]) + "…"
}
