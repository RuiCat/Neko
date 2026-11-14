// e2eGateTools_test.go covers the agent-facing training tools over the real
// stdio transport: teacher samples land in the data dir, status reflects them,
// and gate_train invokes the training script (dry-run by default).
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// toolText extracts the first text content of one tool result.
func toolText(t *testing.T, session *stdioSession, name string, arguments map[string]any) string {
	t.Helper()
	result, err := session.callToolErr(name, arguments)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	items, ok := result["content"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("%s 缺少 content: %v", name, result)
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("%s content 条目不是对象: %v", name, items[0])
	}
	text, _ := item["text"].(string)
	return text
}

// toolJSON decodes the JSON body of one tool result.
func toolJSON(t *testing.T, session *stdioSession, name string, arguments map[string]any) map[string]any {
	t.Helper()
	text := toolText(t, session, name, arguments)
	body := map[string]any{}
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		t.Fatalf("%s 非 JSON: %v (%s)", name, err, text)
	}
	return body
}

// TestE2EGateTeachAndStatus proves teacher samples written through the real
// stdio tool surface show up in gate_status.
func TestE2EGateTeachAndStatus(t *testing.T) {
	session := startStdioSession(t)
	session.initialize(t)

	empty := toolJSON(t, session, "gate_status", map[string]any{})
	if empty["exportable"] != float64(0) {
		t.Fatalf("初始状态应无样本: %v", empty)
	}

	appended := toolJSON(t, session, "gate_teach", map[string]any{
		"samples": []any{
			map[string]any{
				"target_expert": "知识边界", "event_kind": "question",
				"summary": "猫猫的设定是什么？", "hard_state_keys": []any{"知识边界"},
			},
			map[string]any{
				"target_expert": "自我记忆", "event_kind": "emotion",
				"summary": "今天好难过", "affect": map[string]any{"valence": -0.8},
			},
		},
	})
	if appended["appended"] != float64(2) {
		t.Fatalf("教师样本提交失败: %v", appended)
	}

	status := toolJSON(t, session, "gate_status", map[string]any{})
	if status["teacher_samples"] != float64(2) || status["exportable"] != float64(2) {
		t.Fatalf("状态统计错误: %v", status)
	}
	labels, ok := status["labels"].(map[string]any)
	if !ok || labels["知识边界"] != float64(1) || labels["自我记忆"] != float64(1) {
		t.Fatalf("标签分布错误: %v", status["labels"])
	}

	// 非法样本整批拒绝，状态不变。
	text := toolText(t, session, "gate_teach", map[string]any{
		"samples": []any{map[string]any{"target_expert": "表达偏好"}},
	})
	if !strings.Contains(text, "❌") {
		t.Fatalf("非法教师样本未拒绝: %s", text)
	}
	after := toolJSON(t, session, "gate_status", map[string]any{})
	if after["teacher_samples"] != float64(2) {
		t.Fatalf("非法样本不应落盘: %v", after)
	}

	if code := session.shutdown(t); code != EXIT_OK {
		t.Fatalf("exit code = %d (stderr=%s)", code, session.stderr.String())
	}
}

// TestE2EGateTrainInvokesScript proves gate_train reaches the real binary's
// script runner: dry-run by default, deploy only when asked.
func TestE2EGateTrainInvokesScript(t *testing.T) {
	root := t.TempDir()
	scriptDir := filepath.Join(root, "scripts", "tools")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	script := filepath.Join(scriptDir, "trainGateModel.sh")
	if err := os.WriteFile(script, []byte("#!/usr/bin/env bash\nprintf 'FAKE-ARGS: %s\\n' \"$*\"\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin", "astrbox-train"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write train binary: %v", err)
	}
	t.Setenv("NEKO_ROOT", root)

	session := startStdioSession(t)
	session.initialize(t)

	dry := toolJSON(t, session, "gate_train", map[string]any{"min_samples": 12})
	if dry["ok"] != true || dry["deploy"] != false {
		t.Fatalf("dry-run 结果错误: %v", dry)
	}
	tail, _ := dry["output_tail"].(string)
	if !strings.Contains(tail, "--dry-run") || !strings.Contains(tail, "--min-samples 12") {
		t.Fatalf("dry-run 参数错误: %s", tail)
	}

	deployed := toolJSON(t, session, "gate_train", map[string]any{"deploy": true})
	if deployed["ok"] != true || deployed["deploy"] != true {
		t.Fatalf("deploy 结果错误: %v", deployed)
	}
	if tail, _ := deployed["output_tail"].(string); strings.Contains(tail, "--dry-run") {
		t.Fatalf("deploy 不应带 --dry-run: %s", tail)
	}

	if code := session.shutdown(t); code != EXIT_OK {
		t.Fatalf("exit code = %d (stderr=%s)", code, session.stderr.String())
	}
}
