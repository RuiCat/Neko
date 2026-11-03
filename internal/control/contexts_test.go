// Tests for the context control-plane routes.
package control

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RuiCat/Neko/internal/bootstrap"
)

// newContextTestServer builds a server without a driver so routes answer 503.
func newContextTestServer(t *testing.T) *Server {
	t.Helper()
	return New(&bootstrap.Runtime{DataDir: t.TempDir()}, nil, Options{})
}

func TestContextRoutesDisabled(t *testing.T) {
	server := newContextTestServer(t)
	list := server.dispatch(httptest.NewRequest(http.MethodGet, "/contexts", nil))
	if list.status != http.StatusServiceUnavailable {
		t.Fatalf("GET /contexts status = %d, want 503", list.status)
	}
	invoke := server.dispatch(httptest.NewRequest(http.MethodPost, "/context-invoke",
		strings.NewReader(`{"contextId":"ctx-cat","text":"hi"}`)))
	if invoke.status != http.StatusServiceUnavailable {
		t.Fatalf("POST /context-invoke status = %d, want 503", invoke.status)
	}
	body, ok := invoke.body["error"].(map[string]any)
	if !ok || body["code"] != CODE_CONTEXTS_DISABLED {
		t.Fatalf("error body = %+v", invoke.body)
	}
}

func TestContextRouteMethods(t *testing.T) {
	server := newContextTestServer(t)
	if resp := server.dispatch(httptest.NewRequest(http.MethodPost, "/contexts", nil)); resp.status != http.StatusMethodNotAllowed {
		t.Fatalf("POST /contexts status = %d, want 405", resp.status)
	}
	if resp := server.dispatch(httptest.NewRequest(http.MethodGet, "/context-invoke", nil)); resp.status != http.StatusMethodNotAllowed {
		t.Fatalf("GET /context-invoke status = %d, want 405", resp.status)
	}
}
