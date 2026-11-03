// Auth matrix and cross-origin guards: token shapes, read-only mode, Host
// allowlist, Origin exact match and the absence of CORS headers.
package monitor

import (
	"net/http"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/RuiCat/Neko/internal/monitor/auth"
)

// pageFS is the minimal embedded page used by the token-free static test.
var pageFS = fstest.MapFS{
	"index.html": &fstest.MapFile{Data: []byte("<html>monitor</html>")},
}

// TestAuthMatrix covers K3/C7: static without token, API token header forms,
// WS subprotocol token, read-only writes disabled and token charset validation.
func TestAuthMatrix(t *testing.T) {
	env := newTestEnv(t, Options{Token: TEST_TOKEN, Version: "test", WebFS: pageFS})

	static := env.request(t, http.MethodGet, "/monitor/", nil, nil)
	if static.Code != http.StatusOK || !strings.Contains(bodyText(static), "monitor") {
		t.Fatalf("静态页 staticecd = %d (%s)", static.Code, bodyText(static))
	}

	anonymous := env.request(t, http.MethodGet, "/monitor/api/health", nil, nil)
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("无 token API status = %d", anonymous.Code)
	}
	if code := failureCode(t, decode(t, anonymous)); code != auth.CODE_UNAUTHORIZED {
		t.Fatalf("无 token code = %s", code)
	}

	headerToken := env.request(t, http.MethodGet, "/monitor/api/health", nil,
		map[string]string{"X-Auth-Token": TEST_TOKEN})
	if headerToken.Code != http.StatusOK {
		t.Fatalf("header token status = %d", headerToken.Code)
	}
	bearer := env.request(t, http.MethodGet, "/monitor/api/health", nil,
		map[string]string{"Authorization": "Bearer " + TEST_TOKEN})
	if bearer.Code != http.StatusOK {
		t.Fatalf("bearer token status = %d", bearer.Code)
	}

	noProtocol := env.request(t, http.MethodGet, "/monitor/ws", nil, nil)
	if noProtocol.Code != http.StatusUnauthorized {
		t.Fatalf("WS 无子协议 status = %d", noProtocol.Code)
	}
	wrongProtocol := env.request(t, http.MethodGet, "/monitor/ws", nil,
		map[string]string{"Sec-WebSocket-Protocol": "astrbox.token.wrong-token"})
	if wrongProtocol.Code != http.StatusUnauthorized {
		t.Fatalf("WS 错误 token status = %d", wrongProtocol.Code)
	}

	readOnly := newTestEnv(t, Options{})
	health := readOnly.request(t, http.MethodGet, "/monitor/api/health", nil, nil)
	monitorShape, _ := decode(t, health)["monitor"].(map[string]any)
	if health.Code != http.StatusOK || monitorShape["readOnly"] != true {
		t.Fatalf("只读 health = %d %v", health.Code, decode(t, health))
	}
	for _, target := range []string{"/monitor/api/tick", "/monitor/api/entities/猫猫/set"} {
		recorder := readOnly.request(t, http.MethodPost, target, []byte(`{}`), nil)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("%s 只读 status = %d", target, recorder.Code)
		}
		if code := failureCode(t, decode(t, recorder)); code != auth.CODE_WRITE_DISABLED {
			t.Fatalf("%s 只读 code = %s", target, code)
		}
	}

	if !auth.ValidTokenCharset(TEST_TOKEN) {
		t.Fatal("合法 token 被判非法")
	}
	for _, bad := range []string{"short", "bad token!", strings.Repeat("a", 129)} {
		if auth.ValidTokenCharset(bad) {
			t.Fatalf("非法 token 被接受: %q", bad)
		}
	}
}

// TestHostAllowlist covers K3/C8 host whitelisting and the no-CORS invariant.
func TestHostAllowlist(t *testing.T) {
	env := newTestEnv(t, Options{})
	allowed := []string{"127.0.0.1:8765", "localhost:8765", "[::1]:8765", "localhost"}
	for _, host := range allowed {
		recorder := env.requestWithHost(t, host, http.MethodGet, "/monitor/api/health", nil, nil)
		if recorder.Code != http.StatusOK {
			t.Fatalf("Host %q status = %d", host, recorder.Code)
		}
	}
	for _, host := range []string{"evil.example:8765", "127.0.0.1.evil:8765", ""} {
		recorder := env.requestWithHost(t, host, http.MethodGet, "/monitor/api/health", nil, nil)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("Host %q status = %d", host, recorder.Code)
		}
	}
	withOrigin := env.request(t, http.MethodGet, "/monitor/api/health", nil,
		map[string]string{"Origin": "http://evil.example"})
	if withOrigin.Code != http.StatusOK {
		t.Fatalf("Origin 不应影响读请求: %d", withOrigin.Code)
	}
	if value := withOrigin.Header().Get("Access-Control-Allow-Origin"); value != "" {
		t.Fatalf("不允许 CORS 头，得到 %q", value)
	}
}

// TestWSOriginRejected covers K2/K3/C8 WebSocket Origin exact matching.
func TestWSOriginRejected(t *testing.T) {
	env := newTestEnv(t, Options{})
	upgrade := map[string]string{
		"Upgrade": "websocket", "Connection": "Upgrade",
		"Sec-WebSocket-Key": "dGhlIHNhbXBsZSBub25jZQ==", "Sec-WebSocket-Version": "13",
	}
	for _, origin := range []string{"http://evil.example", "null", "http://127.0.0.1:8765.evil"} {
		headers := map[string]string{"Origin": origin}
		for key, value := range upgrade {
			headers[key] = value
		}
		recorder := env.request(t, http.MethodGet, "/monitor/ws", nil, headers)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("Origin %q status = %d", origin, recorder.Code)
		}
	}
	headers := map[string]string{"Origin": "http://127.0.0.1:8765"}
	for key, value := range upgrade {
		headers[key] = value
	}
	if recorder := env.request(t, http.MethodGet, "/monitor/ws", nil, headers); recorder.Code == http.StatusForbidden {
		t.Fatalf("合法同源 Origin 被拒绝: %d", recorder.Code)
	}
}
