// httpProvider_test.go pins the request shape, retry policy, budget and key
// handling of the OpenAI-compatible adapter.
package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func newTestProvider(t *testing.T, handler http.HandlerFunc) (*HTTPProvider, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	provider, err := NewHTTPProvider(server.URL, "test-key", "test-model")
	if err != nil {
		t.Fatal(err)
	}
	provider.Client = server.Client()
	provider.RetryBackoff = 0
	return provider, server
}

func TestCompleteParsesResponse(t *testing.T) {
	provider, server := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["model"] != "test-model" {
			t.Errorf("model = %v", body["model"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "test-model",
			"choices": []any{map[string]any{"message": map[string]any{"content": "喵"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 2},
		})
	})
	defer server.Close()
	response, err := provider.Complete(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if response.Content != "喵" || response.Usage.PromptTokens != 3 {
		t.Fatalf("响应错误：%+v", response)
	}
	if provider.Budget.Used() != 0 {
		t.Fatalf("未配置预算不应累计：%d", provider.Budget.Used())
	}
}

func TestRetriesOnServerError(t *testing.T) {
	var calls atomic.Int32
	provider, server := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": "ok"}}},
		})
	})
	defer server.Close()
	response, err := provider.Complete(context.Background(), Request{})
	if err != nil {
		t.Fatal(err)
	}
	if response.Content != "ok" || calls.Load() != 2 {
		t.Fatalf("重试行为错误：content=%q calls=%d", response.Content, calls.Load())
	}
}

func TestNoRetryOnClientError(t *testing.T) {
	var calls atomic.Int32
	provider, server := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	})
	defer server.Close()
	if _, err := provider.Complete(context.Background(), Request{}); err == nil {
		t.Fatal("400 应返回错误")
	}
	if calls.Load() != 1 {
		t.Fatalf("400 不应重试，调用 %d 次", calls.Load())
	}
}

func TestBudgetExceededBeforeCall(t *testing.T) {
	var calls atomic.Int32
	provider, server := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
	})
	defer server.Close()
	provider.Budget = NewBudget(10)
	provider.Budget.Add(10)
	if _, err := provider.Complete(context.Background(), Request{}); err != ErrBudgetExceeded {
		t.Fatalf("预算用尽应拒绝，得到 %v", err)
	}
	if calls.Load() != 0 {
		t.Fatal("预算用尽不应发请求")
	}
}

func TestMissingKeyRejected(t *testing.T) {
	if _, err := NewHTTPProvider("http://localhost", "  ", "m"); err == nil {
		t.Fatal("空 key 应被拒绝")
	}
}

func TestKeyNeverAppearsInError(t *testing.T) {
	provider, server := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	})
	defer server.Close()
	_, err := provider.Complete(context.Background(), Request{})
	if err == nil {
		t.Fatal("应返回错误")
	}
	if strings.Contains(err.Error(), "test-key") {
		t.Fatalf("错误信息泄漏 key: %v", err)
	}
}
