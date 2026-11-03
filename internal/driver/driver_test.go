// Tests for the context driver against a fake DSH HTTP surface.
package driver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// promptRecord is one prompt the fake DSH accepted.
type promptRecord struct {
	RequestID string
	SessionID string
	Mode      string
	Text      string
}

// fakeDSH serves /connect and the four session Remote endpoints.
type fakeDSH struct {
	t                *testing.T
	mu               sync.Mutex
	connectCount     int
	createCount      int
	prompts          []promptRecord
	createSessionID  string
	listItems        []SessionSummary
	failPromptCode   string
	unauthorizedOnce bool
	server           *httptest.Server
}

// newFakeDSH starts the fake server and registers its cleanup.
func newFakeDSH(t *testing.T) *fakeDSH {
	t.Helper()
	fake := &fakeDSH{t: t, createSessionID: "dsh-session"}
	mux := http.NewServeMux()
	mux.HandleFunc("/connect", fake.handleConnect)
	mux.HandleFunc("/api/", fake.handleAPI)
	fake.server = httptest.NewServer(mux)
	t.Cleanup(fake.server.Close)
	return fake
}

// handleConnect exchanges the launch token for the signed cookie.
func (f *fakeDSH) handleConnect(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("token") != "test-token" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	f.mu.Lock()
	f.connectCount++
	f.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "dsh-auth-test", Value: "ok", Path: "/"})
	w.Header().Set("Location", "./api/remote.mux")
	w.WriteHeader(http.StatusFound)
}

// handleAPI dispatches the four Remote endpoints used by the driver.
func (f *fakeDSH) handleAPI(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("dsh-auth-test"); err != nil || cookie.Value != "ok" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	f.mu.Lock()
	unauthorized := f.unauthorizedOnce
	f.unauthorizedOnce = false
	f.mu.Unlock()
	if unauthorized {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		f.t.Errorf("读取请求体: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	request := struct {
		RPCID   string          `json:"rpcId"`
		Method  string          `json:"method"`
		Payload json.RawMessage `json:"payload"`
	}{}
	if err := json.Unmarshal(body, &request); err != nil {
		f.t.Errorf("解析请求: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// The DSH Gateway wraps business arguments in exactly one `args` field,
	// and each Remote descriptor names its business parameter (`request`).
	envelope := struct {
		Args json.RawMessage `json:"args"`
	}{}
	_ = json.Unmarshal(request.Payload, &envelope)
	named := struct {
		Request json.RawMessage `json:"request"`
	}{}
	_ = json.Unmarshal(envelope.Args, &named)
	switch request.Method {
	case "session/create":
		f.mu.Lock()
		f.createCount++
		f.mu.Unlock()
		f.writeValue(w, request.RPCID, map[string]string{"sessionId": f.createSessionID})
	case "session/prompt":
		payload := struct {
			RequestID string `json:"requestId"`
			SessionID string `json:"sessionId"`
			Mode      string `json:"mode"`
			Content   []struct {
				Text string `json:"text"`
			} `json:"content"`
		}{}
		_ = json.Unmarshal(named.Request, &payload)
		text := ""
		for _, part := range payload.Content {
			text += part.Text
		}
		f.mu.Lock()
		code := f.failPromptCode
		f.mu.Unlock()
		if code != "" {
			f.writeError(w, request.RPCID, code, "rejected")
			return
		}
		f.mu.Lock()
		f.prompts = append(f.prompts, promptRecord{
			RequestID: payload.RequestID, SessionID: payload.SessionID, Mode: payload.Mode, Text: text,
		})
		f.mu.Unlock()
		f.writeValue(w, request.RPCID, map[string]bool{"accepted": true})
	case "session/cancel":
		f.writeValue(w, request.RPCID, map[string]bool{"accepted": true})
	case "session/list":
		f.mu.Lock()
		items := append([]SessionSummary(nil), f.listItems...)
		f.mu.Unlock()
		f.writeValue(w, request.RPCID, map[string]any{"items": items})
	default:
		f.writeError(w, request.RPCID, "gateway/not-found", "unknown endpoint")
	}
}

// writeValue answers one successful reply envelope.
func (f *fakeDSH) writeValue(w http.ResponseWriter, rpcID string, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "server-response", "rpcId": rpcID,
		"result": map[string]any{"ok": true, "value": value},
	})
}

// writeError answers one failed reply envelope.
func (f *fakeDSH) writeError(w http.ResponseWriter, rpcID, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "server-response", "rpcId": rpcID,
		"result": map[string]any{"ok": false, "error": map[string]any{"code": code, "message": message}},
	})
}

