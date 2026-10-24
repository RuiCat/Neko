package events

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ruicat/astrbox/internal/entity"
	"github.com/ruicat/astrbox/internal/neko"
)

type fakeCaller struct {
	calls []call
	defs  map[string]neko.FuncDef
}

type call struct {
	name   string
	args   []any
	entity string
}

func (f *fakeCaller) Call(name string, args []any, current *entity.Entity) (any, error) {
	entityID := ""
	if current != nil {
		entityID = current.ID
	}
	f.calls = append(f.calls, call{name: name, args: args, entity: entityID})
	return "ok", nil
}

func (f *fakeCaller) Register(def neko.FuncDef) error {
	if f.defs == nil {
		f.defs = map[string]neko.FuncDef{}
	}
	f.defs[def.Name] = def
	return nil
}

func (f *fakeCaller) Unregister(name string) { delete(f.defs, name) }

func newStore(t *testing.T) *entity.Store {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	template := `{"身体":{"体力":75},"状态":{"身体":"活跃"},"__annotations__":{"身体.体力":{"type":"int","event":"体力管理","min":0,"max":100}}}`
	if err := os.WriteFile(filepath.Join(dir, "templates", "NPC.json"), []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}
	store := entity.NewStore(dir)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestFieldChangeFiresBoundEvent(t *testing.T) {
	store := newStore(t)
	caller := &fakeCaller{}
	system := NewSystem(store, caller)
	system.RegisterFuncs([]neko.FuncDef{{Name: "体力管理"}})
	system.RegisterEvents([]neko.EventDef{{
		Name: "体力管理", Condition: "变化量 < -20", Handler: "体力管理",
	}})
	ent, err := store.Create("猫猫", "NPC", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ent.Set("身体.体力", float64(50)); err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(caller.calls))
	}
	if caller.calls[0].name != "体力管理" {
		t.Fatalf("handler = %q", caller.calls[0].name)
	}
	if delta, ok := caller.calls[0].args[0].(float64); !ok || delta != -25 {
		t.Fatalf("delta = %v", caller.calls[0].args[0])
	}
	if caller.calls[0].entity != "猫猫" {
		t.Fatalf("entity = %q", caller.calls[0].entity)
	}

	// Condition does not match: delta +20, no extra call.
	if err := ent.Set("身体.体力", float64(70)); err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("condition not enforced, calls = %d", len(caller.calls))
	}
}

func TestUnboundFieldDoesNotFire(t *testing.T) {
	store := newStore(t)
	caller := &fakeCaller{}
	system := NewSystem(store, caller)
	system.RegisterEvents([]neko.EventDef{{Name: "体力管理", Handler: "体力管理"}})
	ent, err := store.Create("猫猫", "NPC", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ent.Set("状态.身体", "疲惫"); err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("unexpected calls: %v", caller.calls)
	}
}

func TestEventConditionArithmetic(t *testing.T) {
	store := newStore(t)
	caller := &fakeCaller{}
	system := NewSystem(store, caller)
	system.RegisterEvents([]neko.EventDef{{
		Name: "体力管理", Condition: "变化量 + 1 > 0", Handler: "体力管理",
	}})
	ent, err := store.Create("猫猫", "NPC", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ent.Set("身体.体力", float64(74.5)); err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 {
		t.Fatalf("caller calls = %d", len(caller.calls))
	}
}

func TestValidationLog(t *testing.T) {
	store := newStore(t)
	caller := &fakeCaller{}
	bridge := NewBridge(store, NewBus(store, caller))
	ent, err := store.Create("猫猫", "NPC", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	bridge.Attach(ent)
	if err := ent.Set("身体.体力", float64(999)); err != nil {
		t.Fatal(err)
	}
	validations := bridge.LastValidations(10)
	if len(validations) != 1 {
		t.Fatalf("validations = %d", len(validations))
	}
	if validations[0]["path"] != "身体.体力" {
		t.Fatalf("validation = %+v", validations[0])
	}
}
