// Request-body reading limits shared by the POST endpoints.
package control

import (
	"fmt"
	"io"
	"net/http"
)

// MAX_BODY_BYTES caps one control-plane request body (K1: ≤64 KiB).
const MAX_BODY_BYTES = 64 * 1024

// readBody reads at most MAX_BODY_BYTES and rejects larger payloads instead of
// truncating them.
func readBody(r *http.Request) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, MAX_BODY_BYTES+1))
	if err != nil {
		return nil, fmt.Errorf("读取请求体失败: %w", err)
	}
	if len(raw) > MAX_BODY_BYTES {
		return nil, fmt.Errorf("请求体超过 %d 字节上限", MAX_BODY_BYTES)
	}
	return raw, nil
}
