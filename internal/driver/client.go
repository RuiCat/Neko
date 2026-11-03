// DSH HTTP client: launch-token exchange plus the unary session Remote calls.
package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// DEFAULT_TIMEOUT bounds one DSH HTTP round trip.
	DEFAULT_TIMEOUT = 10 * time.Second
	// CONNECT_PATH is the launch-token exchange route.
	CONNECT_PATH = "/connect"
	// API_PREFIX is the Remote RPC channel.
	API_PREFIX = "/api/"
	// MAX_RESPONSE_BYTES caps one response body; session/list includes every
	// session's projections and can reach several MB.
	MAX_RESPONSE_BYTES = 64 << 20
	// ERROR_BODY_BYTES bounds the diagnostic text kept for a non-200 reply.
	ERROR_BODY_BYTES = 64 << 10
)

// APIError carries a transport status or a {ok:false} business code.
type APIError struct {
	// StatusCode is the HTTP status; 200 marks a business failure.
	StatusCode int
	// Code is the DSH error code, for example session/agent-busy.
	Code string
	// Message is the human-readable failure text.
	Message string
}

// Error renders the status, business code and message.
func (e *APIError) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("DSH HTTP %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("DSH %s: %s", e.Code, e.Message)
}

// IsCode reports whether the failure carries the given DSH business code.
func (e *APIError) IsCode(code string) bool {
	return e != nil && e.Code == code
}

// Client talks to one DSH Web GUI over the authenticated /api channel.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
	mu      sync.Mutex
	cookie  string
	rpcID   atomic.Uint64
}

// NewClient builds a client for one DSH origin.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http: &http.Client{
			Timeout: DEFAULT_TIMEOUT,
			// /connect answers 302 with the signed cookie; follow nothing.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// ensureCookie exchanges the launch token for the signed API cookie once.
func (c *Client) ensureCookie(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ensureCookieLocked(ctx)
}

// ensureCookieLocked performs the exchange; the caller holds c.mu.
func (c *Client) ensureCookieLocked(ctx context.Context) error {
	if c.cookie != "" {
		return nil
	}
	endpoint := c.baseURL + CONNECT_PATH + "?token=" + url.QueryEscape(c.token) + "&interactions=none"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode != http.StatusFound {
		return &APIError{StatusCode: response.StatusCode, Code: "connect/rejected",
			Message: "启动 token 交换失败（token 是否正确、DSH 是否在运行）"}
	}
	for _, cookie := range response.Cookies() {
		if cookie.Name != "" {
			c.cookie = cookie.Name + "=" + cookie.Value
			return nil
		}
	}
	return &APIError{StatusCode: response.StatusCode, Code: "connect/no-cookie",
		Message: "启动 token 交换没有返回 Cookie"}
}

// post calls one Remote endpoint, refreshing the cookie once on 401.
func (c *Client) post(ctx context.Context, endpoint string, payload any, out any) error {
	if err := c.ensureCookie(ctx); err != nil {
		return err
	}
	err := c.postOnce(ctx, endpoint, payload, out)
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized {
		c.mu.Lock()
		c.cookie = ""
		c.mu.Unlock()
		if refreshErr := c.ensureCookie(ctx); refreshErr != nil {
			return refreshErr
		}
		return c.postOnce(ctx, endpoint, payload, out)
	}
	return err
}

// postOnce sends one envelope and decodes its result. The DSH Gateway requires
// the payload to be exactly one plain-object `args` field.
func (c *Client) postOnce(ctx context.Context, endpoint string, payload any, out any) error {
	rpcID := fmt.Sprintf("neko-%d", c.rpcID.Add(1))
	body, err := json.Marshal(clientRequest{
		Type: "client-request", RPCID: rpcID, Method: endpoint,
		Payload: remoteArgs{Args: payload},
	})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+API_PREFIX+endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	c.mu.Lock()
	request.Header.Set("Cookie", c.cookie)
	c.mu.Unlock()
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized {
		return &APIError{StatusCode: response.StatusCode, Code: "unauthorized", Message: "DSH API 需要重新认证"}
	}
	if response.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, ERROR_BODY_BYTES))
		return &APIError{StatusCode: response.StatusCode, Code: "http",
			Message: strings.TrimSpace(string(raw))}
	}
	// session/list carries every session's projections and can reach MBs, so
	// decode as a stream over the (generous) cap instead of buffering first.
	envelope := serverResponse{}
	if err := json.NewDecoder(io.LimitReader(response.Body, MAX_RESPONSE_BYTES)).Decode(&envelope); err != nil {
		return fmt.Errorf("解析 DSH 响应失败: %w", err)
	}
	if !envelope.Result.OK {
		if envelope.Result.Error == nil {
			return &APIError{StatusCode: response.StatusCode, Code: "unknown", Message: "DSH 返回了失败但没有错误码"}
		}
		return &APIError{StatusCode: response.StatusCode,
			Code: envelope.Result.Error.Code, Message: envelope.Result.Error.Message}
	}
	if out == nil || len(envelope.Result.Value) == 0 {
		return nil
	}
	return json.Unmarshal(envelope.Result.Value, out)
}

