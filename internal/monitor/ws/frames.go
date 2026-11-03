// Frame vocabulary and snapshot builders for the /monitor/ws channel.
package ws

import (
	"sort"
	"strings"
	"time"

	"github.com/RuiCat/Neko/internal/diagnostics"
)

const (
	// TOPIC_STATE selects entity snapshots and delta frames.
	TOPIC_STATE = "state"
	// TOPIC_DIAGNOSTICS selects generation/summary/record frames.
	TOPIC_DIAGNOSTICS = "diagnostics"
	// TOPIC_LOGS selects log frames.
	TOPIC_LOGS = "logs"
	// TOPIC_TICK selects tick frames.
	TOPIC_TICK = "tick"
)

// DEFAULT_TOPICS is the subscription used when topics is absent.
var DEFAULT_TOPICS = []string{TOPIC_STATE, TOPIC_DIAGNOSTICS, TOPIC_LOGS, TOPIC_TICK}

// clientMessage is one browser-to-server frame.
type clientMessage struct {
	Type   string   `json:"type"`
	Topics []string `json:"topics"`
	Entity string   `json:"entity"`
	Paths  []string `json:"paths"`
}

// helloFrame announces the connection identity.
func helloFrame(seq uint64, version string, readOnly bool, now time.Time) map[string]any {
	return map[string]any{
		"type": "hello", "seq": seq, "version": version,
		"readOnly": readOnly, "serverTime": now.UTC().Format(time.RFC3339Nano),
	}
}

// snapshotFrame wraps one topic snapshot.
func snapshotFrame(seq uint64, topic string, data map[string]any) map[string]any {
	return map[string]any{"type": "snapshot", "seq": seq, "topic": topic, "data": data}
}

// parseTopics turns the topics query into a set; empty means all topics.
func parseTopics(raw string) map[string]bool {
	topics := map[string]bool{}
	for _, part := range splitList(raw) {
		switch part {
		case TOPIC_STATE, TOPIC_DIAGNOSTICS, TOPIC_LOGS, TOPIC_TICK:
			topics[part] = true
		}
	}
	if len(topics) == 0 {
		for _, topic := range DEFAULT_TOPICS {
			topics[topic] = true
		}
	}
	return topics
}

// orderedTopics returns the subscribed topics in a stable frame order.
func orderedTopics(topics map[string]bool) []string {
	out := make([]string, 0, len(topics))
	for _, topic := range DEFAULT_TOPICS {
		if topics[topic] {
			out = append(out, topic)
		}
	}
	return out
}

// splitList flattens comma-separated and repeated query values.
func splitList(values ...string) []string {
	out := make([]string, 0)
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

// stateSnapshot copies every entity through Snapshot, ordered by id.
func (h *Hub) stateSnapshot() map[string]any {
	entities := h.deps.Store.ListAll()
	sort.Slice(entities, func(i, j int) bool { return entities[i].ID < entities[j].ID })
	rows := make([]map[string]any, 0, len(entities))
	for _, ent := range entities {
		rows = append(rows, map[string]any{
			"id": ent.ID, "type": ent.Type, "data": ent.Snapshot(),
		})
	}
	return map[string]any{"entities": rows}
}

// diagnosticsSnapshot returns the bounded diagnostics view.
func (h *Hub) diagnosticsSnapshot() map[string]any {
	store := h.deps.Diagnostics
	filter := diagnostics.Filter{}
	return map[string]any{
		"generation": store.Generation(),
		"summary":    store.Summary(filter),
		"items":      store.Snapshot(filter, DIAGNOSTIC_SNAPSHOT_LIMIT),
	}
}

// tickSnapshot returns the latest tick payload, or nil before the first tick.
func (h *Hub) tickSnapshot() map[string]any {
	h.latestMu.RLock()
	defer h.latestMu.RUnlock()
	if !h.hasLatest {
		return map[string]any{"result": nil}
	}
	return map[string]any{"result": h.latest}
}

// snapshotFor builds the snapshot payload for one topic.
func (h *Hub) snapshotFor(topic string) map[string]any {
	switch topic {
	case TOPIC_STATE:
		return h.stateSnapshot()
	case TOPIC_DIAGNOSTICS:
		return h.diagnosticsSnapshot()
	case TOPIC_TICK:
		return h.tickSnapshot()
	default:
		return map[string]any{"lines": []any{}}
	}
}

// DIAGNOSTIC_SNAPSHOT_LIMIT caps the diagnostics snapshot item list.
const DIAGNOSTIC_SNAPSHOT_LIMIT = 100
