// Write-path contract tests: bounded gate waits, save-failure rollback/dirty and
// the write audit log (daily rotation, seven-day prune, no token leakage).
package monitor

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// authed returns the token header map for token-shaped tests.
func authed() map[string]string {
	return map[string]string{"X-Auth-Token": TEST_TOKEN}
}

// TestSetSaveFailureRollback covers K1/C5: a failed Save answers 500 with either
// a rolled-back memory value or dirty:true, never saved:true.
func TestSetSaveFailureRollback(t *testing.T) {
	env := newTestEnv(t, Options{Token: TEST_TOKEN})
	entity := env.runtime.Store.Get("猫猫")
	before, err := entity.Get("状态.情绪")
	if err != nil {
		t.Fatalf("get before: %v", err)
	}
	userDir := filepath.Join(env.dataDir, "user")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatalf("mkdir user: %v", err)
	}
	if err := os.Mkdir(filepath.Join(userDir, "entities.json"), 0o755); err != nil {
		t.Fatalf("block user entities file: %v", err)
	}

	recorder := env.request(t, http.MethodPost, "/monitor/api/entities/猫猫/set",
		[]byte(`{"path":"状态.情绪","value":"愤怒"}`), authed())
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("保存失败 status = %d (%s)", recorder.Code, bodyText(recorder))
	}
	payload := decode(t, recorder)
	if payload["saved"] == true {
		t.Fatalf("保存失败却返回 saved:true: %v", payload)
	}
	after, err := entity.Get("状态.情绪")
	if err != nil {
		t.Fatalf("get after: %v", err)
	}
	if after != before && payload["dirty"] != true {
		t.Fatalf("既未回滚也未标记 dirty: before=%v after=%v payload=%v", before, after, payload)
	}
}

// TestGateBusyBounded covers K1/C6: a held state gate answers 409 for tick
// immediately and for set within the 3s bound.
func TestGateBusyBounded(t *testing.T) {
	env := newTestEnv(t, Options{Token: TEST_TOKEN})
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_ = env.mcp.WithStateWrite(func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	var once sync.Once
	releaseGate := func() {
		once.Do(func() {
			close(release)
			<-finished
		})
	}
	t.Cleanup(releaseGate)

	started := time.Now()
	tickRecorder := env.request(t, http.MethodPost, "/monitor/api/tick", []byte(`{}`), authed())
	if tickRecorder.Code != http.StatusConflict {
		t.Fatalf("tick 忙 status = %d", tickRecorder.Code)
	}
	if code := failureCode(t, decode(t, tickRecorder)); code != "tick_busy" {
		t.Fatalf("tick 忙 code = %s", code)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("tick 忙未在有界时间内返回: %s", elapsed)
	}

	started = time.Now()
	setRecorder := env.request(t, http.MethodPost, "/monitor/api/entities/猫猫/set",
		[]byte(`{"path":"备注","value":"blocked"}`), authed())
	elapsed := time.Since(started)
	if setRecorder.Code != http.StatusConflict || elapsed > 3*time.Second {
		t.Fatalf("set 忙 status=%d elapsed=%s", setRecorder.Code, elapsed)
	}
	if code := failureCode(t, decode(t, setRecorder)); code != "write_busy" {
		t.Fatalf("set 忙 code = %s", code)
	}
}

// TestAuditLog covers K3/C23: one line per write, no token, daily file names,
// seven-day prune and a logging failure that does not fail the write.
func TestAuditLog(t *testing.T) {
	env := newTestEnv(t, Options{Token: TEST_TOKEN})
	logsDir := filepath.Join(env.dataDir, "logs")

	recorder := env.request(t, http.MethodPost, "/monitor/api/entities/猫猫/set",
		[]byte(`{"path":"备注","value":"audited"}`), authed())
	if recorder.Code != http.StatusOK {
		t.Fatalf("写入 status = %d (%s)", recorder.Code, bodyText(recorder))
	}
	today := filepath.Join(logsDir, "monitor-audit-"+time.Now().Format("20060102")+".log")
	raw, err := os.ReadFile(today)
	if err != nil {
		t.Fatalf("读取审计日志: %v", err)
	}
	text := string(raw)
	for _, needle := range []string{"action=set", "entity=猫猫", "path=备注", "result=ok"} {
		if !strings.Contains(text, needle) {
			t.Fatalf("审计行缺少 %q: %s", needle, text)
		}
	}
	if strings.Contains(text, TEST_TOKEN) {
		t.Fatal("审计日志泄漏了 token")
	}

	staleDate := time.Now().AddDate(0, 0, -8).Format("20060102")
	stalePath := filepath.Join(logsDir, "monitor-audit-"+staleDate+".log")
	if err := os.WriteFile(stalePath, []byte("old\n"), 0o644); err != nil {
		t.Fatalf("写入过期审计文件: %v", err)
	}
	if recorder := env.request(t, http.MethodPost, "/monitor/api/entities/猫猫/set",
		[]byte(`{"path":"备注","value":"prune"}`), authed()); recorder.Code != http.StatusOK {
		t.Fatalf("触发 prune 的写入 status = %d", recorder.Code)
	}
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatalf("超过 7 天的审计文件未被清理: %v", err)
	}

	if err := os.RemoveAll(logsDir); err != nil {
		t.Fatalf("remove logs dir: %v", err)
	}
	if err := os.WriteFile(logsDir, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("block logs dir: %v", err)
	}
	unwritable := env.request(t, http.MethodPost, "/monitor/api/entities/猫猫/set",
		[]byte(`{"path":"备注","value":"unwritable-audit"}`), authed())
	if unwritable.Code != http.StatusOK {
		t.Fatalf("审计不可写时业务失败: %d (%s)", unwritable.Code, bodyText(unwritable))
	}
}