// lastPrompt returns the most recent accepted prompt.
func (f *fakeDSH) lastPrompt() promptRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.prompts) == 0 {
		return promptRecord{}
	}
	return f.prompts[len(f.prompts)-1]
}

// connectTotal returns how many token exchanges happened.
func (f *fakeDSH) connectTotal() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.connectCount
}

// createTotal returns how many session/create calls happened.
func (f *fakeDSH) createTotal() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.createCount
}

// testConfig builds a valid config pointing at the fake server.
func testConfig(t *testing.T, fake *fakeDSH) Config {
	t.Helper()
	dir := t.TempDir()
	return Config{
		DshURL:       fake.server.URL,
		DshToken:     "test-token",
		RegistryPath: filepath.Join(dir, "contexts.json"),
		Contexts: []Spec{{
			ContextID:   "ctx-cat",
			CharacterID: "猫猫",
			SessionID:   "neko-cat",
			PresetID:    "standard",
			Cwd:         dir,
			Entity:      "猫猫",
		}},
	}
}

// newTestDriver builds a driver over the fake server.
func newTestDriver(t *testing.T, fake *fakeDSH) (*Driver, Config) {
	t.Helper()
	cfg := testConfig(t, fake)
	contextDriver, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return contextDriver, cfg
}

func TestOpenAdoptsReturnedSession(t *testing.T) {
	fake := newFakeDSH(t)
	fake.createSessionID = "dsh-adopted"
	contextDriver, cfg := newTestDriver(t, fake)
	record, err := contextDriver.Open(context.Background(), "ctx-cat")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if record.SessionID != "dsh-adopted" {
		t.Fatalf("SessionID = %q, want dsh-adopted", record.SessionID)
	}
	if record.Status != STATUS_IDLE {
		t.Fatalf("Status = %q, want idle", record.Status)
	}
	if _, err := os.Stat(cfg.RegistryPath); err != nil {
		t.Fatalf("registry not persisted: %v", err)
	}
}

