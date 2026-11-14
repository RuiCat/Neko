// eventKind_test.go pins the tick-outcome → gate event-vocabulary mapping.
package mcpserver

import (
	"testing"

	"github.com/RuiCat/Neko/internal/engine"
)

// transition builds one tick transition that wrote the given state paths.
func transition(paths ...string) engine.Transition {
	operations := make([]map[string]any, 0, len(paths))
	for _, path := range paths {
		operations = append(operations, map[string]any{"op": "set", "path": path, "value": 1})
	}
	return engine.Transition{Operations: operations}
}

// TestEventKindOfMapsTickTransitions covers every mapped state group.
func TestEventKindOfMapsTickTransitions(t *testing.T) {
	cases := []struct {
		name   string
		result engine.TickResult
		want   string
	}{
		{"冲突优先", engine.TickResult{Conflicts: []engine.Conflict{{ConflictID: "c1"}}}, "conflict"},
		{"关系状态", engine.TickResult{Transitions: []engine.Transition{transition("关系.亲密")}}, "relationship"},
		{"情绪状态", engine.TickResult{Transitions: []engine.Transition{transition("状态.情绪")}}, "emotion"},
		{"心情状态", engine.TickResult{Transitions: []engine.Transition{transition("心情.当前")}}, "emotion"},
		{"睡眠状态", engine.TickResult{Transitions: []engine.Transition{transition("睡眠.疲劳")}}, "self"},
		{"身体状态", engine.TickResult{Transitions: []engine.Transition{transition("身体.体力")}}, "self"},
		{"优先级", engine.TickResult{Transitions: []engine.Transition{transition("睡眠.疲劳", "关系.亲密")}}, "relationship"},
		{"无信息", engine.TickResult{}, "turn"},
		{"无 path 操作", engine.TickResult{Transitions: []engine.Transition{{Operations: []map[string]any{{"op": "append"}}}}}, "turn"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := eventKindOf(testCase.result); got != testCase.want {
				t.Fatalf("eventKindOf = %q, want %q", got, testCase.want)
			}
		})
	}
}
