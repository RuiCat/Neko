// Package ws implements the /monitor/ws realtime channel: one listener per
// process, per-connection sequence numbers and snapshot-on-every-connection.
package ws

import (
	"sync"

	"github.com/RuiCat/Neko/internal/diagnostics"
	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/entity"
	"github.com/RuiCat/Neko/internal/mcpserver"
)

// Deps is the read-only runtime surface the hub snapshots.
type Deps struct {
	Store       *entity.Store
	Diagnostics *diagnostics.Store
	ReadOnly    bool
	Version     string
}

// Hub fans engine changes out to every connected client. Registering a client
// never installs an entity listener; the process-wide listeners are wired once
// by monitor.New.
type Hub struct {
	deps      Deps
	mu        sync.RWMutex
	clients   map[*Client]struct{}
	latestMu  sync.RWMutex
	latest    map[string]any
	hasLatest bool
}

// NewHub builds an empty hub.
func NewHub(deps Deps) *Hub {
	return &Hub{deps: deps, clients: map[*Client]struct{}{}}
}

// Register adds one connection and queues its hello/snapshot frames while
// holding the fan-out lock, so no delta frame can be enqueued before the
// snapshot that covers it.
func (h *Hub) Register(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[client] = struct{}{}
	client.sendInitial()
}

// unregister removes one connection from the fan-out set.
func (h *Hub) unregister(client *Client) {
	h.mu.Lock()
	delete(h.clients, client)
	h.mu.Unlock()
}

// snapshotClients copies the fan-out set so publishing never holds the lock.
func (h *Hub) snapshotClients() []*Client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	clients := make([]*Client, 0, len(h.clients))
	for client := range h.clients {
		clients = append(clients, client)
	}
	return clients
}

// PublishTick records the latest tick payload (built by the single
// mcpserver.TickPayload constructor) and pushes one tick frame per client.
func (h *Hub) PublishTick(result engine.TickResult) {
	payload := mcpserver.TickPayload(result)
	h.latestMu.Lock()
	h.latest = payload
	h.hasLatest = true
	h.latestMu.Unlock()
	frame := map[string]any{"type": "tick", "result": payload}
	for _, client := range h.snapshotClients() {
		if client.accepts(TOPIC_TICK, "", "") {
			client.push(frame)
		}
	}
}

// PublishDelta pushes one entity field change; it is called from the
// process-wide entity change listener installed by monitor.New.
func (h *Hub) PublishDelta(entityID, path string, oldValue, newValue any) {
	frame := map[string]any{
		"type": "delta", "entity": entityID, "path": path,
		"oldValue": oldValue, "newValue": newValue,
	}
	for _, client := range h.snapshotClients() {
		if client.accepts(TOPIC_STATE, entityID, path) {
			client.push(frame)
		}
	}
}

// PublishDiagnostic pushes one added/updated/removed diagnostic notification;
// it is called from diagnostics.Store.SetNotify after the store lock is released.
func (h *Hub) PublishDiagnostic(record diagnostics.Record, op string) {
	frame := map[string]any{"type": "diagnostics", "op": op, "generation": record.Generation}
	if op == "removed" {
		frame["id"] = record.ID
	} else {
		frame["record"] = record
	}
	for _, client := range h.snapshotClients() {
		if client.accepts(TOPIC_DIAGNOSTICS, record.Entity, "") {
			client.push(frame)
		}
	}
}

// PublishLog pushes one log line to log subscribers.
func (h *Hub) PublishLog(kind, line string) {
	frame := map[string]any{"type": "log", "kind": kind, "line": line}
	for _, client := range h.snapshotClients() {
		if client.accepts(TOPIC_LOGS, "", "") {
			client.push(frame)
		}
	}
}

// LatestTickPayload returns the most recent tick payload for tick/latest.
func (h *Hub) LatestTickPayload() (map[string]any, bool) {
	h.latestMu.RLock()
	defer h.latestMu.RUnlock()
	return h.latest, h.hasLatest
}

// CloseAll force-closes every live connection during shutdown.
func (h *Hub) CloseAll() {
	for _, client := range h.snapshotClients() {
		client.closeNow()
	}
}
