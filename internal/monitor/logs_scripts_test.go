// Log-read guards and script-source guards: dated whitelist, symlink refusal,
// bounded complete-line tails and registry.ReadScript only.
package monitor

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/RuiCat/Neko/internal/monitor/api"
	"github.com/RuiCat/Neko/internal/registry"
)

// TestLogsGuards covers K1/C13: whitelisted names only, symlinks refused,
// bounded tail reads, complete lines only and a <=1 MiB response.
func TestLogsGuards(t *testing.T) {
	env := newTestEnv(t, Options{})
	logsDir := filepath.Join(env.dataDir, "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}

	badDate := env.request(t, http.MethodGet, "/monitor/api/logs?kind=state&date=../../etc/passwd", nil, nil)
	if badDate.Code != http.StatusBadRequest {
		t.Fatalf("路径穿越 date status = %d", badDate.Code)
	}
	badKind := env.request(t, http.MethodGet, "/monitor/api/logs?kind=../../etc/passwd", nil, nil)
	if badKind.Code != http.StatusBadRequest {
		t.Fatalf("非法 kind status = %d", badKind.Code)
	}
	missing := env.request(t, http.MethodGet, "/monitor/api/logs?kind=state&date=2099-01-01", nil, nil)
	if missing.Code != http.StatusOK {
		t.Fatalf("无日志文件 status = %d", missing.Code)
	}
	if lines, _ := decode(t, missing)["lines"].([]any); len(lines) != 0 {
		t.Fatalf("无日志文件 lines = %v", lines)
	}

	secret := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(secret, []byte("TOPSECRET"), 0o644); err != nil {
		t.Fatalf("secret: %v", err)
	}
	if err := os.Symlink(secret, filepath.Join(logsDir, "state_2026-10-26.log")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	symlinked := env.request(t, http.MethodGet, "/monitor/api/logs?kind=state&date=2026-10-26", nil, nil)
	if strings.Contains(bodyText(symlinked), "TOPSECRET") {
		t.Fatalf("symlink 泄漏了目标内容: %s", bodyText(symlinked))
	}

	incomplete := `{"tick_id":"a"}` + "\n" + `{"tick_id":"b"}` + "\n" + "INCOMPLETE_TAIL"
	if err := os.WriteFile(filepath.Join(logsDir, "state_2026-10-25.log"), []byte(incomplete), 0o644); err != nil {
		t.Fatalf("write state log: %v", err)
	}
	partial := env.request(t, http.MethodGet, "/monitor/api/logs?kind=state&date=2026-10-25", nil, nil)
	if strings.Contains(bodyText(partial), "INCOMPLETE_TAIL") {
		t.Fatalf("返回了不完整的末行: %s", bodyText(partial))
	}
	if lines, _ := decode(t, partial)["lines"].([]any); len(lines) != 2 {
		t.Fatalf("完整行数 = %d", len(lines))
	}

	var builder strings.Builder
	pad := strings.Repeat("x", 1000)
	for index := 0; index < 2000; index++ {
		builder.WriteString(`{"tick_id":"` + strconv.Itoa(index) + pad + `"}`)
		builder.WriteString("\n")
	}
	if err := os.WriteFile(filepath.Join(logsDir, "state_2026-10-24.log"), []byte(builder.String()), 0o644); err != nil {
		t.Fatalf("write big log: %v", err)
	}
	big := env.request(t, http.MethodGet, "/monitor/api/logs?kind=state&date=2026-10-24&limit=2000", nil, nil)
	if big.Code != http.StatusOK {
		t.Fatalf("大日志 status = %d", big.Code)
	}
	if !json.Valid(big.Body.Bytes()) {
		t.Fatal("大日志响应不是合法 JSON")
	}
	if big.Body.Len() > api.MAX_LOG_RESPONSE_BYTES+512 {
		t.Fatalf("大日志响应 %d 字节超过上限", big.Body.Len())
	}
	if decode(t, big)["truncated"] != true {
		t.Fatalf("大日志 truncated = %v", decode(t, big)["truncated"])
	}

	controlLine := "2026-10-23T10:00:00Z hello\n"
	if err := os.WriteFile(filepath.Join(logsDir, "dsh-control-20261023.log"), []byte(controlLine), 0o644); err != nil {
		t.Fatalf("write control log: %v", err)
	}
	controlLog := env.request(t, http.MethodGet, "/monitor/api/logs?kind=control&date=2026-10-23", nil, nil)
	lines, _ := decode(t, controlLog)["lines"].([]any)
	if len(lines) != 1 {
		t.Fatalf("control lines = %v", lines)
	}
	first, _ := lines[0].(map[string]any)
	if first["ts"] != "2026-10-23T10:00:00Z" || first["raw"] != "2026-10-23T10:00:00Z hello" {
		t.Fatalf("control line = %v", first)
	}
}

// TestScriptSourceGuards covers K1/C16: reads go through registry.ReadScript,
// which refuses symlinks (no target content) and oversized sources (413).
func TestScriptSourceGuards(t *testing.T) {
	env := newTestEnv(t, Options{})
	const source = "@定义 NPC {\n  \"身体\": {\"体力\": 75}\n}\n"
	if result := env.runtime.Registry.Apply("base", source, false, true); !result.OK {
		t.Fatalf("apply script: %+v", result.Errors)
	}
	list := env.request(t, http.MethodGet, "/monitor/api/scripts", nil, nil)
	if list.Code != http.StatusOK {
		t.Fatalf("scripts status = %d", list.Code)
	}
	items, _ := decode(t, list)["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("scripts items = %v", items)
	}
	withSource := env.request(t, http.MethodGet, "/monitor/api/scripts/base?source=1", nil, nil)
	if withSource.Code != http.StatusOK {
		t.Fatalf("script source status = %d", withSource.Code)
	}
	if payload := decode(t, withSource); payload["source"] != source {
		t.Fatalf("source 不一致: %v", payload["source"])
	}
	withoutSource := env.request(t, http.MethodGet, "/monitor/api/scripts/base", nil, nil)
	if _, ok := decode(t, withoutSource)["source"]; ok {
		t.Fatal("未请求 source 时返回了源码")
	}
	unknown := env.request(t, http.MethodGet, "/monitor/api/scripts/nope", nil, nil)
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("未知脚本 status = %d", unknown.Code)
	}

	scriptPath := filepath.Join(env.scriptsDir, "base.neko")
	if err := os.Remove(scriptPath); err != nil {
		t.Fatalf("remove script: %v", err)
	}
	secret := filepath.Join(t.TempDir(), "secret.neko")
	if err := os.WriteFile(secret, []byte("TOPSECRET_SCRIPT"), 0o644); err != nil {
		t.Fatalf("secret: %v", err)
	}
	if err := os.Symlink(secret, scriptPath); err != nil {
		t.Fatalf("symlink script: %v", err)
	}
	symlinked := env.request(t, http.MethodGet, "/monitor/api/scripts/base?source=1", nil, nil)
	if symlinked.Code < 400 || symlinked.Code >= 500 {
		t.Fatalf("symlink 脚本 status = %d", symlinked.Code)
	}
	if strings.Contains(bodyText(symlinked), "TOPSECRET_SCRIPT") {
		t.Fatal("symlink 脚本泄漏了目标内容")
	}

	if err := os.Remove(scriptPath); err != nil {
		t.Fatalf("remove symlink: %v", err)
	}
	oversize := bytes.Repeat([]byte("x"), registry.MAX_SCRIPT_BYTES+1)
	if err := os.WriteFile(scriptPath, oversize, 0o644); err != nil {
		t.Fatalf("write oversize: %v", err)
	}
	tooLarge := env.request(t, http.MethodGet, "/monitor/api/scripts/base?source=1", nil, nil)
	if tooLarge.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("超大脚本 status = %d", tooLarge.Code)
	}
	if code := failureCode(t, decode(t, tooLarge)); code != api.CODE_TOO_LARGE {
		t.Fatalf("超大脚本 code = %s", code)
	}
}
