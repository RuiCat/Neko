// observations.go wires the observation bus into the tick pipeline and the
// direct-write path; it replaces the retired per-entity event boards.
package mcpserver

import (
	"time"

	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/observation"
)

// SetObservations wires the shared observation bus and its policy. Without it
// the tick behaves exactly as before, with no observation side effect.
func (s *Server) SetObservations(bus *observation.Bus, policy *observation.Policy) {
	s.observations = bus
	s.observationPolicy = policy
}

// observeTick records one finished tick's net changes on the observation bus.
// The bus never fails a tick: a broken observation stream only loses detail.
func (s *Server) observeTick(result engine.TickResult) {
	if s.observations == nil {
		return
	}
	at := time.Now()
	for _, group := range observation.ChangesFromWrites(result.Writes) {
		s.observations.AppendAll(observation.EventsForEntity(group.Entity, group.Changes, result.TickID, at))
	}
}

// observeDirect records writes that bypass the tick (MCP tools and the control
// plane), so subscription pushes see them too.
func (s *Server) observeDirect(entityID string, changes []observation.Change, source string) {
	if s.observations == nil || len(changes) == 0 {
		return
	}
	s.observations.AppendAll(observation.EventsForEntity(entityID, changes, "direct:"+source, time.Now()))
}

// seedObservations records one entity's current non-noise state as the
// baseline of its stream, so a state card has content before the first change.
func (s *Server) seedObservations(entityID string, state map[string]any) {
	if s.observations == nil || entityID == "" {
		return
	}
	observation.SeedEntity(s.observations, s.ObservationPolicy(), entityID, state)
}

// Observations returns the observation bus, or nil when the server was built
// without one (tests and embedders).
func (s *Server) Observations() *observation.Bus { return s.observations }

// ObservationPolicy returns the active policy, falling back to the built-in one
// so read paths never have to guard against nil.
func (s *Server) ObservationPolicy() *observation.Policy {
	if s.observationPolicy == nil {
		return observation.DefaultPolicy()
	}
	return s.observationPolicy
}
