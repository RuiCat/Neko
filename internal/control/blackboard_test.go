// blackboard_test.go covers the read-only K3 board routes: list, one entity,
// empty results, ordering and method handling.
package control

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RuiCat/Neko/internal/eventboard"
)

// TestBlackboardListsBoards covers GET /blackboard before and after a changed
// tick: an empty list first, then one K1 board object.
func TestBlackboardListsBoards(t *testing.T) {
	env := newTestEnv(t, Options{})
	payload := decodeBody(t, env.call(t, http.MethodGet, "/blackboard", nil))
	if payload["ok"] != true {
		t.Fatalf("ok = %v", payload["ok"])
	}
	if boards, ok := payload["boards"].([]any); !ok || len(boards) != 0 {
		t.Fatalf("boards before any tick = %v, want []", payload["boards"])
	}

	env.addTransitionRule(t)
	if recorder := env.call(t, http.MethodPost, "/tick", []byte(`{}`)); recorder.Code != http.StatusOK {
		t.Fatalf("tick status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	recorder := env.call(t, http.MethodGet, "/blackboard", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	payload = decodeBody(t, recorder)
	boards, ok := payload["boards"].([]any)
	if !ok || len(boards) != 1 {
		t.Fatalf("boards = %v, want one", payload["boards"])
	}
	board, ok := boards[0].(map[string]any)
	if !ok {
		t.Fatalf("board = %T, want object", boards[0])
	}
	if board["entity"] != "猫猫" {
		t.Fatalf("entity = %v, want 猫猫", board["entity"])
	}
	if version, _ := board["version"].(float64); version < 1 {
		t.Fatalf("version = %v, want >= 1", board["version"])
	}
	if tickID, _ := board["tick_id"].(string); tickID == "" {
		t.Fatalf("tick_id = %v, want the finished tick", board["tick_id"])
	}
	entries, ok := board["entries"].([]any)
	if !ok || len(entries) != 1 {
		t.Fatalf("entries = %v, want one", board["entries"])
	}
	entry, ok := entries[0].(map[string]any)
	if !ok || entry["path"] != "状态.控制测试" || entry["to"] != "已触发" {
		t.Fatalf("entry = %v", entries[0])
	}
}

// TestBlackboardServesOneEntity covers the entity query: a known board, then a
// missing entity answered as board:null with 200 (never 404).
func TestBlackboardServesOneEntity(t *testing.T) {
	env := newTestEnv(t, Options{})
	env.addTransitionRule(t)
	env.call(t, http.MethodPost, "/tick", []byte(`{}`))

	payload := decodeBody(t, env.call(t, http.MethodGet, "/blackboard?entity=猫猫", nil))
	board, ok := payload["board"].(map[string]any)
	if !ok || board["entity"] != "猫猫" {
		t.Fatalf("board = %v, want the 猫猫 board", payload["board"])
	}

	recorder := env.call(t, http.MethodGet, "/blackboard?entity=不存在", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("missing entity status = %d, want 200", recorder.Code)
	}
	payload = decodeBody(t, recorder)
	if payload["ok"] != true {
		t.Fatalf("ok = %v", payload["ok"])
	}
	if board, exists := payload["board"]; !exists || board != nil {
		t.Fatalf("missing entity board = %v, want null", payload["board"])
	}
}

// TestBlackboardSortedByEntity covers the K3 ordering: boards ascend by entity.
func TestBlackboardSortedByEntity(t *testing.T) {
	env := newTestEnv(t, Options{})
	env.runtime.EventBoards.Observe([]string{"beta", "alpha"})
	payload := decodeBody(t, env.call(t, http.MethodGet, "/blackboard", nil))
	boards, ok := payload["boards"].([]any)
	if !ok || len(boards) != 2 {
		t.Fatalf("boards = %v, want two", payload["boards"])
	}
	want := []string{"alpha", "beta"}
	for index, entity := range want {
		board := boards[index].(map[string]any)
		if board["entity"] != entity {
			t.Fatalf("boards[%d] = %v, want %s", index, board["entity"], entity)
		}
	}
}

// TestBlackboardRejectsNonGet covers 405 with the Allow header.
func TestBlackboardRejectsNonGet(t *testing.T) {
	env := newTestEnv(t, Options{})
	recorder := env.call(t, http.MethodPost, "/blackboard", []byte(`{}`))
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

// TestBlackboardWithoutRegistryAnswersEmpty covers a runtime assembled without
// a board registry: both routes answer an empty result instead of failing.
func TestBlackboardWithoutRegistryAnswersEmpty(t *testing.T) {
	server := &Server{}
	list := server.handleBlackboard(httptest.NewRequest(http.MethodGet, "/blackboard", nil))
	if list.status != http.StatusOK {
		t.Fatalf("list status = %d", list.status)
	}
	boards, ok := list.body["boards"].([]*eventboard.Board)
	if !ok || len(boards) != 0 {
		t.Fatalf("boards = %v, want empty", list.body["boards"])
	}
	one := server.handleBlackboard(httptest.NewRequest(http.MethodGet, "/blackboard?entity=梦梦", nil))
	if one.status != http.StatusOK || one.body["board"] != nil {
		t.Fatalf("one = %v/%v, want 200/null", one.status, one.body["board"])
	}
}
