// Driver opens character contexts on DSH and routes Neko events into them.
package driver

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Prompt delivery modes supported by session/prompt.
const (
	// MODE_QUEUE waits for the running turn to finish.
	MODE_QUEUE = "queue"
	// MODE_STEER inserts the message at the nearest step boundary.
	MODE_STEER = "steer"
)

// Event priorities accepted by Invoke.
const (
	// PRIORITY_NORMAL queues behind the running turn.
	PRIORITY_NORMAL = "normal"
	// PRIORITY_URGENT steers a running turn.
	PRIORITY_URGENT = "urgent"
)

// requestSeq disambiguates auto-generated event ids inside one process.
var requestSeq atomic.Uint64

// RequestError marks a caller mistake such as an unknown context or empty text.
type RequestError struct {
	// Message is the human-readable reason.
	Message string
}

// Error renders the caller mistake.
func (e *RequestError) Error() string { return e.Message }

// InvokeRequest is one Neko event delivered to a character context.
type InvokeRequest struct {
	// ContextID selects the target context.
	ContextID string `json:"contextId"`
	// EventID is the idempotency key; empty generates one per call.
	EventID string `json:"eventId,omitempty"`
	// Text is the event card delivered as the prompt body.
	Text string `json:"text"`
	// Priority is normal (default) or urgent.
	Priority string `json:"priority,omitempty"`
	// Source labels the event origin, for example tick or context:猫猫.
	Source string `json:"source,omitempty"`
}

// InvokeResult reports the accepted prompt.
type InvokeResult struct {
	// ContextID is the target context.
	ContextID string `json:"contextId"`
	// SessionID is the DSH session that received the prompt.
	SessionID string `json:"sessionId"`
	// RequestID is the idempotency key sent to DSH.
	RequestID string `json:"requestId"`
	// Mode is queue or steer.
	Mode string `json:"mode"`
	// Status is the registry status after admission.
	Status string `json:"status"`
}

// Driver owns the DSH client, the context specs and the runtime registry.
type Driver struct {
	client    *Client
	registry  *Registry
	specs     map[string]Spec
	routes    map[string][]Route
	window    *routeWindow
	sendGuard *sendGuard
	routeSem  chan struct{}
	mu        sync.Mutex
	locks     map[string]*sync.Mutex
	opened    map[string]bool
	onError   func(error)
}

// New builds a driver from a validated config.
func New(cfg Config) (*Driver, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	registry, err := LoadRegistry(cfg.RegistryPath, cfg.Contexts)
	if err != nil {
		return nil, err
	}
	specs := make(map[string]Spec, len(cfg.Contexts))
	for _, spec := range cfg.Contexts {
		specs[spec.ContextID] = spec
	}
	routes := make(map[string][]Route, len(cfg.Routes))
	for _, route := range cfg.Routes {
		routes[route.Event] = append(routes[route.Event], route)
	}
	return &Driver{
		client:    NewClient(cfg.DshURL, cfg.DshToken),
		registry:  registry,
		specs:     specs,
		routes:    routes,
		window:    newRouteWindow(),
		sendGuard: newSendGuard(),
		routeSem:  make(chan struct{}, ROUTE_MAX_CONCURRENT),
		locks:     map[string]*sync.Mutex{},
		opened:    map[string]bool{},
	}, nil
}

// SetErrorHandler receives event-driven delivery failures; nil disables it.
func (d *Driver) SetErrorHandler(handler func(error)) {
	d.mu.Lock()
	d.onError = handler
	d.mu.Unlock()
}

// Snapshot returns every context record ordered by context id.
func (d *Driver) Snapshot() []Record { return d.registry.List() }

// lockFor returns the per-context serialization lock.
func (d *Driver) lockFor(contextID string) *sync.Mutex {
	d.mu.Lock()
	defer d.mu.Unlock()
	lock, ok := d.locks[contextID]
	if !ok {
		lock = &sync.Mutex{}
		d.locks[contextID] = lock
	}
	return lock
}

// isOpened reports whether this process already created the session.
func (d *Driver) isOpened(contextID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.opened[contextID]
}

// OpenAll opens every declared context and returns the per-context failures.
func (d *Driver) OpenAll(ctx context.Context) map[string]error {
	failures := map[string]error{}
	for _, spec := range d.specs {
		if _, err := d.Open(ctx, spec.ContextID); err != nil {
			failures[spec.ContextID] = err
		}
	}
	return failures
}

// Open creates or resumes one context's DSH session and adopts the session id
// DSH actually returned.
func (d *Driver) Open(ctx context.Context, contextID string) (Record, error) {
	if _, ok := d.specs[contextID]; !ok {
		return Record{}, &RequestError{Message: fmt.Sprintf("未知上下文 %q", contextID)}
	}
	lock := d.lockFor(contextID)
	lock.Lock()
	defer lock.Unlock()
	return d.openLocked(ctx, contextID)
}

