// POST /clock handler: advance or set the in-world clock (K19).
package control

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/RuiCat/Neko/internal/mcpserver"
)

// clockRequest is the POST /clock body; minutes and time are mutually
// exclusive and exactly one is required.
type clockRequest struct {
	Minutes *int   `json:"minutes,omitempty"`
	Time    string `json:"time,omitempty"`
}

// handleClock writes 世界.环境.当前时间 plus its month/phase mirrors under the
// shared state gate and saves the store. The phase arithmetic is the same one
// the tick path uses.
func (s *Server) handleClock(r *http.Request) response {
	raw, err := readBody(r)
	if err != nil {
		return badRequest(err.Error())
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return badRequest("请求体不能为空")
	}
	request := clockRequest{}
	if err := json.Unmarshal(raw, &request); err != nil {
		return badRequest("请求体不是合法 JSON: " + err.Error())
	}
	if request.Minutes != nil && request.Time != "" {
		return badRequest("minutes 与 time 不能同时使用")
	}
	if request.Minutes == nil && request.Time == "" {
		return badRequest("缺少 minutes 或 time")
	}
	var moment time.Time
	if request.Minutes != nil {
		if *request.Minutes < 1 || *request.Minutes > mcpserver.MAX_CLOCK_ADVANCE_MINUTES {
			return badRequest(fmt.Sprintf("minutes 必须在 1..%d 之间", mcpserver.MAX_CLOCK_ADVANCE_MINUTES))
		}
	} else {
		parsed, parseErr := time.Parse(time.RFC3339, request.Time)
		if parseErr != nil {
			return badRequest("time 必须是 RFC3339 时间: " + parseErr.Error())
		}
		moment = parsed
	}
	var state mcpserver.ClockState
	writeErr := s.mcp.WithStateWrite(func() error {
		var err error
		if request.Minutes != nil {
			state, err = s.mcp.AdvanceWorldClockState(*request.Minutes)
		} else {
			state, err = s.mcp.SetWorldClockState(moment)
		}
		if err != nil {
			return err
		}
		return s.runtime.Store.Save()
	})
	if writeErr != nil {
		switch {
		case errors.Is(writeErr, mcpserver.ErrWorldEntityMissing):
			return notFound(fmt.Sprintf("实体 '%s' 不存在", mcpserver.WorldEntityID()))
		case errors.Is(writeErr, mcpserver.ErrWorldTimeInvalid):
			return badRequest(mcpserver.ErrWorldTimeInvalid.Error())
		default:
			return internalError("时钟写入失败: " + writeErr.Error())
		}
	}
	return response{status: http.StatusOK, body: map[string]any{
		"ok": true, "worldTime": state.Time, "month": state.Month, "phase": state.Phase,
	}, note: fmt.Sprintf("worldTime=%s phase=%s", state.Time, state.Phase)}
}
