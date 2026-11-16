// worldClockTool.go registers the MCP world_clock tool (K20).
package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type worldClockArgs struct {
	Minutes *int   `json:"minutes,omitempty" jsonschema:"推进分钟数（1..1440）；与 time 二选一"`
	Time    string `json:"time,omitempty" jsonschema:"设定时间（RFC3339，如 2026-11-07T18:00:00+08:00）；与 minutes 二选一"`
}

// clockBody renders one clock state as the shared {ok,worldTime,month,phase}
// body used by POST /clock and this tool.
func clockBody(state ClockState) map[string]any {
	return map[string]any{
		"ok":        true,
		"worldTime": state.Time,
		"month":     state.Month,
		"phase":     state.Phase,
	}
}

// clockFailure maps one clock error onto the existing ❌ tool convention.
func clockFailure(err error) (*mcp.CallToolResult, any, error) {
	switch {
	case errors.Is(err, ErrWorldEntityMissing):
		return fail("实体 '%s' 不存在", worldEntityID())
	case errors.Is(err, ErrWorldTimeInvalid):
		return fail("%s", ErrWorldTimeInvalid)
	default:
		return fail("%s", err)
	}
}

// addWorldClockTool registers read/advance/set of the in-world clock.
func (s *Server) addWorldClockTool(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "world_clock",
		Description: "读取/推进/设定世界时钟（世界.环境.当前时间、当前月份、状态.阶段）。" +
			"无参数=读取；minutes 或 time 二选一；写入会落盘。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args worldClockArgs) (*mcp.CallToolResult, any, error) {
		if args.Minutes == nil && args.Time == "" {
			state, err := s.WorldClockState()
			if err != nil {
				return clockFailure(err)
			}
			return jsonResult(clockBody(state))
		}
		if args.Minutes != nil && args.Time != "" {
			return fail("minutes 与 time 不能同时使用")
		}
		var moment time.Time
		if args.Minutes != nil {
			if *args.Minutes < 1 || *args.Minutes > MAX_CLOCK_ADVANCE_MINUTES {
				return fail("minutes 必须在 1..%d 之间", MAX_CLOCK_ADVANCE_MINUTES)
			}
		} else {
			parsed, err := time.Parse(time.RFC3339, args.Time)
			if err != nil {
				return fail("time 必须是 RFC3339 时间: %s", err)
			}
			moment = parsed
		}
		var state ClockState
		writeErr := s.WithStateWrite(func() error {
			var err error
			if args.Minutes != nil {
				state, err = s.AdvanceWorldClockState(*args.Minutes)
			} else {
				state, err = s.SetWorldClockState(moment)
			}
			if err != nil {
				return err
			}
			if err := s.store.Save(); err != nil {
				return fmt.Errorf("保存失败: %w", err)
			}
			return nil
		})
		if writeErr != nil {
			return clockFailure(writeErr)
		}
		return jsonResult(clockBody(state))
	})
}