func TestInvokeQueueThenSteer(t *testing.T) {
	fake := newFakeDSH(t)
	contextDriver, _ := newTestDriver(t, fake)
	ctx := context.Background()
	first, err := contextDriver.Invoke(ctx, InvokeRequest{ContextID: "ctx-cat", EventID: "evt-1", Text: "第一次"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if first.Mode != MODE_QUEUE {
		t.Fatalf("first mode = %q, want queue", first.Mode)
	}
	if first.RequestID != "neko:ctx-cat:evt-1" {
		t.Fatalf("requestId = %q", first.RequestID)
	}
	second, err := contextDriver.Invoke(ctx, InvokeRequest{
		ContextID: "ctx-cat", EventID: "evt-2", Text: "第二次", Priority: PRIORITY_URGENT,
	})
	if err != nil {
		t.Fatalf("Invoke urgent: %v", err)
	}
	if second.Mode != MODE_STEER {
		t.Fatalf("second mode = %q, want steer", second.Mode)
	}
	last := fake.lastPrompt()
	if last.Mode != MODE_STEER || last.RequestID != "neko:ctx-cat:evt-2" {
		t.Fatalf("last prompt = %+v", last)
	}
	if last.SessionID != "dsh-session" {
		t.Fatalf("prompt session = %q, want dsh-session", last.SessionID)
	}
}

func TestTurnEndResetsIdle(t *testing.T) {
	fake := newFakeDSH(t)
	contextDriver, _ := newTestDriver(t, fake)
	ctx := context.Background()
	if _, err := contextDriver.Invoke(ctx, InvokeRequest{ContextID: "ctx-cat", EventID: "evt-1", Text: "a"}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if err := contextDriver.HandleTurnEnd("dsh-session", 3); err != nil {
		t.Fatalf("HandleTurnEnd: %v", err)
	}
	record, ok := contextDriver.registry.Get("ctx-cat")
	if !ok || record.Status != STATUS_IDLE || record.LastTurn != 3 {
		t.Fatalf("record = %+v", record)
	}
	result, err := contextDriver.Invoke(ctx, InvokeRequest{ContextID: "ctx-cat", EventID: "evt-2", Text: "b"})
	if err != nil {
		t.Fatalf("Invoke after turn end: %v", err)
	}
	if result.Mode != MODE_QUEUE {
		t.Fatalf("mode after idle = %q, want queue", result.Mode)
	}
}

func TestInvokeUnknownContext(t *testing.T) {
	fake := newFakeDSH(t)
	contextDriver, _ := newTestDriver(t, fake)
	_, err := contextDriver.Invoke(context.Background(), InvokeRequest{ContextID: "nope", Text: "x"})
	var requestError *RequestError
	if !errors.As(err, &requestError) {
		t.Fatalf("err = %v, want RequestError", err)
	}
}

func TestInvokeRejectsEmptyText(t *testing.T) {
	fake := newFakeDSH(t)
	contextDriver, _ := newTestDriver(t, fake)
	_, err := contextDriver.Invoke(context.Background(), InvokeRequest{ContextID: "ctx-cat", Text: "   "})
	var requestError *RequestError
	if !errors.As(err, &requestError) {
		t.Fatalf("err = %v, want RequestError", err)
	}
}

func TestPromptBusinessError(t *testing.T) {
	fake := newFakeDSH(t)
	fake.failPromptCode = "session/agent-busy"
	contextDriver, _ := newTestDriver(t, fake)
	_, err := contextDriver.Invoke(context.Background(), InvokeRequest{ContextID: "ctx-cat", Text: "a"})
	var apiError *APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("err = %v, want APIError", err)
	}
	if !apiError.IsCode("session/agent-busy") {
		t.Fatalf("code = %q", apiError.Code)
	}
}

func TestUnauthorizedRefreshesCookie(t *testing.T) {
	fake := newFakeDSH(t)
	fake.unauthorizedOnce = true
	contextDriver, _ := newTestDriver(t, fake)
	if _, err := contextDriver.Invoke(context.Background(), InvokeRequest{ContextID: "ctx-cat", Text: "a"}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := fake.connectTotal(); got != 2 {
		t.Fatalf("connect count = %d, want 2", got)
	}
}

func TestRegistryPersistsAcrossDrivers(t *testing.T) {
	fake := newFakeDSH(t)
	first, cfg := newTestDriver(t, fake)
	if _, err := first.Invoke(context.Background(), InvokeRequest{ContextID: "ctx-cat", EventID: "e", Text: "a"}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if err := first.HandleTurnEnd("dsh-session", 7); err != nil {
		t.Fatalf("HandleTurnEnd: %v", err)
	}
	second, err := New(cfg)
	if err != nil {
		t.Fatalf("New reload: %v", err)
	}
	record, ok := second.registry.Get("ctx-cat")
	if !ok {
		t.Fatal("record missing after reload")
	}
	if record.Status != STATUS_IDLE || record.LastTurn != 7 || record.SessionID != "dsh-session" {
		t.Fatalf("record after reload = %+v", record)
	}
}

func TestRestartResetsStaleRunning(t *testing.T) {
	fake := newFakeDSH(t)
	first, cfg := newTestDriver(t, fake)
	if _, err := first.Invoke(context.Background(), InvokeRequest{ContextID: "ctx-cat", EventID: "e", Text: "a"}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	second, err := New(cfg)
	if err != nil {
		t.Fatalf("New reload: %v", err)
	}
	record, err := second.Open(context.Background(), "ctx-cat")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if record.Status != STATUS_IDLE {
		t.Fatalf("status after restart = %q, want idle", record.Status)
	}
}

func TestOpenSkipsCreateForPersistedSession(t *testing.T) {
	fake := newFakeDSH(t)
	first, cfg := newTestDriver(t, fake)
	if _, err := first.Open(context.Background(), "ctx-cat"); err != nil {
		t.Fatalf("first Open: %v", err)
	}
	second, err := New(cfg)
	if err != nil {
		t.Fatalf("New reload: %v", err)
	}
	record, err := second.Open(context.Background(), "ctx-cat")
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	if record.SessionID != "dsh-session" {
		t.Fatalf("SessionID = %q", record.SessionID)
	}
	if got := fake.createTotal(); got != 1 {
		t.Fatalf("create calls = %d, want 1 (a persisted session must not be re-created)", got)
	}
}
