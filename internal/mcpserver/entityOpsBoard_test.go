// entityOpsBoard_test.go proves direct MCP state writes reach the event board,
// so scene/entity content edits show up in subscription pushes.
package mcpserver

import (
	"path/filepath"
	"testing"

	"github.com/RuiCat/Neko/internal/eventboard"
	"github.com/RuiCat/Neko/internal/hardstate"
)

// TestEntityStateSetRecordsBoard covers a scene-entity content write through
// the real MCP tool: the board records from/to with a per-entry version.
func TestEntityStateSetRecordsBoard(t *testing.T) {
	fixture := newTickFixture(t)
	registry := eventboard.New(filepath.Join(fixture.dataDir, eventboard.FILE_NAME))
	fixture.server.SetEventBoards(registry)
	session, cancel := connect(t, fixture.server)
	defer cancel()

	callToolJSON(t, session, "entity_state_set", map[string]any{
		"entity": "猫猫", "values": map[string]any{"状态.情绪": "开心"},
	})
	board := registry.Board("猫猫")
	if board == nil || board.Version != 1 || len(board.Entries) != 1 {
		t.Fatalf("board = %+v", board)
	}
	entry := board.Entries[0]
	if entry.Path != "状态.情绪" || entry.To != "开心" || entry.Version != 1 {
		t.Fatalf("entry = %+v", entry)
	}
	if board.TickID != "direct:"+hardstate.SOURCE_MCP {
		t.Fatalf("tick_id = %q", board.TickID)
	}

	// 同值重写不推进版本，避免订阅推送重复内容。
	callToolJSON(t, session, "entity_state_set", map[string]any{
		"entity": "猫猫", "values": map[string]any{"状态.情绪": "开心"},
	})
	if board := registry.Board("猫猫"); board.Version != 1 || board.Entries[0].Version != 1 {
		t.Fatalf("同值写入推进了版本: %+v", board)
	}
}
