// opsTools.go registers thin MCP wrappers over the shared operation package:
// nightly decay, judging, the candidate queue and the repair loop (K20).
package mcpserver

import (
	"context"

	"github.com/RuiCat/Neko/internal/control/ops"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type nightlyRunArgs struct {
	Factor *float64 `json:"factor,omitempty" jsonschema:"衰减系数 (0,1)；留空用默认 0.5"`
}

type judgeRunArgs struct {
	EntityID string   `json:"entity_id" jsonschema:"实体ID"`
	Path     string   `json:"path" jsonschema:"点分路径"`
	Proposed string   `json:"proposed,omitempty" jsonschema:"拟议变更值（JSON 或纯文本）"`
	Source   string   `json:"source,omitempty" jsonschema:"来源标记，如 teacher"`
	Evidence []string `json:"evidence,omitempty" jsonschema:"证据列表（可含 snapshot:<id> / when:<expr>）"`
}

type candidateResolveArgs struct {
	ID     string `json:"id" jsonschema:"候选ID"`
	Status string `json:"status" jsonschema:"approved 或 rejected（approved 才能上线修复）"`
	Note   string `json:"note,omitempty" jsonschema:"处置备注"`
}

type candidateApplyArgs struct {
	ID    string `json:"id" jsonschema:"候选ID（须已 approved）"`
	When  string `json:"when,omitempty" jsonschema:"规则条件；缺省取 evidence 的 when:<expr>"`
	Match string `json:"match,omitempty" jsonschema:"实体选择器；缺省用候选实体"`
}

// SetOpsDeps wires the shared operation dependencies from the runtime. Without
// it the operational tools report a configuration failure instead of panicking.
func (s *Server) SetOpsDeps(deps ops.Deps) { s.opsDeps = deps }

// opsToolResult turns one shared-operation result into the MCP tool result:
// success is JSON, a classified failure keeps the existing ❌ text convention.
func opsToolResult(body map[string]any, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return fail("%s", err)
	}
	return jsonResult(body)
}

// addOpsTools registers the six operational tools over the shared operations.
func (s *Server) addOpsTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "nightly_run",
		Description: "运行一次夜间衰减：专家上下文权重×factor，并清理过期候选；结果落盘。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args nightlyRunArgs) (*mcp.CallToolResult, any, error) {
		body, err := ops.Nightly(s.opsDeps, args.Factor)
		return opsToolResult(body, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "judge_run",
		Description: "对一条拟议状态变更做只读裁决；未被规则覆盖的候选会进入复核队列。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args judgeRunArgs) (*mcp.CallToolResult, any, error) {
		request := ops.JudgeRequest{
			EntityID: args.EntityID,
			Path:     args.Path,
			Source:   args.Source,
			Evidence: args.Evidence,
		}
		if args.Proposed != "" {
			request.Proposed = parseValue(args.Proposed)
		}
		body, err := ops.Judge(s.opsDeps, request)
		return opsToolResult(body, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "candidates_list",
		Description: "列出待复核的候选（pending），按队列文件顺序返回。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		body, err := ops.Candidates(s.opsDeps)
		return opsToolResult(body, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "candidate_resolve",
		Description: "批准或拒绝一条候选（approved/rejected）；批准后可用 candidate_apply 上线。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args candidateResolveArgs) (*mcp.CallToolResult, any, error) {
		body, err := ops.ResolveCandidate(s.opsDeps, ops.ResolveRequest{
			ID: args.ID, Status: args.Status, Note: args.Note,
		})
		return opsToolResult(body, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "candidate_apply",
		Description: "把已批准的候选上线为修复版本并跑回归闸门；未通过会回滚。" +
			"需要 when 条件（参数或 evidence 的 when:<expr>）与快照锚点。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args candidateApplyArgs) (*mcp.CallToolResult, any, error) {
		body, err := ops.ApplyCandidate(s.opsDeps, ops.ApplyRequest{
			ID: args.ID, When: args.When, Match: args.Match,
		})
		return opsToolResult(body, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "repair_rollback",
		Description: "回滚最新一个未回滚的修复版本，并移除它安装的引擎规则。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		body, err := ops.RollbackRepair(s.opsDeps)
		return opsToolResult(body, err)
	})
}
