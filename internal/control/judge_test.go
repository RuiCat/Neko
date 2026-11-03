// judge_test.go pins the C3 control endpoint: legal is not queued, uncovered
// is persisted and listed, malformed input is rejected.
package control

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RuiCat/Neko/internal/engine"
)

func postJudge(t *testing.T, env *testEnv, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/judge", bytes.NewReader([]byte(body)))
	recorder := httptest.NewRecorder()
	env.server.ServeHTTP(recorder, request)
	return recorder
}

func TestJudgeQueuesUncovered(t *testing.T) {
	env := newTestEnv(t, Options{})
	recorder := postJudge(t, env, `{"entity_id":"猫猫","path":"状态.信任值","proposed":0.3,"source":"teacher"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("状态码 %d：%s", recorder.Code, recorder.Body.String())
	}
	payload := decodeBody(t, recorder)
	if payload["queued"] != true {
		t.Fatalf("uncovered 应入队：%v", payload)
	}
	if id, _ := payload["candidate_id"].(string); id == "" {
		t.Fatalf("缺少 candidate_id：%v", payload)
	}
	verdict, _ := payload["verdict"].(map[string]any)
	if verdict["status"] != engine.VERDICT_UNCOVERED {
		t.Fatalf("裁决错误：%v", verdict)
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/candidates", nil)
	listRecorder := httptest.NewRecorder()
	env.server.ServeHTTP(listRecorder, listRequest)
	listPayload := decodeBody(t, listRecorder)
	if listPayload["count"] != float64(1) {
		t.Fatalf("候选数 %v", listPayload["count"])
	}
}

func TestJudgeLegalIsNotQueued(t *testing.T) {
	env := newTestEnv(t, Options{})
	env.runtime.Machine.AddRule(engine.Rule{
		ID: "r_mood", Match: "猫猫", Enabled: true,
		Branches: []engine.Branch{{Then: []engine.Op{{Op: "set", Path: "状态.情绪", Value: "平静"}}}},
	})
	recorder := postJudge(t, env, `{"entity_id":"猫猫","path":"状态.情绪","proposed":"平静"}`)
	payload := decodeBody(t, recorder)
	if payload["queued"] != false {
		t.Fatalf("legal 不应入队：%v", payload)
	}
}

func TestJudgeRejectsBadInput(t *testing.T) {
	env := newTestEnv(t, Options{})
	for _, body := range []string{`{}`, `{"entity_id":"猫猫"}`, `{oops`} {
		if recorder := postJudge(t, env, body); recorder.Code != http.StatusBadRequest {
			t.Fatalf("body %q 应 400，得到 %d", body, recorder.Code)
		}
	}
}

func TestResolveCandidateEndpoint(t *testing.T) {
	env := newTestEnv(t, Options{})
	recorder := postJudge(t, env, `{"entity_id":"猫猫","path":"状态.信任值"}`)
	payload := decodeBody(t, recorder)
	id, _ := payload["candidate_id"].(string)
	if id == "" {
		t.Fatal("缺少 candidate_id")
	}
	resolve := func(body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/candidates/resolve", bytes.NewReader([]byte(body)))
		recorder := httptest.NewRecorder()
		env.server.ServeHTTP(recorder, request)
		return recorder
	}
	if response := resolve(`{"id":"` + id + `","status":"approved","note":"人工确认"}`); response.Code != http.StatusOK {
		t.Fatalf("处置状态码 %d：%s", response.Code, response.Body.String())
	}
	listRequest := httptest.NewRequest(http.MethodGet, "/candidates", nil)
	listRecorder := httptest.NewRecorder()
	env.server.ServeHTTP(listRecorder, listRequest)
	if decodeBody(t, listRecorder)["count"] != float64(0) {
		t.Fatal("已处置候选仍在 pending")
	}
	if response := resolve(`{"id":"` + id + `","status":"bad"}`); response.Code != http.StatusBadRequest {
		t.Fatalf("非法状态应 400，得到 %d", response.Code)
	}
	if response := resolve(`{"id":"missing","status":"approved"}`); response.Code != http.StatusNotFound {
		t.Fatalf("未知候选应 404，得到 %d", response.Code)
	}
}
