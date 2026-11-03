// JSON envelope, error-code vocabulary and shared limits for /monitor/api.
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

const (
	// CODE_BAD_REQUEST marks malformed JSON or parameters.
	CODE_BAD_REQUEST = "bad_request"
	// CODE_UNAUTHORIZED marks a missing or wrong token.
	CODE_UNAUTHORIZED = "unauthorized"
	// CODE_FORBIDDEN marks a Host/Origin rejection or a disabled write.
	CODE_FORBIDDEN = "forbidden"
	// CODE_NOT_FOUND marks an unknown entity or script.
	CODE_NOT_FOUND = "not_found"
	// CODE_TICK_BUSY marks a tick already in flight.
	CODE_TICK_BUSY = "tick_busy"
	// CODE_WRITE_BUSY marks a state write that waited past the 2s gate limit.
	CODE_WRITE_BUSY = "write_busy"
	// CODE_TOO_LARGE marks a script source or log response over its limit.
	CODE_TOO_LARGE = "too_large"
	// CODE_INTERNAL marks a failed store save.
	CODE_INTERNAL = "internal"
	// CODE_DEADLINE_EXCEEDED is reserved for a request-level deadline.
	CODE_DEADLINE_EXCEEDED = "deadline_exceeded"
	// CODE_WRITE_DISABLED marks a write in the no-token read-only shape.
	CODE_WRITE_DISABLED = "write_disabled"

	// MIN_MAX_BYTES is the smallest accepted maxBytes query value.
	MIN_MAX_BYTES = 1024
	// MAX_MAX_BYTES is the largest accepted maxBytes query value.
	MAX_MAX_BYTES = 1024 * 1024
	// DEFAULT_MAX_BYTES is used when maxBytes is absent.
	DEFAULT_MAX_BYTES = 64 * 1024
	// MAX_REQUEST_BYTES bounds one request body.
	MAX_REQUEST_BYTES = 1 << 20
)

// WriteJSON writes one buffered JSON reply with no-store caching and no CORS.
func WriteJSON(w http.ResponseWriter, status int, body map[string]any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// WriteOK writes a 200 reply around the given fields.
func WriteOK(w http.ResponseWriter, fields map[string]any) {
	body := make(map[string]any, len(fields)+1)
	body["ok"] = true
	for key, value := range fields {
		body[key] = value
	}
	WriteJSON(w, http.StatusOK, body)
}

// WriteFailure writes the frozen {"ok":false,"error":{code,message}} envelope.
func WriteFailure(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, map[string]any{
		"ok":    false,
		"error": map[string]any{"code": code, "message": message},
	})
}

// ParseMaxBytes validates the optional maxBytes query parameter.
func ParseMaxBytes(raw string) (int, error) {
	if raw == "" {
		return DEFAULT_MAX_BYTES, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("maxBytes 必须是整数")
	}
	if value < MIN_MAX_BYTES || value > MAX_MAX_BYTES {
		return 0, fmt.Errorf("maxBytes 必须在 %d..%d 之间", MIN_MAX_BYTES, MAX_MAX_BYTES)
	}
	return value, nil
}

// ReadBody reads one request body under the shared size limit. Reading one byte
// past the limit detects silent truncation instead of accepting a clipped body.
func ReadBody(r *http.Request) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, MAX_REQUEST_BYTES+1))
	if err != nil {
		return nil, fmt.Errorf("读取请求体失败: %w", err)
	}
	if len(raw) > MAX_REQUEST_BYTES {
		return nil, fmt.Errorf("请求体超过 %d 字节上限", MAX_REQUEST_BYTES)
	}
	return raw, nil
}

// DecodeBody unmarshals a request body into target; an empty body is allowed.
func DecodeBody(r *http.Request, target any) error {
	raw, err := ReadBody(r)
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("请求体不是合法 JSON: %w", err)
	}
	return nil
}

// MarshalBytes returns the JSON encoding and its byte length.
func MarshalBytes(value any) ([]byte, int, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, 0, err
	}
	return raw, len(raw), nil
}
