// v2_test.go is the V2 acceptance walk: the 收集任务 template drives two
// completions, and the growth/affinity rewards must be paid exactly once while
// the second advance reports a replay (C4 single settlement).
package verify

import (
	"testing"

	"github.com/RuiCat/Neko/internal/flow/task"
)

// TestTaskSingleSettlement advances the checked-in 收集任务 template twice and
// pins the reward rows: experience 0 → 20 and affinity 50 → 55, one write each,
// while the task stage row stays untouched (it belongs to the rules).
func TestTaskSingleSettlement(t *testing.T) {
	document := flowSpecFixture(t)
	template := taskTemplateByID(t, document, "收集任务")
	if len(template.Stages) != 3 {
		t.Fatalf("收集任务 stages = %v, want the three-stage library template", template.Stages)
	}
	if template.Reward.Experience != 20 || template.Reward.Affinity != 5 {
		t.Fatalf("收集任务 reward = %+v, want 20/5", template.Reward)
	}

	store := newFixtureStore()
	first, second := advanceTaskTwice(t, store, template)
	if first.Replay {
		t.Fatal("first advance must not be a replay")
	}
	if len(first.Rewards) != 2 {
		t.Fatalf("first advance rewards = %+v, want 2 rows", first.Rewards)
	}
	if !second.Replay {
		t.Fatal("second advance must be a replay")
	}
	if len(second.Rewards) != 0 {
		t.Fatalf("replay paid rewards: %+v", second.Rewards)
	}

	requireInt(t, store, task.PATH_GROWTH_EXPERIENCE, 20)
	requireInt(t, store, task.PATH_RELATION_AFFINITY, 55)
	if got := store.writeCount(task.PATH_GROWTH_EXPERIENCE); got != 1 {
		t.Fatalf("experience writes = %d, want 1", got)
	}
	if got := store.writeCount(task.PATH_RELATION_AFFINITY); got != 1 {
		t.Fatalf("affinity writes = %d, want 1", got)
	}
	if got := store.writeCount(task.PATH_TASK_STAGE); got != 0 {
		t.Fatalf("task stage writes = %d, want 0 (rule-owned)", got)
	}
	requireInt(t, store, task.PATH_TASK_PROGRESS, task.PROGRESS_COMPLETE)
	requireText(t, store, task.PATH_TASK_CURRENT, template.ID)
	t.Logf("task ok: 成长.经验=20(1 write) 关系.数值.好感度=55(1 write) replay=%v", second.Replay)
}
