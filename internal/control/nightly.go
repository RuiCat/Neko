// nightly.go wires POST /nightly to the shared nightly decay operation.
package control

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/RuiCat/Neko/internal/control/ops"
)

// nightlyRequest is the optional POST /nightly body.
type nightlyRequest struct {
	Factor *float64 `json:"factor"`
}

// handleNightly decays every expert context and reports what was dropped.
func (s *Server) handleNightly(r *http.Request) response {
	raw, err := readBody(r)
	if err != nil {
		return badRequest(err.Error())
	}
	request := nightlyRequest{}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &request); err != nil {
			return badRequest("请求体不是合法 JSON: " + err.Error())
		}
	}
	body, err := ops.Nightly(s.opsDeps(), request.Factor)
	return opsResponse(body, err)
}
