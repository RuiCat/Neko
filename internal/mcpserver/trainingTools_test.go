// trainingTools_test.go covers the agent-facing gate tools: teacher samples,
// training-data status and the one-click training invocation (default dry-run,
// deploy only on request).
package mcpserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RuiCat/Neko/internal/learn/samples"
)

// writeFakeRoot 在临时目录里搭一个最小仓库根：训练脚本 + bin/astrbox-train。
func writeFakeRoot(t *testing.T, scriptBody string) string {
	t.Helper()
	root := t.TempDir()
	scriptDir := filepath.Join(root, "scripts", "tools")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("mkdir script dir: %v", err)
	}
	script := filepath.Join(scriptDir, "trainGateModel.sh")
	if err := os.WriteFile(script, []byte(scriptBody), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	trainBinary := filepath.Join(binDir, "astrbox-train")
	if err := os.WriteFile(trainBinary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write train binary: %v", err)
	}
	return root
}

// TestGateTeachAndStatus covers teacher-sample submission and the status view.
func TestGateTeachAndStatus(t *testing.T) {
	fixture := newTickFixture(t)
	session, cancel := connect(t, fixture.server)
	defer cancel()

	empty := callToolJSON(t, session, "gate_status", map[string]any{})
	if empty["ok"] != true || empty["exportable"] != float64(0) {
		t.Fatalf("空数据状态错误: %v", empty)
	}
	coverage, ok := empty["coverage"].(map[string]any)
	if !ok || coverage["ready"] != false {
		t.Fatalf("空数据不应 ready: %v", empty["coverage"])
	}

	appended := callToolJSON(t, session, "gate_teach", map[string]any{
		"samples": []any{
			map[string]any{
				"target_expert": "知识边界", "event_kind": "question",
				"summary": "猫猫的设定是什么？", "hard_state_keys": []any{"知识边界"},
			},
			map[string]any{
				"target_expert": "关系记忆", "event_kind": "relationship",
				"summary": "我想你了", "affect": map[string]any{"trust": 0.4},
			},
		},
	})
	if appended["ok"] != true || appended["appended"] != float64(2) {
		t.Fatalf("教师样本提交失败: %v", appended)
	}
	records, err := samples.ReadFile(samples.Path(fixture.dataDir, samples.TEACHER_FILE))
	if err != nil || len(records) != 2 {
		t.Fatalf("教师样本 = %d 条, err=%v", len(records), err)
	}
	if records[0].Source != TEACHER_SOURCE || records[0].Primary != "知识边界" {
		t.Fatalf("教师样本标记错误: %+v", records[0])
	}

	status := callToolJSON(t, session, "gate_status", map[string]any{})
	if status["teacher_samples"] != float64(2) || status["exportable"] != float64(2) {
		t.Fatalf("状态统计错误: %v", status)
	}
	labels := status["labels"].(map[string]any)
	if labels["知识边界"] != float64(1) || labels["关系记忆"] != float64(1) {
		t.Fatalf("标签分布错误: %v", labels)
	}
}

// TestGateTeachRejectsInvalidSamples proves a bad entry rejects the whole batch
// without writing a partial file.
func TestGateTeachRejectsInvalidSamples(t *testing.T) {
	fixture := newTickFixture(t)
	session, cancel := connect(t, fixture.server)
	defer cancel()

	cases := []map[string]any{
		{"samples": []any{map[string]any{"target_expert": "表达偏好"}}},
		{"samples": []any{map[string]any{"target_expert": "未知专家"}}},
		{"samples": []any{map[string]any{"target_expert": "事件记忆", "event_kind": "不存在的类型"}}},
		{"samples": []any{}},
	}
	for _, args := range cases {
		text := callToolText(t, session, "gate_teach", args)
		if !strings.Contains(text, "❌") {
			t.Fatalf("非法教师样本应被拒绝: %s", text)
		}
	}
	if _, err := os.Stat(samples.Path(fixture.dataDir, samples.TEACHER_FILE)); !os.IsNotExist(err) {
		t.Fatalf("非法提交不应落盘: %v", err)
	}
}

