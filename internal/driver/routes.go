// Event routing: .neko event dispatches become prompts for character contexts.
package driver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// ROUTE_TIMEOUT bounds one event-driven invoke.
	ROUTE_TIMEOUT = 15 * time.Second
	// ROUTE_MIN_INTERVAL suppresses a second routed invoke for one context.
	ROUTE_MIN_INTERVAL = 2 * time.Second
	// ROUTE_WINDOW is the quota window for routed invokes.
	ROUTE_WINDOW = time.Minute
	// ROUTE_BURST caps routed invokes per context per window.
	ROUTE_BURST = 10
	// DEFAULT_ROUTE_TEMPLATE renders a routed event into prompt text.
	DEFAULT_ROUTE_TEMPLATE = "【neko 事件】event={{event}} entity={{entity}} field={{field}} old={{old}} new={{new}}"
	// MAX_ROUTE_VALUE_CHARS truncates one rendered placeholder value.
	MAX_ROUTE_VALUE_CHARS = 200
	// ROUTE_MAX_CONCURRENT bounds event-driven invokes in flight at once so a
	// burst of events cannot spawn unbounded goroutines.
	ROUTE_MAX_CONCURRENT = 32
)

// routeSeq disambiguates generated route event ids.
var routeSeq atomic.Uint64

// Event is one .neko event dispatch handed to the routing table.
type Event struct {
	// Name is the @事件 name that fired.
	Name string
	// Entity is the entity whose field changed.
	Entity string
	// Field is the dotted field path that changed.
	Field string
	// Old and New are the field values around the change.
	Old any
	New any
	// Delta is the change the bus passed to the handler.
	Delta any
	// Records are the handler execution lines the bus produced.
	Records []string
}

// DispatchReport reports what one event routed.
type DispatchReport struct {
	// Dispatched counts the invokes started.
	Dispatched int
	// Skipped lists context ids suppressed by the cooldown or quota.
	Skipped []string
}

// routeWindow enforces the per-context cooldown and quota.
type routeWindow struct {
	mu     sync.Mutex
	states map[string]*routeWindowState
}

// routeWindowState is one context's cooldown and quota counters.
type routeWindowState struct {
	lastAt      time.Time
	windowStart time.Time
	count       int
}

// newRouteWindow builds an empty window table.
func newRouteWindow() *routeWindow {
	return &routeWindow{states: map[string]*routeWindowState{}}
}

// allow reports whether one routed invoke may start for the context.
func (w *routeWindow) allow(contextID string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now()
	state := w.states[contextID]
	if state == nil {
		state = &routeWindowState{windowStart: now}
		w.states[contextID] = state
	}
	if now.Sub(state.lastAt) < ROUTE_MIN_INTERVAL {
		return false
	}
	if now.Sub(state.windowStart) >= ROUTE_WINDOW {
		state.windowStart = now
		state.count = 0
	}
	if state.count >= ROUTE_BURST {
		return false
	}
	state.lastAt = now
	state.count++
	return true
}

// OnEvent starts the invokes declared for one event and reports skips.
func (d *Driver) OnEvent(event Event) DispatchReport {
	report := DispatchReport{}
	for _, route := range d.routes[event.Name] {
		if !d.window.allow(route.ContextID) {
			report.Skipped = append(report.Skipped, route.ContextID)
			continue
		}
		text := renderRouteText(route.Template, event)
		select {
		case d.routeSem <- struct{}{}:
			report.Dispatched++
			go func() {
				defer func() { <-d.routeSem }()
				d.dispatchRoute(route, event.Name, text)
			}()
		default:
			report.Skipped = append(report.Skipped, route.ContextID)
		}
	}
	return report
}

// dispatchRoute delivers one routed event without blocking the event bus.
func (d *Driver) dispatchRoute(route Route, eventName, text string) {
	ctx, cancel := context.WithTimeout(context.Background(), ROUTE_TIMEOUT)
	defer cancel()
	eventID := fmt.Sprintf("route-%s-%d", eventName, routeSeq.Add(1))
	_, err := d.Invoke(ctx, InvokeRequest{
		ContextID: route.ContextID,
		EventID:   eventID,
		Text:      text,
		Priority:  route.Priority,
		Source:    "event:" + eventName,
	})
	if err != nil {
		d.reportError(err)
	}
}

// renderRouteText fills the route template; empty templates use the default.
func renderRouteText(template string, event Event) string {
	if strings.TrimSpace(template) == "" {
		template = DEFAULT_ROUTE_TEMPLATE
	}
	replacer := strings.NewReplacer(
		"{{event}}", event.Name,
		"{{entity}}", event.Entity,
		"{{field}}", event.Field,
		"{{old}}", formatRouteValue(event.Old),
		"{{new}}", formatRouteValue(event.New),
		"{{delta}}", formatRouteValue(event.Delta),
	)
	return replacer.Replace(template)
}

// formatRouteValue renders one placeholder value as bounded text.
func formatRouteValue(value any) string {
	if value == nil {
		return ""
	}
	var text string
	switch typed := value.(type) {
	case string:
		text = typed
	case bool, float64, int, int64:
		text = fmt.Sprint(typed)
	default:
		if raw, err := json.Marshal(typed); err == nil {
			text = string(raw)
		} else {
			text = fmt.Sprint(typed)
		}
	}
	if len([]rune(text)) > MAX_ROUTE_VALUE_CHARS {
		runes := []rune(text)
		text = string(runes[:MAX_ROUTE_VALUE_CHARS]) + "…"
	}
	return text
}

// reportError forwards an event-driven delivery failure without blocking callers.
func (d *Driver) reportError(err error) {
	d.mu.Lock()
	handler := d.onError
	d.mu.Unlock()
	if handler != nil {
		handler(err)
	}
}
