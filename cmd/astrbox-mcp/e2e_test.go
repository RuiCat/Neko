package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var serverBinary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "astrbox-e2e")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	serverBinary = filepath.Join(dir, "astrbox-mcp")
	build := exec.Command("go", "build", "-o", serverBinary, ".")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "build e2e server:", err)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func copyRepoDirs(t *testing.T) (string, string) {
	t.Helper()
	dataDir := t.TempDir()
	scriptsDir := t.TempDir()
	if err := os.CopyFS(dataDir, os.DirFS(filepath.Join("..", "..", "data"))); err != nil {
		t.Fatalf("copy data: %v", err)
	}
	if err := os.CopyFS(scriptsDir, os.DirFS(filepath.Join("..", "..", "scripts"))); err != nil {
		t.Fatalf("copy scripts: %v", err)
	}
	return dataDir, scriptsDir
}

func startStdio(t *testing.T, dataDir, scriptsDir string) (*mcp.ClientSession, func()) {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "e2e-client", Version: "1.0.0"}, nil)
	command := exec.Command(serverBinary, "-data-dir", dataDir, "-scripts-dir", scriptsDir)
	var stderr strings.Builder
	command.Stderr = &stderr
	session, err := client.Connect(context.Background(), &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect: %v (%s)", err, stderr.String())
	}
	return session, func() {
		_ = session.Close()
	}
}

func callJSON(t *testing.T, session *mcp.ClientSession, name string, args map[string]any, out any) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if len(result.Content) == 0 {
		t.Fatalf("%s: empty content", name)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if strings.HasPrefix(text, "❌") {
		t.Fatalf("%s failed: %s", name, text)
	}
	if err := json.Unmarshal([]byte(text), out); err != nil {
		t.Fatalf("%s is not json: %v (%s)", name, err, text)
	}
}

