// clock_test.go covers POST /clock: advance, set, rollover, validation and the
// missing-world-entity contract.
package control

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// createWorld adds the 世界 entity with one in-world timestamp.
func createWorld(t *testing.T, env *testEnv, timestamp string) {
	t.Helper()
	if _, err := env.runtime.Store.Create("世界", "世界", map[string]any{
		"环境": map[string]any{"当前时间": timestamp, "当前月份": 11},
		"状态": map[string]any{"阶段": "傍晚"},
	}, nil); err != nil {
		t.Fatalf("create world: %v", err)
	}
}

// TestClockAdvances covers the minutes form: the timestamp moves, the month and
// phase mirrors are refreshed and the result is persisted.
func TestClockAdvances(t *testing.T) {
	env := newTestEnv(t, Options{})
	createWorld(t, env, "2026-11-07T18:30:00+08:00")
	recorder := env.call(t, http.MethodPost, "/clock", []byte(`{"minutes":60}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	payload := decodeBody(t, recorder)
	if payload["ok"] != true || payload["worldTime"] != "2026-11-07T19:30:00+08:00" {
		t.Fatalf("payload = %v", payload)
	}
	if payload["month"] != float64(11) || payload["phase"] != "傍晚" {
		t.Fatalf("month/phase = %v/%v", payload["month"], payload["phase"])
	}
	world := env.runtime.Store.Get("世界")
	if value, _ := world.Get("环境.当前时间"); value != "2026-11-07T19:30:00+08:00" {
		t.Fatalf("stored time = %v", value)
	}
	if _, err := env.runtime.Store.Get("世界").Get("环境.当前月份"); err != nil {
		t.Fatalf("month mirror missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(env.dataDir, "entities.json")); err != nil {
		t.Fatalf("clock not saved: %v", err)
	}
}

// TestClockCrossesMonth covers rollover: advancing past the last day moves the
// month mirror and the phase label.
func TestClockCrossesMonth(t *testing.T) {
	env := newTestEnv(t, Options{})
	createWorld(t, env, "2026-11-30T23:30:00+08:00")
	payload := decodeBody(t, env.call(t, http.MethodPost, "/clock", []byte(`{"minutes":60}`)))
	if payload["worldTime"] != "2026-12-01T00:30:00+08:00" {
		t.Fatalf("worldTime = %v", payload["worldTime"])
	}
	if payload["month"] != float64(12) || payload["phase"] != "凌晨" {
		t.Fatalf("month/phase = %v/%v", payload["month"], payload["phase"])
	}
}

// TestClockSetsTime covers the explicit time form.
func TestClockSetsTime(t *testing.T) {
	env := newTestEnv(t, Options{})
	createWorld(t, env, "2026-11-07T18:30:00+08:00")
	payload := decodeBody(t, env.call(t, http.MethodPost, "/clock", []byte(`{"time":"2026-12-25T08:15:00+08:00"}`)))
	if payload["worldTime"] != "2026-12-25T08:15:00+08:00" {
		t.Fatalf("worldTime = %v", payload["worldTime"])
	}
	if payload["month"] != float64(12) || payload["phase"] != "清晨" {
		t.Fatalf("month/phase = %v/%v", payload["month"], payload["phase"])
	}
}

// TestClockRejectsBadInput covers the 400 vocabulary and the missing world
// entity's 404.
func TestClockRejectsBadInput(t *testing.T) {
	env := newTestEnv(t, Options{})
	createWorld(t, env, "2026-11-07T18:30:00+08:00")
	for _, body := range []string{
		"", `{oops`, `{}`, `{"minutes":0}`, `{"minutes":1441}`,
		`{"time":"不是时间"}`, `{"minutes":5,"time":"2026-11-07T18:30:00+08:00"}`,
	} {
		if recorder := env.call(t, http.MethodPost, "/clock", []byte(body)); recorder.Code != http.StatusBadRequest {
			t.Fatalf("body %q: status = %d, want 400 (%s)", body, recorder.Code, recorder.Body.String())
		}
	}

	noWorld := newTestEnv(t, Options{})
	recorder := noWorld.call(t, http.MethodPost, "/clock", []byte(`{"minutes":5}`))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("missing world status = %d, want 404", recorder.Code)
	}
	if recorder := noWorld.call(t, http.MethodGet, "/clock", nil); recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /clock status = %d, want 405", recorder.Code)
	}
}

// TestClockRejectsUnparsableTime covers a world entity whose timestamp is not
// RFC3339: advancing reports 400 instead of silently doing nothing.
func TestClockRejectsUnparsableTime(t *testing.T) {
	env := newTestEnv(t, Options{})
	createWorld(t, env, "昨晚")
	if recorder := env.call(t, http.MethodPost, "/clock", []byte(`{"minutes":5}`)); recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", recorder.Code, recorder.Body.String())
	}
}
