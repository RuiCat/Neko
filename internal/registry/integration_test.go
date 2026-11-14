package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/entity"
	"github.com/RuiCat/Neko/internal/events"
	"github.com/RuiCat/Neko/internal/function"
	"github.com/RuiCat/Neko/internal/testutil"
)

func TestReplayAllScripts(t *testing.T) {
	dataDir := t.TempDir()
	scriptsDir := t.TempDir()
	testutil.CopyDataDir(t, "../../data", dataDir)
	if err := os.CopyFS(scriptsDir, os.DirFS("../../scripts")); err != nil {
		t.Fatalf("copy scripts dir: %v", err)
	}
	store := entity.NewStore(dataDir)
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	machine := engine.New(store)
	funcs := function.New(store)
	system := events.NewSystem(store, funcs)
	machine.SetFuncCaller(funcs)
	reg, err := New(store, machine, filepath.Join(dataDir, "scripts.json"), scriptsDir)
	if err == nil {
		reg.SetFunctionSink(system)
	}
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	// data/scripts.json 是派生索引（可过期）：先从 sources 引导，再断言注册表覆盖整个 scripts/ 树。
	if _, err := reg.BootstrapFromSources(); err != nil {
		t.Fatalf("bootstrap from sources: %v", err)
	}
	// 脚本语料 2026-11-14 重写：数量按 scripts/ 目录动态统计。
	scriptCount := 0
	_ = filepath.WalkDir(scriptsDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() && isContentPath(scriptsDir, path) {
			return filepath.SkipDir
		}
		if !entry.IsDir() && filepath.Ext(path) == ".neko" {
			scriptCount++
		}
		return nil
	})
	if got := len(reg.List()); got != scriptCount {
		t.Fatalf("registered scripts = %d, want %d", got, scriptCount)
	}
	registryPath := filepath.Join(dataDir, "scripts.json")
	before, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	replay := reg.ReplayAll()
	after, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("startup replay rewrote scripts.json")
	}
	if len(replay.Failed) != 0 {
		t.Fatalf("replay failures: %v %v", replay.Failed, replay.Details)
	}
	if len(replay.Replayed) != scriptCount {
		t.Fatalf("replayed %d scripts, want %d", len(replay.Replayed), scriptCount)
	}
	if len(machine.Rules()) == 0 {
		t.Fatal("no rules loaded")
	}
	tick := machine.Tick("*")
	raw, err := json.Marshal(tick)
	if err != nil {
		t.Fatalf("tick result not serializable: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("empty tick result")
	}
	t.Logf("tick: transitions=%d conflicts=%d rules=%d", len(tick.Transitions), len(tick.Conflicts), len(machine.Rules()))

	if !funcs.Has("代谢投影体力") {
		t.Fatal("corpus functions not registered")
	}
	if _, ok := system.Bus().Get("事件休克"); !ok {
		t.Fatal("event 事件休克 not registered")
	}
	cat := store.Get("测试猫")
	if cat == nil {
		t.Skip("data/ 里没有 测试猫 实例，跳过实体断言（需先 neko-data create）")
	}
	if cat.GetOrNone("代谢.体温") == nil {
		t.Fatal("代谢.体温 missing")
	}
	if err := cat.Set("伤害.失血量", 50); err != nil {
		t.Fatal(err)
	}
	machine.Tick("测试猫")
	if got := cat.GetOrNone("状态.身体"); got != "休克" {
		t.Fatalf("状态机未结算: 状态.身体 = %v", got)
	}
}
