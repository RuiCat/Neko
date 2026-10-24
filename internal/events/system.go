package events

import (
	"github.com/ruicat/astrbox/internal/entity"
	"github.com/ruicat/astrbox/internal/neko"
)

// FuncRegistry is the function-executor subset managed by the system.
type FuncRegistry interface {
	Caller
	Register(def neko.FuncDef) error
	Unregister(name string)
}

// System owns the event bus, the schema bridge and the function registry.
type System struct {
	store  *entity.Store
	funcs  FuncRegistry
	bus    *Bus
	bridge *Bridge
}

// NewSystem wires the bus and bridge around a function registry.
func NewSystem(store *entity.Store, funcs FuncRegistry) *System {
	bus := NewBus(store, funcs)
	bridge := NewBridge(store, bus)
	store.OnCreate = func(ent *entity.Entity) { bridge.Attach(ent) }
	return &System{store: store, funcs: funcs, bus: bus, bridge: bridge}
}

// RegisterFuncs adds @函数 definitions to the executor.
func (s *System) RegisterFuncs(defs []neko.FuncDef) {
	if s.funcs == nil {
		return
	}
	for _, def := range defs {
		_ = s.funcs.Register(def)
	}
}

// UnregisterFuncs removes functions by name.
func (s *System) UnregisterFuncs(names []string) {
	if s.funcs == nil {
		return
	}
	for _, name := range names {
		s.funcs.Unregister(name)
	}
}

// RegisterEvents adds @事件 definitions to the bus.
func (s *System) RegisterEvents(defs []neko.EventDef) { s.bus.Register(defs) }

// UnregisterEvents removes events by name.
func (s *System) UnregisterEvents(names []string) { s.bus.Unregister(names) }

// Refresh rebuilds schemas and re-attaches entities after template changes.
func (s *System) Refresh() { s.bridge.Refresh() }

// Bus exposes the underlying event bus (for tests and inventory).
func (s *System) Bus() *Bus { return s.bus }

// Bridge exposes the change bridge (for tests and inventory).
func (s *System) Bridge() *Bridge { return s.bridge }
