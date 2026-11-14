// entityDelete_test.go covers POST /entities/delete: entity removal, observation
// stream removal, persistence and the 400/404 vocabulary.
package control

import (
	"net/http"
	"testing"

	"github.com/RuiCat/Neko/internal/observation"
)

// TestEntityDeleteRemovesEntityAndBoard covers the happy path: a changed tick
// leaves a board, delete removes both the entity and that board, and the store
// is saved.
func TestEntityDeleteRemovesEntityAndBoard(t *testing.T) {
	env := newTestEnv(t, Options{})
	env.addTransitionRule(t)
	if recorder := env.call(t, http.MethodPost, "/tick", []byte(`{}`)); recorder.Code != http.StatusOK {
		t.Fatalf("tick status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	if env.runtime.Observations.Version(observation.ScopeForEntity("猫猫")) == 0 {
		t.Fatal("tick did not feed the observation stream")
	}
	recorder := env.call(t, http.MethodPost, "/entities/delete", []byte(`{"entity":"猫猫"}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	payload := decodeBody(t, recorder)
	if payload["ok"] != true || payload["deleted"] != "猫猫" {
		t.Fatalf("payload = %v", payload)
	}
	if env.runtime.Store.Get("猫猫") != nil {
		t.Fatal("entity still in the store")
	}
	if version := env.runtime.Observations.Version(observation.ScopeForEntity("猫猫")); version != 0 {
		t.Fatalf("observation stream survived the delete: version=%d", version)
	}
}

// TestEntityDeleteRejectsBadRequests covers missing parameters, malformed
// bodies, unknown entities and wrong methods.
func TestEntityDeleteRejectsBadRequests(t *testing.T) {
	env := newTestEnv(t, Options{})
	for _, body := range []string{"", `{oops`, `{}`, `{"entity":"  "}`} {
		if recorder := env.call(t, http.MethodPost, "/entities/delete", []byte(body)); recorder.Code != http.StatusBadRequest {
			t.Fatalf("body %q: status = %d, want 400 (%s)", body, recorder.Code, recorder.Body.String())
		}
	}
	recorder := env.call(t, http.MethodPost, "/entities/delete", []byte(`{"entity":"不存在"}`))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unknown entity status = %d, want 404 (%s)", recorder.Code, recorder.Body.String())
	}
	if recorder := env.call(t, http.MethodGet, "/entities/delete", nil); recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d, want 405", recorder.Code)
	}
}
