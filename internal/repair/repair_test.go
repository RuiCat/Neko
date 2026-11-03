// repair_test.go pins the repair routing (preference excluded, snapshot
// required) and the version/rollback lifecycle.
package repair

import (
	"testing"
	"time"

	"github.com/RuiCat/Neko/internal/triage"
)

func finding(class triage.Class, turnID, snapshotID string) triage.Finding {
	return triage.Finding{
		TurnID: turnID, SnapshotID: snapshotID, Class: class,
		Kind: "state_mismatch", Severity: triage.SEVERITY_ERROR, Detail: "示例",
	}
}

func TestBuildRejectsPreference(t *testing.T) {
	candidates, rejections := Build([]triage.Finding{
		finding(triage.ClassPreference, "evt_1", "evt_1"),
	}, time.Now())
	if len(candidates) != 0 {
		t.Fatalf("偏好样本不应进候选: %+v", candidates)
	}
	if len(rejections) != 1 {
		t.Fatalf("应有一条拒收: %+v", rejections)
	}
}

func TestBuildRejectsMissingSnapshot(t *testing.T) {
	candidates, rejections := Build([]triage.Finding{
		finding(triage.ClassConsistency, "evt_1", ""),
	}, time.Now())
	if len(candidates) != 0 || len(rejections) != 1 {
		t.Fatalf("无快照应拒收: candidates=%d rejections=%d", len(candidates), len(rejections))
	}
}

func TestBuildMapsKind(t *testing.T) {
	candidates, _ := Build([]triage.Finding{
		finding(triage.ClassConsistency, "evt_1", "evt_1"),
		finding(triage.ClassFact, "evt_2", "evt_2"),
	}, time.Now())
	if len(candidates) != 2 {
		t.Fatalf("应得 2 个候选: %+v", candidates)
	}
	if candidates[0].Kind != KindPolicy || candidates[1].Kind != KindRule {
		t.Fatalf("分类映射错误: %+v", candidates)
	}
	if candidates[0].SourceSnapshotID == "" || candidates[0].Hash == "" {
		t.Fatalf("候选缺少快照/哈希: %+v", candidates[0])
	}
}

func TestApplyAndRollback(t *testing.T) {
	store := NewStore(t.TempDir() + "/versions.json")
	now := time.Now()
	candidates := []Candidate{{ID: "repair_a", Hash: "aaaa"}, {ID: "repair_b", Hash: "bbbb"}}

	first, err := store.Apply(candidates, now)
	if err != nil || first.Version != "v1" || first.Count != 2 {
		t.Fatalf("首次 Apply = %+v, %v", first, err)
	}
	second, err := store.Apply(candidates, now)
	if err != nil || second.Version != "v2" {
		t.Fatalf("二次 Apply = %+v, %v", second, err)
	}
	versions, err := store.Load()
	if err != nil || len(versions) != 2 {
		t.Fatalf("Load = %d, %v", len(versions), err)
	}
	rolled, err := store.Rollback()
	if err != nil || rolled.Version != "v2" || !rolled.RolledBack {
		t.Fatalf("Rollback = %+v, %v", rolled, err)
	}
	versions, _ = store.Load()
	if !versions[1].RolledBack || versions[0].RolledBack {
		t.Fatalf("回滚标记错误: %+v", versions)
	}
}

func TestApplyEmptyIsNoop(t *testing.T) {
	store := NewStore(t.TempDir() + "/versions.json")
	version, err := store.Apply(nil, time.Now())
	if err != nil || version.Version != "" {
		t.Fatalf("空批次应为 no-op: %+v, %v", version, err)
	}
}
