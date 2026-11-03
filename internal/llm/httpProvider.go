// httpProvider.go is the OpenAI-compatible HTTP adapter: retries only on 429
// and 5xx, never logs the API key, and checks the token budget before calling.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// DEFAULT_TIMEOUT bounds one provider call.
const DEFAULT_TIMEOUT = 30 * time.Second

// DEFAULT_MAX_RETRIES is the retry allowance for 429/5xx responses.
const DEFAULT_MAX_RETRIES = 2

// MAX_RESPONSE_BYTES caps how much of an error body is read.
const MAX_RESPONSE_BYTES = 1 << 20

// HTTPProvider talks to an OpenAI-compatible chat-completions endpoint.
type HTTPProvider struct {
	Endpoint     string
	APIKey       string
	Model        string
	Client       *http.Client
	MaxRetries   int
	RetryBackoff time.Duration
	Budget       *Budget
}

// NewHTTPProvider builds a provider. An empty key is rejected so a missing
// environment variable fails at startup, not on the first user turn.
func NewHTTPProvider(endpoint, apiKey, model string) (*HTTPProvider, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, ErrMissingAPIKey
	}
	if strings.TrimSpace(endpoint) == "" {
		return nil, fmt.Errorf("llm: endpoint 不能为空")
	}
	return &HTTPProvider{
		Endpoint:     strings.TrimRight(endpoint, "/"),
		APIKey:       apiKey,
		Model:        model,
		Client:       &http.Client{Timeout: DEFAULT_TIMEOUT},
		MaxRetries:   DEFAULT_MAX_RETRIES,
		RetryBackoff: 50 * time.Millisecond,
	}, nil
}

// FromEnv reads endpoint, key and model from the named environment variables.
func FromEnv(endpointVar, keyVar, modelVar string) (*HTTPProvider, error) {
	return NewHTTPProvider(os.Getenv(endpointVar), os.Getenv(keyVar), os.Getenv(modelVar))
}

// Complete sends one chat request and returns the first choice.
func (p *HTTPProvider) Complete(ctx context.Context, req Request) (Response, error) {
	if err := p.Budget.Check(); err != nil {
		return Response{}, err
	}
	model := req.Model
	if model == "" {
		model = p.Model
	}
	body, err := json.Marshal(map[string]any{
		"model":       model,
		"messages":    req.Messages,
		"temperature": req.Temperature,
		"max_tokens":  req.MaxTokens,
	})
	if err != nil {
		return Response{}, fmt.Errorf("llm: 请求序列化失败: %w", err)
	}
	retries := p.MaxRetries
	if retries < 0 {
		retries = 0
	}
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		response, retry, err := p.attempt(ctx, body)
		if err == nil {
			p.Budget.Add(int64(response.Usage.PromptTokens + response.Usage.CompletionTokens))
			return response, nil
		}
		lastErr = err
		if !retry {
			return Response{}, err
		}
		if attempt < retries {
			time.Sleep(p.RetryBackoff * time.Duration(attempt+1))
		}
	}
	return Response{}, lastErr
}

// attempt performs one HTTP call and reports whether a retry is allowed.
func (p *HTTPProvider) attempt(ctx context.Context, body []byte) (Response, bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.Endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Response{}, false, fmt.Errorf("llm: 构造请求失败: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+p.APIKey)
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: DEFAULT_TIMEOUT}
	}
	httpResponse, err := client.Do(request)
	if err != nil {
		return Response{}, true, fmt.Errorf("llm: 请求失败: %w", err)
	}
	defer httpResponse.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(httpResponse.Body, MAX_RESPONSE_BYTES))
	if readErr != nil {
		return Response{}, true, fmt.Errorf("llm: 读取响应失败: %w", readErr)
	}
	if httpResponse.StatusCode == http.StatusTooManyRequests || httpResponse.StatusCode >= 500 {
		return Response{}, true, fmt.Errorf("llm: HTTP %d: %s", httpResponse.StatusCode, truncate(raw))
	}
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		return Response{}, false, fmt.Errorf("llm: HTTP %d: %s", httpResponse.StatusCode, truncate(raw))
	}
	return parseResponse(raw)
}

// parseResponse maps the OpenAI chat-completions shape onto Response.
func parseResponse(raw []byte) (Response, bool, error) {
	var payload struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage Usage `json:"usage"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return Response{}, false, fmt.Errorf("llm: 响应解析失败: %w", err)
	}
	if len(payload.Choices) == 0 {
		return Response{}, false, fmt.Errorf("llm: 响应没有 choices")
	}
	return Response{Content: payload.Choices[0].Message.Content, Model: payload.Model, Usage: payload.Usage}, false, nil
}

// truncate keeps error bodies bounded and never echoes credentials.
func truncate(raw []byte) string {
	text := strings.TrimSpace(string(raw))
	if len(text) > 200 {
		return text[:200] + "…"
	}
	return text
}
