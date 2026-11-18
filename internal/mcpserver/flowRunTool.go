// flowRunTool.go registers flow_run, the explicit entry of the frozen P4 flow
// engine: one call reads the runtime flow spec, judges the preconditions,
// advances the steps to done, settles the flow, marks the task data rows and
// saves. It owns no business calculation: the task stage and every reward stay
// with scripts/rules/task/taskRules.neko.
package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/RuiCat/Neko/internal/flow/condition"
	"github.com/RuiCat/Neko/internal/flow/host"
	"github.com/RuiCat/Neko/internal/flow/settle"
	"github.com/RuiCat/Neko/internal/flow/spec"
	"github.com/RuiCat/Neko/internal/flow/step"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// FLOW_SPEC_FILE is the K1 artifact flow_run reads at run time. It lives in
	// the data dir so the repository's runs/ artifact path never enters code.
	FLOW_SPEC_FILE = "flow-spec.json"
	// FLOW_LEDGER_FILE is the durable K4 single-settlement ledger. It keeps
	// (flow, entity) idempotent across process restarts.
	FLOW_LEDGER_FILE = "flow-ledger.json"
	// FLOW_STEP_MINUTES is the minutes charged per step. K1 does not model step
	// duration, so nothing is invented and every step records 0.
	FLOW_STEP_MINUTES = 0
	// FLOW_DEFAULT_TICK is the tick recorded on the step state and the
	// completion. The tool is an explicit entry point with no world tick
	// number, so it records the neutral 0 instead of guessing one.
	FLOW_DEFAULT_TICK = 0
	// FLOW_DEFAULT_DATA_DIR is used when the server has no data dir wired.
	FLOW_DEFAULT_DATA_DIR = "data"
	// PATH_TASK_CURRENT is the active task id row. Settlement only writes data:
	// the stage machine and the reward belong to scripts/rules/task/taskRules.neko.
	PATH_TASK_CURRENT = "任务.当前"
	// PATH_TASK_PROGRESS is the 0-100 progress row read by tkFinish.
	PATH_TASK_PROGRESS = "任务.进度"
	// PROGRESS_COMPLETE is the terminal progress value; the task rule owns the
	// stage transition that consumes it.
	PROGRESS_COMPLETE = 100
)

type flowRunArgs struct {
	Entity      string `json:"entity" jsonschema:"执行流程的实体ID"`
	Flow        string `json:"flow" jsonschema:"flow id（flow-spec.json 的 flows[].id）"`
	ExecutionID string `json:"execution_id,omitempty" jsonschema:"K3 执行ID；留空自动生成"`
}

// flowRunResult is the single JSON body flow_run answers. Report is always
// present once the tool reaches execution; Verdict is present whenever the flow
// was found, including an idempotent replay. Errors that are not a judgment
// rejection are carried in Error with ok=false.
type flowRunResult struct {
	OK         bool               `json:"ok"`
	Completion *settle.Completion `json:"completion,omitempty"`
	Report     *flowRunReport     `json:"report,omitempty"`
	Verdict    *condition.Verdict `json:"verdict,omitempty"`
	Error      string             `json:"error,omitempty"`
}

// flowRunReport is the JSON rendering of settle.Report. Errors become strings
// because an error slice marshals to a list of empty objects.
type flowRunReport struct {
	Changes []settle.Change `json:"changes"`
	Errors  []string        `json:"errors"`
}

// addFlowTool registers flow_run. It is deliberately not a tick hook: nothing
// executes until the tool is called.
func (s *Server) addFlowTool(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "flow_run",
		Description: "显式执行一条 flow：判定 → 推进步骤到 done → 结算 → 写任务数据行并落盘。" +
			"读 <data-dir>/flow-spec.json；同一 (entity, flow) 只结算一次。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args flowRunArgs) (*mcp.CallToolResult, any, error) {
		return jsonResult(s.RunFlow(args.Entity, args.Flow, args.ExecutionID))
	})
}

