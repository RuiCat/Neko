// stateWrite_test.go covers POST /state: single and batch writes, mutual
// exclusion, unknown entities, malformed bodies and persistence.
package control

import (
	"net/http"
	"testing"
)

// TestStateWriteSinglePath covers the historical path/value form: 200 with the
// written path and the value persisted for the next tick.
func TestStateWriteSinglePath(t *testing.T) {
	env := newTestEnv(t, Options{})
	recorder := env.call(t, http.MethodPost, "/state", []byte(`{"entity":"猫猫","path":"状态.情绪","value":"开心"}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	payload := decodeBody(t, recorder)
	if payload["ok"] != true || payload["entity"] != "猫猫" {
		t.Fatalf("payload = %v", payload)
	}
	paths, ok := payload["paths"].([]any)
	if !ok || len(paths) != 1 || paths[0] != "状态.情绪" {
		t.Fatalf("paths = %v", payload["paths"])
	}
	value, err := env.runtime.Store.Get("猫猫").Get("状态.情绪")
	if err != nil || value != "开心" {
		t.Fatalf("state = %v (%v)", value, err)
	}
	env.waitForUserEntitiesContain(t, "开心")
}

// TestStateWriteBatchValues covers the values form: several paths in one gated
// write, reported in ascending path order.
func TestStateWriteBatchValues(t *testing.T) {
	env := newTestEnv(t, Options{})
	recorder := env.call(t, http.MethodPost, "/state",
		[]byte(`{"entity":"猫猫","values":{"状态.情绪":"开心","身体.体力":90}}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	payload := decodeBody(t, recorder)
	paths, ok := payload["paths"].([]any)
	if !ok || len(paths) != 2 || paths[0] != "状态.情绪" || paths[1] != "身体.体力" {
		t.Fatalf("paths = %v, want [状态.情绪 身体.体力] (UTF-8 path order)", payload["paths"])
	}
	mood, err := env.runtime.Store.Get("猫猫").Get("状态.情绪")
	if err != nil || mood != "开心" {
		t.Fatalf("情绪 = %v (%v)", mood, err)
	}
	stamina, err := env.runtime.Store.Get("猫猫").Get("身体.体力")
	if err != nil || stamina != float64(90) {
		t.Fatalf("体力 = %v (%v)", stamina, err)
	}
	env.waitForUserEntitiesContain(t, "开心")
}

// TestStateWriteNullValue covers an explicit null write through the batch
// form, which the single form cannot express.
func TestStateWriteNullValue(t *testing.T) {
	env := newTestEnv(t, Options{})
	recorder := env.call(t, http.MethodPost, "/state", []byte(`{"entity":"猫猫","values":{"标记.空":null}}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	if value, err := env.runtime.Store.Get("猫猫").Get("标记.空"); err != nil || value != nil {
		t.Fatalf("标记.空 = %v (%v), want nil", value, err)
	}
}

// TestStateWriteRejectsBadRequests covers the 400/404 vocabulary.
func TestStateWriteRejectsBadRequests(t *testing.T) {
	env := newTestEnv(t, Options{})
	cases := []struct {
		name string
		body string
		code int
	}{
		{"empty body", "", http.StatusBadRequest},
		{"not json", `{oops`, http.StatusBadRequest},
		{"missing entity", `{"path":"状态.情绪","value":"开心"}`, http.StatusBadRequest},
		{"missing write form", `{"entity":"猫猫"}`, http.StatusBadRequest},
		{"path without value", `{"entity":"猫猫","path":"状态.情绪"}`, http.StatusBadRequest},
		{"value without path", `{"entity":"猫猫","value":"开心"}`, http.StatusBadRequest},
		{"both forms", `{"entity":"猫猫","path":"状态.情绪","value":"开心","values":{"状态.情绪":"开心"}}`, http.StatusBadRequest},
		{"unknown entity", `{"entity":"不存在","path":"状态.情绪","value":"开心"}`, http.StatusNotFound},
	}
	for _, testCase := range cases {
		recorder := env.call(t, http.MethodPost, "/state", []byte(testCase.body))
		if recorder.Code != testCase.code {
			t.Fatalf("%s: status = %d, want %d (%s)", testCase.name, recorder.Code, testCase.code, recorder.Body.String())
		}
	}
}

// TestStateRejectsOtherMethods covers the router's multi-method semantics: GET
// stays read-only, POST writes, anything else answers 405 with both allowed.
func TestStateRejectsOtherMethods(t *testing.T) {
	env := newTestEnv(t, Options{})
	recorder := env.call(t, http.MethodPut, "/state", []byte(`{}`))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	if allow := recorder.Header().Get("Allow"); allow != "GET, POST" {
		t.Fatalf("allow = %q, want GET, POST", allow)
	}
}
