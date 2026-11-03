// Cross-context messaging with attribution and loop guards.
package driver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

const (
	// SEND_DEDUPE_WINDOW suppresses the same source→target→text send.
	SEND_DEDUPE_WINDOW = 60 * time.Second
	// SEND_WINDOW is the per-source quota window for cross-context sends.
	SEND_WINDOW = time.Minute
	// SEND_BURST caps cross-context sends per source per window.
	SEND_BURST = 10
)

// SendRequest is one cross-context message.
type SendRequest struct {
	// SourceContextID names the sending context when known.
	SourceContextID string `json:"sourceContextId,omitempty"`
	// TargetContextID names the receiving context.
	TargetContextID string `json:"targetContextId"`
	// Text is the message body.
	Text string `json:"text"`
	// Priority is normal (default) or urgent.
	Priority string `json:"priority,omitempty"`
}

// sendGuard enforces cross-context dedupe and per-source quota.
type sendGuard struct {
	mu        sync.Mutex
	seen      map[string]time.Time
	windows   map[string]*routeWindowState
	lastSweep time.Time
}

// newSendGuard builds an empty guard.
func newSendGuard() *sendGuard {
	return &sendGuard{seen: map[string]time.Time{}, windows: map[string]*routeWindowState{}}
}

// allow reports whether one send may proceed and records it when it may.
func (g *sendGuard) allow(source, target, text string) bool {
	now := time.Now()
	key := source + "\x00" + target + "\x00" + digestText(text)
	g.mu.Lock()
	defer g.mu.Unlock()
	if last, ok := g.seen[key]; ok && now.Sub(last) < SEND_DEDUPE_WINDOW {
		return false
	}
	state := g.windows[source]
	if state == nil {
		state = &routeWindowState{windowStart: now}
		g.windows[source] = state
	}
	if now.Sub(state.windowStart) >= SEND_WINDOW {
		state.windowStart = now
		state.count = 0
	}
	if state.count >= SEND_BURST {
		return false
	}
	state.count++
	g.seen[key] = now
	if now.Sub(g.lastSweep) > SEND_DEDUPE_WINDOW {
		for stale := range g.seen {
			if now.Sub(g.seen[stale]) >= SEND_DEDUPE_WINDOW {
				delete(g.seen, stale)
			}
		}
		g.lastSweep = now
	}
	return true
}

// digestText hashes one message body for the dedupe key.
func digestText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:8])
}

// Send validates a cross-context message, applies the loop guard and then
// invokes the target context.
func (d *Driver) Send(ctx context.Context, request SendRequest) (InvokeResult, error) {
	source := strings.TrimSpace(request.SourceContextID)
	target := strings.TrimSpace(request.TargetContextID)
	text := strings.TrimSpace(request.Text)
	if target == "" {
		return InvokeResult{}, &RequestError{Message: "target_context_id 不能为空"}
	}
	if text == "" {
		return InvokeResult{}, &RequestError{Message: "text 不能为空"}
	}
	if source != "" {
		if _, ok := d.specs[source]; !ok {
			return InvokeResult{}, &RequestError{Message: "未知来源上下文 " + source}
		}
		if source == target {
			return InvokeResult{}, &RequestError{Message: "不能把消息发给自己"}
		}
	}
	if !d.sendGuard.allow(source, target, text) {
		return InvokeResult{}, &RequestError{Message: "跨上下文发送重复或过于频繁，已拒绝"}
	}
	return d.Invoke(ctx, InvokeRequest{
		ContextID: target,
		Text:      text,
		Priority:  request.Priority,
		Source:    "context:" + source,
	})
}
