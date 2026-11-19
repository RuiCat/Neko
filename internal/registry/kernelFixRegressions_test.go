// 回归：本轮排查修掉的 6 个内核逻辑缺陷。
//   B 遗忘永不触发（条件右侧 $.路径 不被重写）
//   C 周期永不推进（int(流逝分钟/1440) 在 60 分钟 tick 下恒 0）
//   D 受孕滞后一拍（5000 早于周期的 5100）
//   E 教育读不到动作（行动本 tick 完成即清空 行动.当前）
//   F 状态词不看疼痛（疼痛 100 仍报「正常」）
//   G 身体.* 投影字段无模板声明
package registry

import (
	"testing"

	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/entity"
	"github.com/RuiCat/Neko/internal/events"
	"github.com/RuiCat/Neko/internal/function"
)

func newKernelFixRuntime(t *testing.T) (*entity.Store, *engine.StateMachine, *entity.Entity) {
	t.Helper()
	dataDir := t.TempDir()
	store := entity.NewStore(dataDir)
	machine := engine.New(store)
	funcs := function.New(store)
	ev := events.NewSystem(store, funcs)
	machine.SetFuncCaller(funcs)
	loadRepoScripts(t, store, machine, funcs, ev, dataDir)

	if _, err := store.Create("猫猫", "角色定义:猫娘", map[string]any{
		"基础": map[string]any{"名字": "猫猫", "性别": "女", "年龄": 20},
	}, nil); err != nil {
		t.Fatalf("create: %v", err)
	}
	ent := store.Get("猫猫")
	set := func(path string, value any) {
		t.Helper()
		if err := ent.Set(path, value); err != nil {
			t.Fatalf("set %s: %v", path, err)
		}
	}
	set("代谢.流逝分钟", 60)
	machine.Tick("")
	return store, machine, ent
}

func TestMemoryForgetTriggers(t *testing.T) {
	_, machine, ent := newKernelFixRuntime(t)
	if err := ent.Set("记忆.短期负荷", 5); err != nil {
		t.Fatal(err)
	}
	if err := ent.Set("记忆.遗忘阈值", 10); err != nil {
		t.Fatal(err)
	}
	if err := ent.Set("记忆.遗忘速率", 1); err != nil {
		t.Fatal(err)
	}
	machine.Tick("")
	if got := asIntValue(t, ent, "记忆.短期负荷"); got != 0 {
		t.Fatalf("短期负荷 %d，want 0（遗忘未触发：条件右侧 $.路径 不会被重写）", got)
	}
}

func TestCycleAdvancesOnShortTicks(t *testing.T) {
	_, machine, ent := newKernelFixRuntime(t)
	// 60 分钟/tick，累计 24 tick = 1440 分钟 = 1 天。
	for i := 0; i < 24; i++ {
		if err := ent.Set("代谢.流逝分钟", 60); err != nil {
			t.Fatal(err)
		}
		machine.Tick("")
	}
	if got := asIntValue(t, ent, "生殖.周期日"); got != 1 {
		t.Fatalf("周期日 %d，want 1（周期应按累积分钟推进，int(流逝分钟/1440) 恒 0 的旧实现算不到）", got)
	}
}

func TestConceptionJudgedInSameTick(t *testing.T) {
	_, machine, ent := newKernelFixRuntime(t)
	for path, value := range map[string]any{
		"生殖.周期分钟":  12 * 1440,
		"生殖.受孕概率":  100,
		"基础.可孕":    true,
		"行为.体内暴露":  true,
		"代谢.流逝分钟": 60,
	} {
		if err := ent.Set(path, value); err != nil {
			t.Fatalf("set %s: %v", path, err)
		}
	}
	machine.Tick("")
	// 周期在 5100 重算窗口，受孕在 5150 判定 —— 同一 tick 内窗口成立就该命中。
	if got := ent.GetOrNone("生殖.怀孕状态"); got == "未孕" {
		t.Fatalf("受孕未在同 tick 判定（状态=%v）：说明受孕仍早于周期推进", got)
	}
}

func TestEducationSettlesOnCompletionTick(t *testing.T) {
	_, machine, ent := newKernelFixRuntime(t)
	for path, value := range map[string]any{
		"行动.当前":   "学习",
		"行动.总分钟":  60,
		"代谢.流逝分钟": 60,
	} {
		if err := ent.Set(path, value); err != nil {
			t.Fatalf("set %s: %v", path, err)
		}
	}
	machine.Tick("")
	if got := asIntValue(t, ent, "教育.已学分钟"); got == 0 {
		t.Fatal("动作在本 tick 完成时教育读不到 行动.当前，学习未结算")
	}
	// 完成态应保留 当前 供下游读取，下一 tick 由 acResetDone 回到空闲。
	if got := ent.GetOrNone("行动.阶段"); got != "完成" {
		t.Fatalf("阶段 = %v，want 完成", got)
	}
	if err := ent.Set("代谢.流逝分钟", 60); err != nil {
		t.Fatal(err)
	}
	machine.Tick("")
	if got := ent.GetOrNone("行动.阶段"); got != "空闲" {
		t.Fatalf("下一 tick 阶段 = %v，want 空闲（完成态未回收会让动作无限重启）", got)
	}
	if got := ent.GetOrNone("行动.当前"); got != "" {
		t.Fatalf("下一 tick 行动.当前 = %v，want 空", got)
	}
	// 不能因为回收而重复结算学习。
	before := asIntValue(t, ent, "教育.已学分钟")
	if err := ent.Set("代谢.流逝分钟", 60); err != nil {
		t.Fatal(err)
	}
	machine.Tick("")
	if got := asIntValue(t, ent, "教育.已学分钟"); got != before {
		t.Fatalf("空闲后学习仍在推进: %d -> %d", before, got)
	}
}

func TestStatusSeesPain(t *testing.T) {
	_, machine, ent := newKernelFixRuntime(t)
	// 伤害.疼痛 每 tick 由 伤害疼痛下一步() 重算，必须用真实来源。
	if err := ent.Set("伤害.失血量", 30); err != nil {
		t.Fatal(err)
	}
	if err := ent.Set("路径.类型", "无"); err != nil {
		t.Fatal(err)
	}
	if err := ent.Set("路径.应力", 100); err != nil {
		t.Fatal(err)
	}
	if err := ent.Set("代谢.流逝分钟", 60); err != nil {
		t.Fatal(err)
	}
	machine.Tick("")
	if pain := asIntValue(t, ent, "伤害.疼痛"); pain < 70 {
		t.Fatalf("前置条件不成立：疼痛 = %d，want >=70", pain)
	}
	if got := ent.GetOrNone("状态.身体"); got != "剧痛" {
		t.Fatalf("状态.身体 = %v，want 剧痛（状态词原本完全不看疼痛）", got)
	}
}

func TestBodyProjectionFieldsDeclared(t *testing.T) {
	_, _, ent := newKernelFixRuntime(t)
	// 投影 tick 每 tick 都写这三个字段；若模板没声明，写入落在未定义路径上，
	// 读取方（状态词、外部工具）会拿到 <err> 而不是数值。
	for _, path := range []string{"身体.体力", "身体.饱食", "身体.水分"} {
		if _, err := ent.Get(path); err != nil {
			t.Errorf("%s 未被声明: %v", path, err)
		}
	}
}
