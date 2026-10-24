package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ruicat/astrbox/internal/engine"
	"github.com/ruicat/astrbox/internal/entity"
	"github.com/ruicat/astrbox/internal/events"
	"github.com/ruicat/astrbox/internal/function"
)

func TestReplayAllScripts(t *testing.T) {
	dataDir := t.TempDir()
	scriptsDir := t.TempDir()
	if err := os.CopyFS(dataDir, os.DirFS("../../data")); err != nil {
		t.Fatalf("copy data dir: %v", err)
	}
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
	if got := len(reg.List()); got != 186 {
		t.Fatalf("registered scripts = %d, want 186", got)
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
	if len(replay.Replayed) < 180 {
		t.Fatalf("replayed only %d scripts", len(replay.Replayed))
	}
	if len(machine.Rules()) < 1700 {
		t.Fatalf("only %d rules loaded", len(machine.Rules()))
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

	if !funcs.Has("时段判断") || !funcs.Has("体力管理") {
		t.Fatal("corpus functions not registered")
	}
	if _, ok := system.Bus().Get("体力管理"); !ok {
		t.Fatal("event 体力管理 not registered")
	}
	period, err := funcs.Call("时段判断", []any{"2026-04-01T15:00"}, nil)
	if err != nil || period != "下午" {
		t.Fatalf("时段判断 = %v, %v", period, err)
	}
	cat := store.Get("猫猫")
	if cat == nil {
		t.Fatal("entity 猫猫 missing")
	}
	current := cat.GetOrNone("身体.体力")
	number, ok := current.(float64)
	if !ok {
		t.Fatalf("体力 is %T", current)
	}
	if err := cat.Set("身体.体力", number-25); err != nil {
		t.Fatal(err)
	}
	if got := cat.GetOrNone("状态.身体"); got != "疲惫" {
		t.Fatalf("event handler did not fire: 状态.身体 = %v", got)
	}
}
