// publish_test.go covers the tick hook that forwards script push messages to
// the session board.
package mcpserver

import (
	"testing"

	"github.com/RuiCat/Neko/internal/engine"
)

// TestPublishTickForwardsPushMessages proves only the push path is forwarded.
func TestPublishTickForwardsPushMessages(t *testing.T) {
	server := &Server{}
	type forwarded struct{ entity, text string }
	var got []forwarded
	server.SetBoardPublisher(func(entityID, text string) {
		got = append(got, forwarded{entityID, text})
	})
	server.publishTick(engine.TickResult{Writes: []engine.EntityWrite{
		{EntityID: "猫猫", Path: "交互.推送.消息", To: "猫猫想主人了"},
		{EntityID: "猫猫", Path: "交互.推送.版本", To: 1},
		{EntityID: "狗", Path: "交互.推送.消息", To: "  "},
	}})
	if len(got) != 1 || got[0].entity != "猫猫" || got[0].text != "猫猫想主人了" {
		t.Fatalf("forwarded = %+v", got)
	}
}

// TestPublishTickWithoutPublisher proves the hook is optional.
func TestPublishTickWithoutPublisher(t *testing.T) {
	server := &Server{}
	server.publishTick(engine.TickResult{})
}
