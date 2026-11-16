// publish.go forwards script push messages to the session board: the engine
// writes 交互.推送.消息, and subscribed sessions receive it as a board delta.
package mcpserver

import (
	"strings"

	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/observation"
)

// pushMessagePath is the field the script push rule writes the message to.
const pushMessagePath = "交互.推送.消息"

// SetBoardPublisher wires the session-board publisher. Without it the tick
// behaves exactly as before: script messages stay in the observation stream.
func (s *Server) SetBoardPublisher(publish func(entityID, text string)) {
	s.boardPublisherMu.Lock()
	s.boardPublisher = publish
	s.boardPublisherMu.Unlock()
}

// publishTick forwards each changed push message of a finished tick. The board
// write never fails a tick: a broken board only loses that message.
func (s *Server) publishTick(result engine.TickResult) {
	s.boardPublisherMu.RLock()
	publish := s.boardPublisher
	s.boardPublisherMu.RUnlock()
	if publish == nil {
		return
	}
	for _, group := range observation.ChangesFromWrites(result.Writes) {
		for _, change := range group.Changes {
			if change.Path != pushMessagePath {
				continue
			}
			text, ok := change.To.(string)
			if !ok || strings.TrimSpace(text) == "" {
				continue
			}
			publish(group.Entity, text)
		}
	}
}