// TestGateTrainDryRunByDefault proves the tool defaults to dry-run and only
// passes deploy through when explicitly requested.
func TestGateTrainDryRunByDefault(t *testing.T) {
	root := writeFakeRoot(t, "#!/usr/bin/env bash\nprintf 'FAKE-ARGS: %s\\n' \"$*\"\n")
	t.Setenv("NEKO_ROOT", root)
	fixture := newTickFixture(t)
	session, cancel := connect(t, fixture.server)
	defer cancel()

	dry := callToolJSON(t, session, "gate_train", map[string]any{
		"min_samples": 12, "min_accuracy": 0.9, "epochs": 7,
	})
	if dry["ok"] != true || dry["deploy"] != false {
		t.Fatalf("dry-run 结果错误: %v", dry)
	}
	tail, ok := dry["output_tail"].(string)
	if !ok || !strings.Contains(tail, "--dry-run") {
		t.Fatalf("dry-run 未传 --dry-run: %v", dry["output_tail"])
	}
	for _, fragment := range []string{"--min-samples 12", "--min-accuracy 0.9", "--epochs 7", "--max-class-share 0.7", "--data-dir"} {
		if !strings.Contains(tail, fragment) {
			t.Fatalf("训练参数缺少 %q: %s", fragment, tail)
		}
	}

	balanced := callToolJSON(t, session, "gate_train", map[string]any{"max_class_share": 0.25})
	if tail, _ := balanced["output_tail"].(string); !strings.Contains(tail, "--max-class-share 0.25") {
		t.Fatalf("max_class_share 未透传: %s", tail)
	}

	deployed := callToolJSON(t, session, "gate_train", map[string]any{"deploy": true})
	if deployed["ok"] != true || deployed["deploy"] != true {
		t.Fatalf("deploy 结果错误: %v", deployed)
	}
	deployTail, _ := deployed["output_tail"].(string)
	if strings.Contains(deployTail, "--dry-run") {
		t.Fatalf("deploy 不应带 --dry-run: %s", deployTail)
	}
}

// TestGateTrainReportsFailure proves a non-zero script exit is reported instead
// of turned into a silent success.
func TestGateTrainReportsFailure(t *testing.T) {
	root := writeFakeRoot(t, "#!/usr/bin/env bash\necho '门槛拒绝'\nexit 3\n")
	t.Setenv("NEKO_ROOT", root)
	fixture := newTickFixture(t)
	session, cancel := connect(t, fixture.server)
	defer cancel()

	result := callToolJSON(t, session, "gate_train", map[string]any{})
	if result["ok"] != false || result["exit_code"] != float64(3) {
		t.Fatalf("失败结果错误: %v", result)
	}
	if tail, _ := result["output_tail"].(string); !strings.Contains(tail, "门槛拒绝") {
		t.Fatalf("失败输出丢失: %v", result["output_tail"])
	}
}

// TestGateTrainNeedsScript proves a missing training script is reported.
func TestGateTrainNeedsScript(t *testing.T) {
	t.Setenv("NEKO_ROOT", t.TempDir())
	fixture := newTickFixture(t)
	session, cancel := connect(t, fixture.server)
	defer cancel()

	text := callToolText(t, session, "gate_train", map[string]any{})
	if !strings.Contains(text, "trainGateModel.sh") {
		t.Fatalf("缺少脚本时应给出提示: %s", text)
	}
}

// TestGateStatusJSONShape pins the status body keys the agent relies on.
func TestGateStatusJSONShape(t *testing.T) {
	fixture := newTickFixture(t)
	session, cancel := connect(t, fixture.server)
	defer cancel()

	text := callToolText(t, session, "gate_status", map[string]any{})
	body := map[string]any{}
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		t.Fatalf("gate_status 非 JSON: %v (%s)", err, text)
	}
	for _, key := range []string{"auto_samples", "teacher_samples", "exportable", "labels", "coverage", "thresholds", "model"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("缺少状态字段 %q: %v", key, body)
		}
	}
}
