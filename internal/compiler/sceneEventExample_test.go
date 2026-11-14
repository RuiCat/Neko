// sceneEventExample_test.go compiles the shipped scene-event example so the
// documented rule keeps producing the engine op the board projection expects.
package compiler

import (
	"os"
	"strings"
	"testing"

	"github.com/RuiCat/Neko/internal/neko"
)

// TestSceneEventExampleCompiles keeps docs/examples/scene_events.neko honest:
// `场景.事件 += 状态.触发` must compile to append(场景.事件) with the trigger as
// a value reference, which the engine resolves to the actor's current scene.
func TestSceneEventExampleCompiles(t *testing.T) {
	raw, err := os.ReadFile("../../docs/examples/scene_events.neko")
	if err != nil {
		t.Fatalf("read example: %v", err)
	}
	ast := neko.Parse(string(raw))
	if len(ast.Errors) > 0 {
		t.Fatalf("parse errors: %v", ast.Errors)
	}
	result := CompileScript(ast)
	if len(result.Errors) > 0 {
		t.Fatalf("compile errors: %v", result.Errors)
	}
	if len(result.Rules) != 1 {
		t.Fatalf("rules = %d, want 1", len(result.Rules))
	}
	if when := result.Rules[0].When; when == nil || !strings.Contains(*when, "{e}.状态.触发") {
		t.Fatalf("when = %v, want the trigger condition rewritten onto the actor", when)
	}
	branches := result.Rules[0].Branches
	if len(branches) != 1 || len(branches[0].Then) != 1 {
		t.Fatalf("branches = %+v, want one branch with one op", branches)
	}
	op := branches[0].Then[0]
	if op.Op != "append" || op.Entity != "场景" || op.Path != "事件" {
		t.Fatalf("compiled op = %+v, want append 场景.事件", op)
	}
	if value, ok := op.Value.(string); !ok || value != "$状态.触发" {
		t.Fatalf("op value = %#v, want the $状态.触发 reference", op.Value)
	}
}
