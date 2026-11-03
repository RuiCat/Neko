// repairApply_test.go pins the P4 closed loop: teacher signals are judged then
// queued, and an approved candidate is promoted only when the regression gate
// passes, rolling back on failure.
package control

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/RuiCat/Neko/internal/diagnostics"
	"github.com/RuiCat/Neko/internal/engine"
)

func TestTeacherSignalQueuesUncovered(t *testing.T) {
	env := newTestEnv(t, Options{})
	recorder := env.call(t, http.MethodPost, "/teacher-signal", []byte(
		`{"semantic_gate":"pass","transition_candidates":[{"EntityID":"猫猫","Path":"状态.信任值","Proposed":0.3,"Source":"teacher"}]}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("状态码 %d：%s", recorder.Code, recorder.Body.String())
	}
	payload := decodeBody(t, recorder)
	if payload["queued"] != float64(1) || payload["uncovered"] != float64(1) {
		t.Fatalf("uncovered 应入队：%v", payload)
	}
}

func TestTeacherSignalIllegalIsNotQueued(t *testing.T) {
	env := newTestEnv(t, Options{})
	recorder := env.call(t, http.MethodPost, "/teacher-signal", []byte(
		`{"transition_candidates":[{"EntityID":"不存在","Path":"状态.信任值","Proposed":1}]}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("状态码 %d", recorder.Code)
	}
	payload := decodeBody(t, recorder)
	if payload["illegal"] != float64(1) || payload["queued"] != float64(0) {
		t.Fatalf("illegal 不应入队：%v", payload)
	}
}

func TestTeacherSignalMalformedRecordsMissing(t *testing.T) {
	env := newTestEnv(t, Options{})
	recorder := env.call(t, http.MethodPost, "/teacher-signal", []byte(`{bad json`))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("坏 JSON 应 400：%d", recorder.Code)
	}
	summary := env.runtime.Diagnostics.Summary(diagnostics.Filter{})
	if summary.Warning == 0 {
		t.Fatalf("应记录 teacher_signal_missing 警告：%+v", summary)
	}
}

// approvedCandidate queues one uncovered candidate with a snapshot anchor and
// approves it through the resolve endpoint.
func approvedCandidate(t *testing.T, env *testEnv) string {
	t.Helper()
	judged := env.call(t, http.MethodPost, "/judge", []byte(
		`{"entity_id":"猫猫","path":"状态.信任值","proposed":0.3,"source":"teacher","evidence":["snapshot:evt_20261103_0001"]}`))
	id, _ := decodeBody(t, judged)["candidate_id"].(string)
	if id == "" {
		t.Fatalf("候选未入队：%s", judged.Body.String())
	}
	resolved := env.call(t, http.MethodPost, "/candidates/resolve", []byte(`{"id":"`+id+`","status":"approved"}`))
	if resolved.Code != http.StatusOK {
		t.Fatalf("批准失败 %d：%s", resolved.Code, resolved.Body.String())
	}
	return id
}