func callText(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if len(result.Content) == 0 {
		t.Fatalf("%s: empty content", name)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if strings.HasPrefix(text, "❌") {
		t.Fatalf("%s failed: %s", name, text)
	}
	return text
}

func TestMCPStdioEndToEnd(t *testing.T) {
	dataDir, scriptsDir := copyRepoDirs(t)
	session, closeSession := startStdio(t, dataDir, scriptsDir)
	defer closeSession()
	ctx := context.Background()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools.Tools) != 17 {
		t.Fatalf("tools = %d, want 17", len(tools.Tools))
	}

	var inventory map[string]any
	callJSON(t, session, "inventory", map[string]any{}, &inventory)
	if count, _ := inventory["entity_count"].(float64); count < 50 {
		t.Fatalf("entity_count = %v", inventory["entity_count"])
	}

	var stats map[string]any
	callJSON(t, session, "stats", map[string]any{}, &stats)
	if count, _ := stats["rules"].(float64); count < 1700 {
		t.Fatalf("rules = %v", stats["rules"])
	}

	var state map[string]any
	callJSON(t, session, "entity_state_get", map[string]any{"entity": "猫猫", "path": "身体.体力"}, &state)
	if state["值"] == nil {
		t.Fatalf("体力 missing: %+v", state)
	}
	stamina, _ := state["值"].(float64)
	callText(t, session, "entity_state_set", map[string]any{
		"entity": "猫猫", "path": "身体.体力", "value": fmt.Sprintf("%v", stamina-25),
	})
	var after map[string]any
	callJSON(t, session, "entity_state_get", map[string]any{"entity": "猫猫", "path": "状态.身体"}, &after)
	if after["值"] != "疲惫" {
		t.Fatalf("event handler did not fire: 状态.身体 = %v", after["值"])
	}

	var tick map[string]any
	callJSON(t, session, "state_machine_tick", map[string]any{"scope": "*"}, &tick)
	transitions, _ := tick["transitions"].([]any)
	if len(transitions) == 0 {
		t.Fatalf("tick has no transitions: %+v", tick)
	}

	var snapshot map[string]any
	callJSON(t, session, "entity_snapshot", map[string]any{"name": "e2e_snapshot"}, &snapshot)
	snapshotPath, _ := snapshot["path"].(string)
	if _, err := os.Stat(snapshotPath); err != nil {
		t.Fatalf("snapshot missing: %v", err)
	}

	var eventLog map[string]any
	callJSON(t, session, "event_log", map[string]any{"limit": 5}, &eventLog)
	if _, ok := eventLog["validations"]; !ok {
		t.Fatalf("event_log = %+v", eventLog)
	}

	script := `@Data 测试实体 {
  "值": 1
}
@规则[$[测试实体]] {
  动作: 值 = 2
}`
	var applied map[string]any
	callJSON(t, session, "neko_script_apply", map[string]any{"name": "e2e_script", "script": script}, &applied)
	if ok, _ := applied["ok"].(bool); !ok {
		t.Fatalf("apply = %+v", applied)
	}
	templatePath := filepath.Join(dataDir, "templates", "测试实体.json")
	if _, err := os.Stat(templatePath); err != nil {
		t.Fatalf("template file missing after apply: %v", err)
	}

	var readBack map[string]any
	callJSON(t, session, "neko_script_read", map[string]any{"name": "e2e_script"}, &readBack)
	if readBack["source"] != script {
		t.Fatalf("read source mismatch: %+v", readBack)
	}
	if hash, _ := readBack["hash"].(string); hash == "" {
		t.Fatalf("read hash missing: %+v", readBack)
	}
	var validated map[string]any
	callJSON(t, session, "neko_script_validate", map[string]any{"name": "e2e_script", "script": script}, &validated)
	if ok, _ := validated["ok"].(bool); !ok {
		t.Fatalf("validate = %+v", validated)
	}
	var rejected map[string]any
	callJSON(t, session, "neko_script_write", map[string]any{
		"name": "e2e_script", "script": script, "expected_hash": "wrong",
	}, &rejected)
	if ok, _ := rejected["ok"].(bool); ok {
		t.Fatalf("write with wrong hash succeeded: %+v", rejected)
	}
	var withDiagnostics map[string]any
	callJSON(t, session, "entity_state_get", map[string]any{"entity": "猫猫"}, &withDiagnostics)
	if _, ok := withDiagnostics["diagnostics"]; !ok {
		t.Fatalf("state response has no diagnostics: %+v", withDiagnostics)
	}
	var diagnosticQuery map[string]any
	callJSON(t, session, "diagnostics_query", map[string]any{"limit": 5}, &diagnosticQuery)
	items, _ := diagnosticQuery["items"].([]any)
	if len(items) == 0 {
		t.Fatalf("diagnostics_query returned no items: %+v", diagnosticQuery)
	}

	// Restart: the script must replay and the template file must survive.
	closeSession()
	session2, closeSession2 := startStdio(t, dataDir, scriptsDir)
	defer closeSession2()
	var scripts map[string]any
	callJSON(t, session2, "neko_script_list", map[string]any{}, &scripts)
	found := false
	for _, name := range scripts["scripts"].([]any) {
		if name == "e2e_script" {
			found = true
		}
	}
	if !found {
		t.Fatalf("e2e_script missing after restart: %+v", scripts)
	}
	var removed map[string]any
	callJSON(t, session2, "neko_script_remove", map[string]any{"name": "e2e_script"}, &removed)
	if _, err := os.Stat(templatePath); !os.IsNotExist(err) {
		t.Fatalf("template file not removed: %v", err)
	}
}

func TestMCPHTTPAuth(t *testing.T) {
	dataDir, scriptsDir := copyRepoDirs(t)
	configPath := filepath.Join(dataDir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"http_token":"secret"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()

	command := exec.Command(serverBinary, "-data-dir", dataDir, "-scripts-dir", scriptsDir,
		"-config", configPath, "-transport", "http", "-addr", addr)
	var stderr strings.Builder
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = command.Process.Kill()
		_, _ = command.Process.Wait()
	}()

	url := "http://" + addr + "/mcp"
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Post(url, "application/json", strings.NewReader("{}"))
		if err == nil {
			response.Body.Close()
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	unauthorized, err := http.Post(url, "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("post: %v (%s)", err, stderr.String())
	}
	unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("without token status = %d", unauthorized.StatusCode)
	}

	request, _ := http.NewRequest(http.MethodPost, url, strings.NewReader("{}"))
	request.Header.Set("Authorization", "Bearer secret")
	authorized, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("authorized post: %v", err)
	}
	authorized.Body.Close()
	if authorized.StatusCode == http.StatusUnauthorized {
		t.Fatalf("valid token rejected")
	}
}

func TestHTTPRefusesNonLoopbackWithoutToken(t *testing.T) {
	dataDir, scriptsDir := copyRepoDirs(t)
	command := exec.Command(serverBinary, "-data-dir", dataDir, "-scripts-dir", scriptsDir,
		"-transport", "http", "-addr", "0.0.0.0:0")
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatal("non-loopback HTTP started without token")
	}
	if !strings.Contains(string(output), "拒绝在非回环地址") {
		t.Fatalf("unexpected output: %s", output)
	}
}