// RunFlow executes one flow for one entity in a single call and returns the
// tool's JSON body. Every read and write happens under the shared state gate,
// so flow execution serializes with ticks and direct writes. A rejected verdict
// returns ok=false with the reasons and writes nothing.
func (s *Server) RunFlow(entityID, flowID, executionID string) flowRunResult {
	var body flowRunResult
	gate := func() error {
		body = s.runFlowLocked(entityID, flowID, executionID)
		return nil
	}
	_ = s.WithStateWrite(gate)
	return body
}

// runFlowLocked is the body of RunFlow. Callers must hold the state gate.
func (s *Server) runFlowLocked(entityID, flowID, executionID string) flowRunResult {
	body := flowRunResult{Report: newFlowRunReport()}
	entityID = strings.TrimSpace(entityID)
	flowID = strings.TrimSpace(flowID)
	if entityID == "" {
		body.Error = "缺少 entity"
		return body
	}
	if flowID == "" {
		body.Error = "缺少 flow"
		return body
	}
	if s.store.Get(entityID) == nil {
		body.Error = fmt.Sprintf("实体 '%s' 不存在。可用: %s", entityID, jsonString(s.entityIDs()))
		return body
	}
	document, err := s.readFlowSpec()
	if err != nil {
		body.Error = err.Error()
		return body
	}
	flow, found := findFlowSpec(document, flowID)
	if !found {
		body.Error = fmt.Sprintf("%s 中没有 flow %q", s.flowSpecPath(), flowID)
		return body
	}
	ledger, err := host.OpenLedger(s.flowLedgerPath())
	if err != nil {
		body.Error = fmt.Sprintf("打开 flow 账本失败: %v", err)
		return body
	}
	store := host.NewEntityStore(s.store)
	if recorded, settled := ledger.SettlementLedger().Lookup(flow.ID, entityID); settled {
		return s.replayFlow(flow, recorded, store)
	}
	snapshot, err := store.Snapshot(entityID)
	if err != nil {
		body.Error = fmt.Sprintf("读取实体快照失败: %v", err)
		return body
	}
	verdict := condition.Judge(flow, snapshot)
	body.Verdict = &verdict
	if !verdict.OK {
		return body
	}
	state, err := finishedFlowState(flow, executionID)
	if err != nil {
		body.Error = err.Error()
		return body
	}
	completion, report := settle.Settle(flow, verdict, state, settle.Deps{
		Snapshot: snapshot,
		Store:    store,
		Ledger:   ledger.SettlementLedger(),
		Entity:   entityID,
		Tick:     FLOW_DEFAULT_TICK,
	})
	body.Report = flowReportFrom(report)
	if report.Failed() {
		body.Error = "结算失败: " + strings.Join(body.Report.Errors, "; ")
		return body
	}
	body.Completion = &completion
	if err := markTaskRows(store, entityID, flow.ID); err != nil {
		body.Error = fmt.Sprintf("写入任务数据失败: %v", err)
		if saveErr := store.Flush(); saveErr != nil {
			body.Error += fmt.Sprintf("；保存失败: %v", saveErr)
		} else {
			body.Error += "（结算已落盘，重复调用只补写任务数据）"
		}
		return body
	}
	if err := store.Flush(); err != nil {
		body.Error = fmt.Sprintf("保存失败: %v", err)
		return body
	}
	body.OK = true
	return body
}

// replayFlow answers an already settled (flow, entity) from the ledger. It never
// settles or pays again: the recorded completion is returned and only the two
// task data rows are written if a previous run failed before they landed. The
// verdict is re-read from the current snapshot for information only.
func (s *Server) replayFlow(
	flow spec.FlowSpec,
	recorded settle.Completion,
	store *host.EntityStore,
) flowRunResult {
	body := flowRunResult{OK: true, Completion: &recorded, Report: newFlowRunReport()}
	if snapshot, err := store.Snapshot(recorded.Entity); err == nil {
		verdict := condition.Judge(flow, snapshot)
		body.Verdict = &verdict
	}
	if err := markTaskRows(store, recorded.Entity, recorded.FlowID); err != nil {
		body.OK = false
		body.Error = fmt.Sprintf("写入任务数据失败: %v", err)
		return body
	}
	if err := store.Flush(); err != nil {
		body.OK = false
		body.Error = fmt.Sprintf("保存失败: %v", err)
	}
	return body
}

