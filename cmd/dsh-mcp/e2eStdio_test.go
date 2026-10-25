package main

import (
	"net/http"
	"os"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"
)

// FROZEN_TOOLS is the K6 tool-name set exposed by dsh-mcp (alphabetical).
var frozenTools = []string{
	"diagnostics_query", "entity_create", "entity_query", "entity_snapshot",
	"entity_state_get", "entity_state_set", "event_log", "inventory",
	"neko_script_apply", "neko_script_list", "neko_script_read",
	"neko_script_remove", "neko_script_validate", "neko_script_write",
	"state_machine_resolve", "state_machine_tick", "stats",
}

// TestE2EStdioToolsListAndStdoutPurity checks tools/list equals the frozen 17
// names and that stdout carries nothing but MCP frames.
func TestE2EStdioToolsListAndStdoutPurity(t *testing.T) {
	session := startStdioSession(t)
	initializeResult := session.initialize(t)
	if initializeResult["protocolVersion"] != "2025-06-18" {
		t.Fatalf("protocolVersion = %v", initializeResult["protocolVersion"])
	}

	response := session.request(t, "tools/list", map[string]any{})
	result, ok := response["result"].(map[string]any)
	if !ok {
		t.Fatalf("tools/list 缺少 result: %v", response)
	}
	rawTools, ok := result["tools"].([]any)
	if !ok {
		t.Fatalf("tools 字段缺失: %v", result["tools"])
	}
	names := make([]string, 0, len(rawTools))
	for _, item := range rawTools {
		tool, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("tool 条目不是对象: %v", item)
		}
		name, _ := tool["name"].(string)
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) != len(frozenTools) {
		t.Fatalf("tools = %d, want %d: %v", len(names), len(frozenTools), names)
	}
	for index, expected := range frozenTools {
		if names[index] != expected {
			t.Fatalf("tools[%d] = %q, want %q (all=%v)", index, names[index], expected, names)
		}
	}

	// A tool call keeps the same stdout contract.
	if _, err := session.callToolErr("inventory", map[string]any{}); err != nil {
		t.Fatalf("inventory: %v", err)
	}
	waitForFile(t, session.socketPath, 10*time.Second)

	if code := session.shutdown(t); code != EXIT_OK {
		t.Fatalf("exit code = %d, want %d (stderr=%s)", code, EXIT_OK, session.stderr.String())
	}
	session.assertStdoutFrames(t)
	if _, err := os.Stat(session.socketPath); !os.IsNotExist(err) {
		t.Fatalf("socket exists after normal exit: %v", err)
	}
	if strings.Contains(session.stderr.String(), "DATA RACE") {
		t.Fatalf("stderr 存在数据竞争报告: %s", session.stderr.String())
	}
}

// TestE2EStateEndpoint checks /state and /health over the real socket.
func TestE2EStateEndpoint(t *testing.T) {
	dataDir := t.TempDir()
	scriptsDir := t.TempDir()
	prepareMinimalData(t, dataDir)
	process := startProcessWithDirs(t, dataDir, scriptsDir)
	client := process.waitForHealth(t)

	status, payload := mustJSON(t, client, http.MethodGet, "/state?entity=猫猫&path=状态.情绪&path=缺失.路径", nil)
	if status != http.StatusOK {
		t.Fatalf("/state status = %d (%v)", status, payload)
	}
	values, _ := payload["values"].(map[string]any)
	if values["状态.情绪"] != "平静" {
		t.Fatalf("values = %v", values)
	}
	missing, _ := payload["missing"].([]any)
	if len(missing) != 1 || missing[0] != "缺失.路径" {
		t.Fatalf("missing = %v", missing)
	}

	if status, _ := mustJSON(t, client, http.MethodGet, "/state?entity=不存在", nil); status != http.StatusNotFound {
		t.Fatalf("unknown entity status = %d", status)
	}
	if status, _ := mustJSON(t, client, http.MethodPost, "/health", nil); status != http.StatusMethodNotAllowed {
		t.Fatalf("POST /health status = %d", status)
	}
	if status, _ := mustJSON(t, client, http.MethodGet, "/nope", nil); status != http.StatusNotFound {
		t.Fatalf("unknown endpoint status = %d", status)
	}

	if err := process.command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal: %v", err)
	}
	if code := process.wait(t, 15*time.Second); code != EXIT_OK {
		t.Fatalf("exit code = %d (stderr=%s)", code, process.stderr.String())
	}
}
