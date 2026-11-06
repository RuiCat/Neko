// entities_test.go covers the read-only entity directory: row shape, id order
// and method handling.
package control

import (
	"net/http"
	"testing"
)

// TestEntitiesListsIDAndTypeSorted covers 200 with id and type only, strictly
// sorted by id and carrying no entity data.
func TestEntitiesListsIDAndTypeSorted(t *testing.T) {
	env := newTestEnv(t, Options{})
	for _, id := range []string{"beta", "alpha"} {
		if _, err := env.runtime.Store.Create(id, "NPC", nil, nil); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	recorder := env.call(t, http.MethodGet, "/entities", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	payload := decodeBody(t, recorder)
	raw, ok := payload["entities"].([]any)
	if !ok {
		t.Fatalf("entities = %T, want array", payload["entities"])
	}
	// ASCII ids sort before the multi-byte entity id, so the expected order is
	// unambiguous and independent of the store's internal map order.
	wantIDs := []string{"alpha", "beta", "猫猫"}
	if len(raw) != len(wantIDs) {
		t.Fatalf("entities = %v, want %v", raw, wantIDs)
	}
	for index, wantID := range wantIDs {
		entry, ok := raw[index].(map[string]any)
		if !ok {
			t.Fatalf("entry %d = %T, want object", index, raw[index])
		}
		if entry["id"] != wantID {
			t.Fatalf("entry %d id = %v, want %s", index, entry["id"], wantID)
		}
		if entry["type"] != "NPC" {
			t.Fatalf("entry %d type = %v, want NPC", index, entry["type"])
		}
		if len(entry) != 2 {
			t.Fatalf("entry %d carries more than id/type (data leaked?): %v", index, entry)
		}
	}
}

// TestEntitiesRejectsNonGet covers 405 with the Allow header on the directory.
func TestEntitiesRejectsNonGet(t *testing.T) {
	env := newTestEnv(t, Options{})
	recorder := env.call(t, http.MethodPost, "/entities", nil)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	if allow := recorder.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("allow = %q", allow)
	}
	if code := errorCode(t, decodeBody(t, recorder)); code != CODE_METHOD_NOT_ALLOWED {
		t.Fatalf("code = %s", code)
	}
}
