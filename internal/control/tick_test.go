package control

import (
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/ruicat/astrbox/internal/engine"
)

// addTransitionRule installs a rule that always writes one state path so a tick
// produces a transition and therefore attempts a store save.
func (env *testEnv) addTransitionRule(t *testing.T) {
	t.Helper()
	env.runtime.Machine.AddRule(engine.Rule{
		ID: "r_control_test", Match: "NPC[*]", Priority: 1, Enabled: true,
		Branches: []engine.Branch{{Then: []engine.Op{{
			Op: "set", Path: "{e}.状态.控制测试", Value: "已触发",
		}}}},
	})
}

// TestTickSuccess checks the 200 payload shape of /tick.
func TestTickSuccess(t *testing.T) {
	env := newTestEnv(t, Options{})
	recorder := env.call(t, http.MethodPost, "/tick", []byte(`{"scope":""}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	payload := decodeBody(t, recorder)
	if payload["ok"] != true {
		t.Fatalf("ok = %v", payload["ok"])
	}
	if tickID, _ := payload["tickId"].(string); tickID == "" {
		t.Fatalf("tickId = %v", payload["tickId"])
	}
	for _, field := range []string{"transitions", "conflicts", "errors"} {
		if _, ok := payload[field].([]any); !ok {
			t.Fatalf("%s = %v (want array)", field, payload[field])
		}
	}
}

// TestTickUnchangedSkipsSave verifies a no-op tick does not create entities.json.
func TestTickUnchangedSkipsSave(t *testing.T) {
	env := newTestEnv(t, Options{})
	entitiesPath := env.dataDir + "/entities.json"
	recorder := env.call(t, http.MethodPost, "/tick", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	if _, err := os.Stat(entitiesPath); !os.IsNotExist(err) {
		t.Fatalf("unchanged tick wrote entities.json: %v", err)
	}
}

// TestTickBusyImmediate409 holds the shared gate (as an in-flight MCP tick
// would) and expects /tick to refuse immediately instead of queueing. A
// delayed safety release bounds the test: a regression that blocks on Lock()
// returns after the release and fails the elapsed check instead of hanging.
func TestTickBusyImmediate409(t *testing.T) {
	env := newTestEnv(t, Options{})
	release := holdStateGate(t, env)
	safety := time.AfterFunc(time.Second, release)
	defer safety.Stop()

	started := time.Now()
	recorder := env.call(t, http.MethodPost, "/tick", []byte(`{}`))
	elapsed := time.Since(started)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	if code := errorCode(t, decodeBody(t, recorder)); code != CODE_TICK_BUSY {
		t.Fatalf("code = %s", code)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("/tick 阻塞了 %s，应立即 409（排队回归）", elapsed)
	}
	release()

	second := env.call(t, http.MethodPost, "/tick", []byte(`{}`))
	if second.Code != http.StatusOK {
		t.Fatalf("after release status = %d (%s)", second.Code, second.Body.String())
	}
}

// TestTickSaveFailureIsDirty forces a save failure and expects 500 dirty:true.
func TestTickSaveFailureIsDirty(t *testing.T) {
	env := newTestEnv(t, Options{})
	env.addTransitionRule(t)
	if err := os.Chmod(env.dataDir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	defer func() { _ = os.Chmod(env.dataDir, 0o755) }()

	recorder := env.call(t, http.MethodPost, "/tick", []byte(`{}`))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	payload := decodeBody(t, recorder)
	if payload["dirty"] != true {
		t.Fatalf("dirty = %v", payload["dirty"])
	}
	if payload["ok"] != false {
		t.Fatalf("ok = %v", payload["ok"])
	}
	if tickID, _ := payload["tickId"].(string); tickID == "" {
		t.Fatalf("tickId missing on dirty response")
	}
}

// TestShuttingDownReturns503 checks the shutdown gate.
func TestShuttingDownReturns503(t *testing.T) {
	env := newTestEnv(t, Options{})
	env.server.closing.Store(true)
	recorder := env.call(t, http.MethodGet, "/health", nil)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	if code := errorCode(t, decodeBody(t, recorder)); code != CODE_SHUTTING_DOWN {
		t.Fatalf("code = %s", code)
	}
}

// TestTickRejectsOversizedBody checks the 64 KiB request limit.
func TestTickRejectsOversizedBody(t *testing.T) {
	env := newTestEnv(t, Options{})
	body := make([]byte, MAX_BODY_BYTES+1)
	for index := range body {
		body[index] = ' '
	}
	recorder := env.call(t, http.MethodPost, "/tick", body)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
}

// TestTickUsesSharedRunTick verifies the control tick drives the same engine as
// the MCP tool (a transition rule writes the entity state).
func TestTickUsesSharedRunTick(t *testing.T) {
	env := newTestEnv(t, Options{})
	env.addTransitionRule(t)
	recorder := env.call(t, http.MethodPost, "/tick", []byte(`{"scope":"NPC[*]"}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	payload := decodeBody(t, recorder)
	transitions, _ := payload["transitions"].([]any)
	if len(transitions) == 0 {
		t.Fatalf("transitions = %v", payload["transitions"])
	}
	value, err := env.runtime.Store.Get("猫猫").Get("状态.控制测试")
	if err != nil || value != "已触发" {
		t.Fatalf("state = %v (%v)", value, err)
	}
}
