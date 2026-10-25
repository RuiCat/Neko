// Tick execution shared by the MCP tool and the control plane.
package mcpserver

import "github.com/ruicat/astrbox/internal/engine"

// TickOptions controls side effects of one tick run.
type TickOptions struct {
	// SkipSaveWhenUnchanged skips the store save when the tick produced neither
	// transitions nor conflicts. MCP state_machine_tick leaves it false so its
	// historical "always save" behavior is preserved.
	SkipSaveWhenUnchanged bool
}

// RunTick executes one state-machine tick, blocking until the shared gate is
// free. scope is normalized to "*" when empty, matching the MCP tool. All side
// effects of the tick (diagnostics, state log, optional save) happen under
// tickMu so only one tick is ever in flight per process.
func (s *Server) RunTick(scope string, options TickOptions) (engine.TickResult, error) {
	s.tickMu.Lock()
	defer s.tickMu.Unlock()
	return s.runTickLocked(scope, options)
}

// TryRunTick runs one tick without queueing: when another tick or state write
// holds the gate it reports busy immediately and returns a zero TickResult with
// a nil error. On success it returns RunTick's result and error plus busy=true.
func (s *Server) TryRunTick(scope string, options TickOptions) (engine.TickResult, bool, error) {
	if !s.tickMu.TryLock() {
		return engine.TickResult{}, false, nil
	}
	defer s.tickMu.Unlock()
	result, err := s.runTickLocked(scope, options)
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
	if options.SkipSaveWhenUnchanged && len(result.Transitions) == 0 && len(result.Conflicts) == 0 {
		return result, nil
	}
	if err := s.store.Save(); err != nil {
		return result, err
	}
	return result, nil
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
