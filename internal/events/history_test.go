package events

import (
	"testing"

	"github.com/ruicat/astrbox/internal/neko"
)

func TestBusHistoryRecordsDispatchOutcomes(t *testing.T) {
	store := newStore(t)
	caller := &fakeCaller{}
	system := NewSystem(store, caller)
	system.RegisterFuncs([]neko.FuncDef{{Name: "体力管理"}})
	system.RegisterEvents([]neko.EventDef{{Name: "体力管理", Handler: "体力管理"}})
	ent, err := store.Create("猫猫", "NPC", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ent.Set("身体.体力", float64(50)); err != nil {
		t.Fatal(err)
	}
	history := system.Bus().History(10)
	if len(history) != 1 || history[0].Status != "ok" || history[0].Name != "体力管理" {
		t.Fatalf("history = %+v", history)
	}
	system.Bus().Fire("未定义事件", Context{EntityID: "猫猫"})
	history = system.Bus().History(10)
	if len(history) != 2 || history[1].Status != "warning" {
		t.Fatalf("history after miss = %+v", history)
	}
	last := system.Bus().History(1)
	if len(last) != 1 || last[0].Name != "未定义事件" {
		t.Fatalf("history(1) = %+v", last)
	}
}
