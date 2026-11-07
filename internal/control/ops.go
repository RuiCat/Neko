// ops.go bridges the control plane to the shared operation package.
package control

import (
	"errors"
	"net/http"

	"github.com/RuiCat/Neko/internal/control/ops"
)

// opsDeps assembles the shared operation dependencies from the runtime.
func (s *Server) opsDeps() ops.Deps {
	return ops.Deps{
		Store:         s.runtime.Store,
		Machine:       s.runtime.Machine,
		Experts:       s.runtime.Experts,
		DataDir:       s.dataDir,
		CandidatesTTL: s.candidatesTTL,
	}
}

// opsResponse maps one shared-operation result onto the control-plane response
// envelope: 400/404/409/500 for classified failures, 200 with the operation's
// body otherwise.
func opsResponse(body map[string]any, err error) response {
	if err == nil {
		return response{status: http.StatusOK, body: body}
	}
	var opErr *ops.Error
	if !errors.As(err, &opErr) {
		return internalError(err.Error())
	}
	switch opErr.Kind {
	case ops.KIND_BAD_REQUEST:
		return badRequest(opErr.Message)
	case ops.KIND_NOT_FOUND:
		return notFound(opErr.Message)
	case ops.KIND_CONFLICT:
		if opErr.Body != nil {
			return response{status: http.StatusConflict, body: opErr.Body}
		}
		return failure(http.StatusConflict, CODE_CONFLICT, opErr.Message)
	default:
		return internalError(opErr.Message)
	}
}