func TestApplyCandidatePromotesWhenGatePasses(t *testing.T) {
	env := newTestEnv(t, Options{})
	id := approvedCandidate(t, env)
	recorder := env.call(t, http.MethodPost, "/candidates/apply", []byte(`{"id":"`+id+`","when":"状态.信任值 < 1"}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("上线失败 %d：%s", recorder.Code, recorder.Body.String())
	}
	payload := decodeBody(t, recorder)
	version, _ := payload["version"].(map[string]any)
	if version["version"] != "v1" {
		t.Fatalf("应得 v1：%v", payload)
	}
	if _, err := os.Stat(filepath.Join(env.dataDir, "repairs", "versions.json")); err != nil {
		t.Fatalf("版本文件未落盘: %v", err)
	}
	foundRule := false
	for _, rule := range env.runtime.Machine.Rules() {
		if rule.Source == "repair" {
			foundRule = true
		}
	}
	if !foundRule {
		t.Fatalf("apply 后 engine 应含 repair 规则")
	}
	verdict, err := env.runtime.Machine.Judge(engine.Candidate{EntityID: "猫猫", Path: "状态.信任值"})
	if err != nil || verdict.Status != engine.VERDICT_LEGAL {
		t.Fatalf("修复规则应覆盖该路径：%+v, %v", verdict, err)
	}
}

func TestApplyCandidateRejectsUnapproved(t *testing.T) {
	env := newTestEnv(t, Options{})
	judged := env.call(t, http.MethodPost, "/judge", []byte(
		`{"entity_id":"猫猫","path":"状态.信任值","proposed":0.3,"evidence":["snapshot:evt_x"]}`))
	id, _ := decodeBody(t, judged)["candidate_id"].(string)
	recorder := env.call(t, http.MethodPost, "/candidates/apply", []byte(`{"id":"`+id+`"}`))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("未批准候选应 400：%d %s", recorder.Code, recorder.Body.String())
	}
}

func TestApplyCandidateRequiresSnapshotAnchor(t *testing.T) {
	env := newTestEnv(t, Options{})
	judged := env.call(t, http.MethodPost, "/judge", []byte(
		`{"entity_id":"猫猫","path":"状态.信任值","proposed":0.3}`))
	id, _ := decodeBody(t, judged)["candidate_id"].(string)
	env.call(t, http.MethodPost, "/candidates/resolve", []byte(`{"id":"`+id+`","status":"approved"}`))
	recorder := env.call(t, http.MethodPost, "/candidates/apply", []byte(`{"id":"`+id+`"}`))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("无快照锚点应 400：%d %s", recorder.Code, recorder.Body.String())
	}
}

func TestApplyCandidateRollsBackOnGateFailure(t *testing.T) {
	env := newTestEnv(t, Options{})
	id := approvedCandidate(t, env)
	// A drifted journal record makes the replay audit fail.
	journalDir := filepath.Join(env.dataDir, "journal")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	drifted := `{"record_version":1,"turn_id":"evt_20261103_0001","status":"complete","source":"dsh-turn","started_at":"2026-11-03T10:00:00+08:00","snapshot":{"snapshot_id":"evt_20261103_0001","turn_id":"evt_20261103_0001","state":{"猫猫.状态.情绪":"平静"}},"gate":{"version":"gate-v1","active":["事件记忆","表达偏好"]},"compensation":{"state":{"猫猫.状态.情绪":"愤怒"},"state_source":"state_machine_snapshot","snapshot_id":"evt_20261103_0001"},"answer":"喵","feedback":{"kind":"none"}}`
	if err := os.WriteFile(filepath.Join(journalDir, "2026-11-03.jsonl"), []byte(drifted+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	recorder := env.call(t, http.MethodPost, "/candidates/apply", []byte(`{"id":"`+id+`","when":"状态.信任值 < 1"}`))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("闸门失败应 409：%d %s", recorder.Code, recorder.Body.String())
	}
	payload := decodeBody(t, recorder)
	if _, ok := payload["rolled_back"].(map[string]any); !ok {
		t.Fatalf("失败应回滚：%v", payload)
	}
}

func TestRollbackRepair(t *testing.T) {
	env := newTestEnv(t, Options{})
	id := approvedCandidate(t, env)
	env.call(t, http.MethodPost, "/candidates/apply", []byte(`{"id":"`+id+`","when":"状态.信任值 < 1"}`))
	recorder := env.call(t, http.MethodPost, "/repairs/rollback", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("回滚失败 %d：%s", recorder.Code, recorder.Body.String())
	}
	if _, ok := decodeBody(t, recorder)["rolled_back"].(map[string]any); !ok {
		t.Fatalf("缺少 rolled_back：%s", recorder.Body.String())
	}
	for _, rule := range env.runtime.Machine.Rules() {
		if rule.Source == "repair" {
			t.Fatalf("回滚后不应残留 repair 规则：%+v", rule)
		}
	}
}

func TestApplyCandidateRequiresWhen(t *testing.T) {
	env := newTestEnv(t, Options{})
	id := approvedCandidate(t, env)
	recorder := env.call(t, http.MethodPost, "/candidates/apply", []byte(`{"id":"`+id+`"}`))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("缺少 when 应 400：%d %s", recorder.Code, recorder.Body.String())
	}
}