// remoteArgs is the single plain-object field the DSH Gateway accepts.
type remoteArgs struct {
	Args any `json:"args"`
}

// clientRequest is the Remote envelope; the wire contract is fixed by DSH.
type clientRequest struct {
	Type    string `json:"type"`
	RPCID   string `json:"rpcId"`
	Method  string `json:"method"`
	Payload any    `json:"payload"`
}

// serverResponse is the Remote reply envelope.
type serverResponse struct {
	Type   string `json:"type"`
	RPCID  string `json:"rpcId"`
	Result struct {
		OK    bool            `json:"ok"`
		Value json.RawMessage `json:"value"`
		Error *remoteError    `json:"error"`
	} `json:"result"`
}

// remoteError is the business failure carried by a reply envelope.
type remoteError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// CreateRequest is the session/create payload.
type CreateRequest struct {
	Cwd         string `json:"cwd,omitempty"`
	AgentPreset string `json:"agentPreset,omitempty"`
	SessionID   string `json:"sessionId,omitempty"`
}

// CreateValue is the session/create reply.
type CreateValue struct {
	SessionID   string `json:"sessionId"`
	AgentPreset string `json:"agentPreset,omitempty"`
}

// ContentPart is one prompt content part.
type ContentPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// PromptRequest is the session/prompt payload.
type PromptRequest struct {
	RequestID      string        `json:"requestId"`
	SessionID      string        `json:"sessionId"`
	Mode           string        `json:"mode"`
	Content        []ContentPart `json:"content"`
	ClientTimeZone string        `json:"clientTimeZone,omitempty"`
}

// SessionSummary is one row of session/list.
type SessionSummary struct {
	SessionID      string                `json:"sessionId"`
	Running        bool                  `json:"running"`
	AgentAvailable bool                  `json:"agentAvailable"`
	Blank          bool                  `json:"blank"`
	Projections    *SessionProjectionSet `json:"projections,omitempty"`
}

// SessionProjectionSet is the subset of projections the driver reconciles.
type SessionProjectionSet struct {
	Values struct {
		SessionStats struct {
			Turns int `json:"turns"`
		} `json:"sessionStats"`
	} `json:"values"`
}

// listValue is the session/list reply.
type listValue struct {
	Items []SessionSummary `json:"items"`
}

// CreateSession creates or resumes one DSH session. The Remote descriptor names
// its single business parameter `request`.
func (c *Client) CreateSession(ctx context.Context, request CreateRequest) (CreateValue, error) {
	value := CreateValue{}
	err := c.post(ctx, "session/create", map[string]any{"request": request}, &value)
	return value, err
}

// Prompt admits one user message into a session.
func (c *Client) Prompt(ctx context.Context, request PromptRequest) error {
	return c.post(ctx, "session/prompt", map[string]any{"request": request}, nil)
}

// Cancel interrupts the active turn of a session while keeping its inbox.
func (c *Client) Cancel(ctx context.Context, sessionID string) error {
	request := map[string]string{"sessionId": sessionID}
	return c.post(ctx, "session/cancel", map[string]any{"request": request}, nil)
}

// List reads every session summary; its descriptor names the parameter
// `_request`.
func (c *Client) List(ctx context.Context) ([]SessionSummary, error) {
	value := listValue{}
	if err := c.post(ctx, "session/list", map[string]any{"_request": map[string]any{}}, &value); err != nil {
		return nil, err
	}
	return value.Items, nil
}
