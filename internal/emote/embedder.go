// embedder.go is the Ollama embedding client: one POST to /api/embeddings per
// query, mirroring the Python request so the shipped nomic-embed-text index
// stays comparable. A failed request returns ErrEmbedding instead of silently
// degrading (decision d3).
package emote

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// MAX_ERROR_BODY bounds how much of an error response is quoted back.
const MAX_ERROR_BODY = 1 << 20

// OllamaEmbedder calls an Ollama embeddings endpoint.
type OllamaEmbedder struct {
	// URL is the /api/embeddings endpoint.
	URL string
	// Model is the embedding model name, e.g. nomic-embed-text.
	Model string
	// Client bounds one request; nil uses a DEFAULT_TIMEOUT client.
	Client *http.Client
}

// NewOllamaEmbedder builds an embedder with sane defaults.
func NewOllamaEmbedder(url, model string, timeout time.Duration) *OllamaEmbedder {
	if strings.TrimSpace(url) == "" {
		url = DEFAULT_EMBED_URL
	}
	if strings.TrimSpace(model) == "" {
		model = DEFAULT_MODEL
	}
	if timeout <= 0 {
		timeout = DEFAULT_TIMEOUT
	}
	return &OllamaEmbedder{URL: url, Model: model, Client: &http.Client{Timeout: timeout}}
}

// embedRequest is Ollama's request body (same fields as the Python version).
type embedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

// embedResponse carries the returned vector.
type embedResponse struct {
	Embedding []float32 `json:"embedding"`
}

// Embed turns text into a vector.
func (o *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	body, err := json.Marshal(embedRequest{Model: o.Model, Prompt: text})
	if err != nil {
		return nil, fmt.Errorf("%w: 请求序列化失败: %v", ErrEmbedding, err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, o.URL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: 构造请求失败: %v", ErrEmbedding, err)
	}
	request.Header.Set("Content-Type", "application/json")
	client := o.Client
	if client == nil {
		client = &http.Client{Timeout: DEFAULT_TIMEOUT}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEmbedding, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, MAX_ERROR_BODY))
		return nil, fmt.Errorf("%w: HTTP %d: %s", ErrEmbedding, response.StatusCode, strings.TrimSpace(string(detail)))
	}
	var decoded embedResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("%w: 响应解析失败: %v", ErrEmbedding, err)
	}
	if len(decoded.Embedding) == 0 {
		return nil, fmt.Errorf("%w: 返回空向量", ErrEmbedding)
	}
	return decoded.Embedding, nil
}
