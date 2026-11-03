// Tick execution shared by the MCP tool, the control plane and the monitor API.
package mcpserver

import (
	"log"
	"time"

	"github.com/RuiCat/Neko/internal/engine"
)

const (
	// OBSERVER_SLOW_THRESHOLD warns about an observer slower than this; it
	// never delays the tick itself.
	OBSERVER_SLOW_THRESHOLD = 100 * time.Millisecond
	// STATE_GATE_POLL_INTERVAL is the bounded-wait polling step for writes.
	STATE_GATE_POLL_INTERVAL = 10 * time.Millisecond
)

// TickOptions controls side effects of one tick run.
type TickOptions struct {
	// SkipSaveWhenUnchanged skips the store save when the tick produced neither
	// transitions nor conflicts. MCP state_machine_tick leaves it false so its
	// historical "always save" behavior is preserved.
	SkipSaveWhenUnchanged bool
	// SkipJournal suppresses the per-tick TurnRecord. The chat loop sets it
	// because it writes one complete record (input + answer) for the turn.
	SkipJournal bool
}

// RunTick executes one state-machine tick, blocking until the shared gate is
// free. scope is normalized to "*" when empty, matching the MCP tool. All side
// effects of the tick (diagnostics, state log, optional save) happen under
// tickMu so only one tick is ever in flight per process. The tick observer is
// told after tickMu is released.
func (s *Server) RunTick(scope string, options TickOptions) (engine.TickResult, error) {
	s.tickMu.Lock()
	result, err := s.runTickLocked(scope, options)
	s.tickMu.Unlock()
	s.notifyTick(result)
	return result, err
}

// TryRunTick runs one tick without queueing: when another tick or state write
// holds the gate it reports busy immediately and returns a zero TickResult with
// a nil error. On success it returns RunTick's result and error plus busy=true.
func (s *Server) TryRunTick(scope string, options TickOptions) (engine.TickResult, bool, error) {
	if !s.tickMu.TryLock() {
		return engine.TickResult{}, false, nil
	}
	result, err := s.runTickLocked(scope, options)
	s.tickMu.Unlock()
	s.notifyTick(result)
	return result, true, err
}

// runTickLocked is the tick body shared by RunTick and TryRunTick. Callers must
// hold tickMu.
func (s *Server) runTickLocked(scope string, options TickOptions) (engine.TickResult, error) {
	if scope == "" {
		scope = "*"
	}
	result := s.machine.Tick(scope)
	s.recordTick(result)
	if s.logger != nil {
		_ = s.logger.Log(result, scope, s.snapshotStates(result))
	}
	if !options.SkipJournal {
		s.appendTurnRecord(result)
	}
	if options.SkipSaveWhenUnchanged && len(result.Transitions) == 0 && len(result.Conflicts) == 0 {
		return result, nil
	}
	if err := s.store.Save(); err != nil {
		return result, err
	}
	return result, nil
}

// notifyTick delivers one finished tick outside tickMu. Delivery is
// fire-and-forget: a slow or panicking observer never delays or breaks the
// tick, it only produces a stderr warning.
func (s *Server) notifyTick(result engine.TickResult) {
	s.tickObserverMu.RLock()
	observer := s.tickObserver
	s.tickObserverMu.RUnlock()
	if observer == nil {
		return
	}
	go func() {
		started := time.Now()
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("mcpserver: tick 观察者 panic: %v", recovered)
			}
			if elapsed := time.Since(started); elapsed > OBSERVER_SLOW_THRESHOLD {
				log.Printf("mcpserver: tick 观察者阻塞 %s（已异步投递，不影响 tick）", elapsed)
			}
		}()
		observer(result)
	}()
}

// WithStateWrite runs one state mutation under the same gate as RunTick, so
// control-plane writes (for example /session-event effects plus their save)
// serialize with in-flight ticks. It returns the callback error unchanged.
// The gate is not reentrant: the callback must not call RunTick or
// WithStateWrite, which would deadlock.
func (s *Server) WithStateWrite(run func() error) error {
	s.tickMu.Lock()
	defer s.tickMu.Unlock()
	return run()
}

// WithStateWriteTimeout acquires the state gate with a bounded wait. It returns
// acquired=false when the gate stays busy past timeout; the callback never runs
// in that case, so a timed-out write leaves no side effect behind.
func (s *Server) WithStateWriteTimeout(timeout time.Duration, run func() error) (bool, error) {
	deadline := time.Now().Add(timeout)
	for {
		if s.tickMu.TryLock() {
			defer s.tickMu.Unlock()
			return true, run()
		}
		if !time.Now().Before(deadline) {
			return false, nil
		}
		time.Sleep(STATE_GATE_POLL_INTERVAL)
	}
}

// TickPayload builds the single K1 tick payload used by the monitor REST tick,
// the monitor tick/latest endpoint and the WebSocket tick frame. Its field set
// must stay equal to the control-plane /tick payload (asserted by tests).
func TickPayload(result engine.TickResult) map[string]any {
	return map[string]any{
		"tickId":          result.TickID,
		"tick_id":         result.TickID,
		"status":          result.Status,
		"transitions":     nonNilSlice(result.Transitions),
		"conflicts":       nonNilSlice(result.Conflicts),
		"errors":          nonNilSlice(result.Errors),
		"hint":            result.ReplyHint,
		"rules_evaluated": result.RulesEvaluated,
	}
}

// nonNilSlice keeps JSON arrays as [] instead of null when nothing was produced.
func nonNilSlice[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}
