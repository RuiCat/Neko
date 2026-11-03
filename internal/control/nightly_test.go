// nightly_test.go pins the decay endpoint: factor validation and dropped
// entries reported per expert.
package control

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/RuiCat/Neko/internal/expert"
	"github.com/RuiCat/Neko/internal/gate"
)

func postNightly(t *testing.T, env *testEnv, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/nightly", bytes.NewReader([]byte(body)))
	recorder := httptest.NewRecorder()
	env.server.ServeHTTP(recorder, request)
	return recorder
}

func TestNightlyDecaysAndReports(t *testing.T) {
	env := newTestEnv(t, Options{})
	env.runtime.Experts.Remember(gate.EXPERT_EVENT, expert.Entry{SnapshotID: "evt_1", Content: "旧事", Weight: 0.06})
	env.runtime.Experts.Remember(gate.EXPERT_EVENT, expert.Entry{SnapshotID: "evt_2", Content: "新事", Weight: 0.8})
	recorder := postNightly(t, env, `{"factor":0.5}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("状态码 %d：%s", recorder.Code, recorder.Body.String())
	}
	payload := decodeBody(t, recorder)
	perExpert, ok := payload["per_expert"].(map[string]any)
	if !ok {
		t.Fatalf("缺少 per_expert：%v", payload)
	}
	if perExpert[gate.EXPERT_EVENT] != float64(1) {
		t.Fatalf("事件专家应移除 1 条：%v", perExpert)
	}
	if payload["removed"] != float64(1) {
		t.Fatalf("总移除数 %v", payload["removed"])
	}
	if _, err := os.Stat(filepath.Join(env.runtime.DataDir, "experts.json")); err != nil {
		t.Fatalf("衰减结果应落盘：%v", err)
	}
}

func TestNightlyRejectsBadFactor(t *testing.T) {
	env := newTestEnv(t, Options{})
	for _, body := range []string{`{"factor":1}`, `{"factor":0}`, `{"factor":-0.2}`, `{"factor":1.5}`} {
		if recorder := postNightly(t, env, body); recorder.Code != http.StatusBadRequest {
			t.Fatalf("factor %s 应 400，得到 %d", body, recorder.Code)
		}
	}
}

func TestNightlyDefaultsToHalf(t *testing.T) {
	env := newTestEnv(t, Options{})
	recorder := postNightly(t, env, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("空 body 应使用默认 factor，得到 %d", recorder.Code)
	}
	payload := decodeBody(t, recorder)
	if payload["factor"] != 0.5 {
		t.Fatalf("默认 factor %v", payload["factor"])
	}
}
