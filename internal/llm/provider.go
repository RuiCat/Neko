// Package llm is the remote-model boundary (D3): one Provider interface and
// one OpenAI-compatible adapter. Keys come from the environment and are never
// written to logs or journal records.
package llm

import (
	"context"
	"errors"
)

// Message is one chat turn sent to the provider.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Request is one completion request.
type Request struct {
	Model       string
	Messages    []Message
	Temperature float64
	MaxTokens   int
}

// Usage reports token consumption for budget accounting.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// Response is one completion result.
type Response struct {
	Content string
	Model   string
	Usage   Usage
}

// Provider is the single remote-model entry point.
type Provider interface {
	Complete(ctx context.Context, req Request) (Response, error)
}

// ErrMissingAPIKey rejects construction without a key.
var ErrMissingAPIKey = errors.New("llm: 缺少 API key（只从环境变量读取）")

// ErrBudgetExceeded stops calls once the configured budget is spent.
var ErrBudgetExceeded = errors.New("llm: 调用预算已用尽")
