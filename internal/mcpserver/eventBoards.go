// eventBoards.go wires the per-entity change board into the tick pipeline (K2).
package mcpserver

import (
	"log"

	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/eventboard"
)

// SetEventBoards wires the shared per-entity change boards. Without it the tick
// behaves exactly as before, with no board side effect.
func (s *Server) SetEventBoards(boards *eventboard.Registry) { s.eventBoards = boards }

// applyEventBoards records this tick's transitions on the entity boards and
// lets the registry persist them with the tick. A board failure is logged and
// never fails the tick (K2): a broken data directory cannot stop the machine.
func (s *Server) applyEventBoards(result engine.TickResult) {
	if s.eventBoards == nil {
		return
	}
	if err := s.eventBoards.Apply(result); err != nil {
		log.Printf("mcpserver: 事件黑板写入失败（tick 照常）: %v", err)
	}
}
