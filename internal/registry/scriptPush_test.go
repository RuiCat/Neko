// 回归：脚本推送链（交互.推送.待发 -> 消息/版本）在真实脚本集上成立。
package registry

import (
	"testing"

	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/entity"
	"github.com/RuiCat/Neko/internal/events"
	"github.com/RuiCat/Neko/internal/function"
)

// TestScriptPushWritesMessage pins the L6 push rule: writing 待发 produces
// 消息/版本 and clears 待发, without the script touching DSH directly.
func TestScriptPushWritesMessage(t *testing.T) {
	dataDir := t.TempDir()
	store := entity.NewStore(dataDir)
	machine := engine.New(store)
	funcs := function.New(store)
	ev := events.NewSystem(store, funcs)
	machine.SetFuncCaller(funcs)
	loadRepoScripts(t, store, machine, funcs, ev, dataDir)

	if _, err := store.Create("猫猫", "角色基础", map[string]any{"基础": map[string]any{"名字": "猫猫"}}, nil); err != nil {
		t.Fatalf("create: %v", err)
	}
	ent := store.Get("猫猫")
	if err := ent.Set("交互.推送.待发", "猫猫想主人了"); err != nil {
		t.Fatalf("set pending: %v", err)
	}
	machine.Tick("")
	if got := ent.GetOrNone("交互.推送.消息"); got != "猫猫想主人了" {
		t.Fatalf("推送.消息 = %v", got)
	}
	if got := ent.GetOrNone("交互.推送.版本"); got == nil || got == 0 {
		t.Fatalf("推送.版本 = %v", got)
	}
	if got := ent.GetOrNone("交互.推送.待发"); got != "" {
		t.Fatalf("推送.待发 应清空, got %v", got)
	}
}