// markTaskRows writes the two task data rows a settled flow owes the task rule:
// the active task id and the terminal progress. It deliberately writes no
// reward and no stage: tkFinish/tkReward in taskRules.neko own those, and a
// second writer would double-pay or skip the stage machine.
func markTaskRows(store *host.EntityStore, entityID, flowID string) error {
	if err := store.Set(entityID, PATH_TASK_CURRENT, flowID); err != nil {
		return fmt.Errorf("%s: %w", PATH_TASK_CURRENT, err)
	}
	if err := store.Set(entityID, PATH_TASK_PROGRESS, PROGRESS_COMPLETE); err != nil {
		return fmt.Errorf("%s: %w", PATH_TASK_PROGRESS, err)
	}
	return nil
}

// finishedFlowState starts one K3 execution and consumes every step, so the
// settlement precondition (done) is met inside this single call. Minutes are 0
// because K1 does not model step duration.
func finishedFlowState(flow spec.FlowSpec, executionID string) (step.State, error) {
	if executionID == "" {
		executionID = fmt.Sprintf("flow-%s-%d", flow.ID, time.Now().UnixNano())
	}
	state, err := step.Start(&flow, executionID, FLOW_DEFAULT_TICK)
	if err != nil {
		return step.State{}, fmt.Errorf("步骤启动失败: %w", err)
	}
	for range flow.Steps {
		state, err = step.Next(&flow, state, FLOW_STEP_MINUTES)
		if err != nil {
			return step.State{}, fmt.Errorf("步骤推进失败: %w", err)
		}
	}
	if state.Status != step.StatusDone {
		return step.State{}, fmt.Errorf("步骤未到 done: %s", state.Status)
	}
	return state, nil
}

// readFlowSpec loads the runtime K1 document. Numbers keep their json.Number
// form so effect literals stay exact through settlement.
func (s *Server) readFlowSpec() (spec.FlowSpecFile, error) {
	path := s.flowSpecPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return spec.FlowSpecFile{}, fmt.Errorf(
				"缺少 flow 规格文件 %s；请把 flow-spec.json 放到 %s",
				path, filepath.Dir(path))
		}
		return spec.FlowSpecFile{}, fmt.Errorf("读取 %s 失败: %v", path, err)
	}
	var document spec.FlowSpecFile
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return spec.FlowSpecFile{}, fmt.Errorf("解析 %s 失败: %v", path, err)
	}
	return document, nil
}

// flowSpecPath returns the runtime flow-spec path inside the data dir.
func (s *Server) flowSpecPath() string {
	return filepath.Join(s.flowDataDir(), FLOW_SPEC_FILE)
}

// flowLedgerPath returns the durable single-settlement ledger path.
func (s *Server) flowLedgerPath() string {
	return filepath.Join(s.flowDataDir(), FLOW_LEDGER_FILE)
}

// flowDataDir resolves the data dir, falling back to "data" like the binaries.
func (s *Server) flowDataDir() string {
	if s.dataDir == "" {
		return FLOW_DEFAULT_DATA_DIR
	}
	return s.dataDir
}

// findFlowSpec looks one flow up by id.
func findFlowSpec(document spec.FlowSpecFile, flowID string) (spec.FlowSpec, bool) {
	for _, flow := range document.Flows {
		if flow.ID == flowID {
			return flow, true
		}
	}
	return spec.FlowSpec{}, false
}

// newFlowRunReport returns an empty report with non-nil slices so JSON renders
// [] instead of null.
func newFlowRunReport() *flowRunReport {
	return &flowRunReport{Changes: []settle.Change{}, Errors: []string{}}
}

// flowReportFrom renders one settlement report with readable error strings.
func flowReportFrom(report settle.Report) *flowRunReport {
	rendered := newFlowRunReport()
	if report.Changes != nil {
		rendered.Changes = report.Changes
	}
	for _, err := range report.Errors {
		if err != nil {
			rendered.Errors = append(rendered.Errors, err.Error())
		}
	}
	return rendered
}