// openLocked performs Open under the per-context lock. A context that already
// has a session id is operated in place: DSH treats session/create on a started
// session as "keep it and create a new one", so calling create again would
// silently fork a fresh conversation. Only a context without a session calls
// create; prompt/cancel resume the existing session.
func (d *Driver) openLocked(ctx context.Context, contextID string) (Record, error) {
	spec := d.specs[contextID]
	record, ok := d.registry.Get(contextID)
	if !ok {
		return Record{}, &RequestError{Message: fmt.Sprintf("未知上下文 %q", contextID)}
	}
	firstOpen := !d.isOpened(contextID)
	createdSessionID := ""
	if record.SessionID == "" {
		value, err := d.client.CreateSession(ctx, CreateRequest{
			Cwd: spec.Cwd, AgentPreset: spec.PresetID, SessionID: spec.SessionID,
		})
		if err != nil {
			return Record{}, err
		}
		createdSessionID = value.SessionID
	}
	updated, updateErr := d.registry.Update(contextID, func(record *Record) {
		if record.SessionID == "" && createdSessionID != "" {
			record.SessionID = createdSessionID
		}
		if firstOpen || record.Status == "" || record.Status == STATUS_CLOSED {
			record.Status = STATUS_IDLE
		}
		if record.OpenedAt.IsZero() {
			record.OpenedAt = time.Now()
		}
	})
	if updateErr != nil {
		return updated, updateErr
	}
	d.mu.Lock()
	d.opened[contextID] = true
	d.mu.Unlock()
	return updated, nil
}

// Invoke delivers one event into a context. A running session queues normal
// input and steers urgent input; an idle or closed context starts a turn.
func (d *Driver) Invoke(ctx context.Context, request InvokeRequest) (InvokeResult, error) {
	request.ContextID = strings.TrimSpace(request.ContextID)
	request.Text = strings.TrimSpace(request.Text)
	if request.ContextID == "" {
		return InvokeResult{}, &RequestError{Message: "context_id 不能为空"}
	}
	if request.Text == "" {
		return InvokeResult{}, &RequestError{Message: "text 不能为空"}
	}
	if _, ok := d.specs[request.ContextID]; !ok {
		return InvokeResult{}, &RequestError{Message: fmt.Sprintf("未知上下文 %q", request.ContextID)}
	}
	priority := strings.TrimSpace(request.Priority)
	if priority == "" {
		priority = PRIORITY_NORMAL
	}
	if priority != PRIORITY_NORMAL && priority != PRIORITY_URGENT {
		return InvokeResult{}, &RequestError{Message: "priority 只能是 normal 或 urgent"}
	}

	lock := d.lockFor(request.ContextID)
	lock.Lock()
	defer lock.Unlock()

	record, ok := d.registry.Get(request.ContextID)
	if !ok {
		return InvokeResult{}, &RequestError{Message: fmt.Sprintf("未知上下文 %q", request.ContextID)}
	}
	if !d.isOpened(request.ContextID) || record.SessionID == "" || record.Status == STATUS_CLOSED {
		opened, err := d.openLocked(ctx, request.ContextID)
		if err != nil {
			return InvokeResult{}, err
		}
		record = opened
	}
	mode := MODE_QUEUE
	if record.Status == STATUS_RUNNING && priority == PRIORITY_URGENT {
		mode = MODE_STEER
	}
	requestID := buildRequestID(request.ContextID, request.EventID)
	err := d.client.Prompt(ctx, PromptRequest{
		RequestID: requestID,
		SessionID: record.SessionID,
		Mode:      mode,
		Content:   []ContentPart{{Type: "text", Text: request.Text}},
	})
	if err != nil {
		return InvokeResult{}, err
	}
	updated, err := d.registry.Update(request.ContextID, func(record *Record) {
		record.Status = STATUS_RUNNING
		record.LastRequestID = requestID
	})
	if err != nil {
		return InvokeResult{}, err
	}
	return InvokeResult{
		ContextID: request.ContextID,
		SessionID: updated.SessionID,
		RequestID: requestID,
		Mode:      mode,
		Status:    updated.Status,
	}, nil
}

// HandleTurnEnd marks the context idle after a finished DSH turn.
func (d *Driver) HandleTurnEnd(sessionID string, turn int) error {
	record, ok := d.registry.FindBySession(sessionID)
	if !ok {
		return nil
	}
	lock := d.lockFor(record.ContextID)
	lock.Lock()
	defer lock.Unlock()
	_, err := d.registry.Update(record.ContextID, func(record *Record) {
		record.Status = STATUS_IDLE
		record.LastTurn = turn
	})
	return err
}

// ContextForSession returns the context that owns a DSH session id.
func (d *Driver) ContextForSession(sessionID string) (Record, bool) {
	return d.registry.FindBySession(sessionID)
}

// buildRequestID builds the idempotency key for one event.
func buildRequestID(contextID, eventID string) string {
	event := strings.TrimSpace(eventID)
	if event == "" {
		event = fmt.Sprintf("%d-%d", time.Now().UnixNano(), requestSeq.Add(1))
	}
	return "neko:" + contextID + ":" + event
}
