// trainingTools.go registers the gate-teaching tools: the agent (智慧体) submits
// its own labeled samples, inspects coverage and runs the one-click training
// pipeline itself. Weights are only installed when the caller explicitly asks
// for deploy, and that change takes effect after dsh-mcp restarts.
package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/RuiCat/Neko/internal/gate"
	"github.com/RuiCat/Neko/internal/learn/samples"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// 训练门槛默认值与 scripts/tools/trainGateModel.sh 保持一致。
const (
	GATE_MIN_SAMPLES       = 50
	GATE_MIN_CLASSES       = 4
	GATE_MIN_CLASS_SAMPLES = 5
	GATE_MAX_CLASS_SHARE   = 0.7
	GATE_MIN_ACCURACY      = 0.8
	GATE_TRAIN_EPOCHS      = 400
	GATE_TEACH_MAX_SAMPLES = 200
	GATE_TRAIN_TIMEOUT     = 10 * time.Minute
)

// TEACHER_SOURCE 标记智慧体主动提交的教师样本（写入 Record.Source）。
const TEACHER_SOURCE = "teacher"

// gateTeachSample 是智慧体提交的一条教师样本：输入信号 + 目标语义专家。
type gateTeachSample struct {
	TargetExpert  string             `json:"target_expert" jsonschema:"目标语义专家（关系记忆/事件记忆/自我记忆/知识边界）"`
	EventKind     string             `json:"event_kind,omitempty" jsonschema:"事件类型（relationship/conflict/apology/question/knowledge/emotion/self/event/turn，缺省 turn）"`
	Summary       string             `json:"summary,omitempty" jsonschema:"对话摘要（训练特征用）"`
	Affect        map[string]float64 `json:"affect,omitempty" jsonschema:"情感向量，如 {trust:0.4,valence:-0.8}"`
	HardStateKeys []string           `json:"hard_state_keys,omitempty" jsonschema:"硬状态键，如 知识边界"`
}

// gateTeachArgs 是一次教师样本提交。
type gateTeachArgs struct {
	Samples []gateTeachSample `json:"samples" jsonschema:"教师样本（1 到 200 条）"`
}

// gateStatusArgs 是空参数占位。
type gateStatusArgs struct{}

// gateTrainArgs 控制一次训练：默认 dry-run，deploy 才安装权重。
type gateTrainArgs struct {
	Deploy          bool    `json:"deploy,omitempty" jsonschema:"是否安装权重：默认 false 只训练校验，true 过门槛后安装（重启 dsh-mcp 生效）"`
	MinSamples      int     `json:"min_samples,omitempty" jsonschema:"样本门槛（默认 50）"`
	MinClasses      int     `json:"min_classes,omitempty" jsonschema:"类别数门槛（默认 4）"`
	MinClassSamples int     `json:"min_class_samples,omitempty" jsonschema:"每类最少样本（默认 5）"`
	MaxClassShare   float64 `json:"max_class_share,omitempty" jsonschema:"最大类别占比门槛（0-1，默认 0.7；超过拒绝，--force 例外）"`
	MinAccuracy     float64 `json:"min_accuracy,omitempty" jsonschema:"留出集准确率门槛（默认 0.8）"`
	Epochs          int     `json:"epochs,omitempty" jsonschema:"训练轮数（默认 400）"`
	ITADir          string  `json:"ita_dir,omitempty" jsonschema:"算法导论仓库路径（默认 $ITA_DIR）"`
}

// addTrainingTools 注册 gate_status / gate_teach / gate_train 三个工具。
func (s *Server) addTrainingTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gate_status",
		Description: "查看门控训练数据状态：自动/教师样本数、可导出样本、主专家标签分布、覆盖门槛是否满足、模型是否已安装。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args gateStatusArgs) (*mcp.CallToolResult, any, error) {
		return jsonResult(s.gateStatus())
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gate_teach",
		Description: "提交教师标签样本（智慧体主动提供训练数据）：每条 = 输入信号 + 目标语义专家，写入 <data>/training/gate-teacher.jsonl，训练时与自动样本合并。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args gateTeachArgs) (*mcp.CallToolResult, any, error) {
		return s.gateTeach(args)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gate_train",
		Description: "运行门控一键训练：样本/类别覆盖/准确率门槛全过才产出权重；默认 dry-run（只训练校验、不安装），deploy=true 才安装（重启 dsh-mcp 后生效）。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args gateTrainArgs) (*mcp.CallToolResult, any, error) {
		return s.gateTrain(ctx, args)
	})
}

// gateStatus 汇总训练数据与门槛状态；读不到样本不报错，按空数据展示。
func (s *Server) gateStatus() map[string]any {
	autoRecords, autoErr := samples.ReadFile(samples.Path(s.dataDir, samples.SAMPLES_FILE))
	teacherRecords, teacherErr := samples.ReadFile(samples.Path(s.dataDir, samples.TEACHER_FILE))
	labels := make(map[string]int, len(gate.SEMANTIC_EXPERTS))
	for _, expert := range gate.SEMANTIC_EXPERTS {
		labels[expert] = 0
	}
	exportable := 0
	skipped := 0
	covered := 0
	minClass := 0
	total := 0
	maxClass := 0
	maxClassName := ""
	perClass := make(map[int]int, len(gate.SEMANTIC_EXPERTS))
	for _, record := range append(append([]samples.Record{}, autoRecords...), teacherRecords...) {
		label := samples.LabelFor(record)
		if label < 0 {
			skipped++
			continue
		}
		exportable++
		perClass[label]++
		labels[gate.SEMANTIC_EXPERTS[label]]++
	}
	for index, count := range perClass {
		covered++
		total += count
		if minClass == 0 || count < minClass {
			minClass = count
		}
		if count > maxClass {
			maxClass = count
			maxClassName = gate.SEMANTIC_EXPERTS[index]
		}
	}
	share := 0.0
	if total > 0 {
		share = float64(maxClass) / float64(total)
	}
	balanced := share <= GATE_MAX_CLASS_SHARE
	modelPath := filepath.Join(s.dataDir, "models", "gate-mlp-v1.json")
	body := map[string]any{
		"ok":              autoErr == nil && teacherErr == nil,
		"data_dir":        s.dataDir,
		"auto_samples":    len(autoRecords),
		"teacher_samples": len(teacherRecords),
		"exportable":      exportable,
		"skipped":         skipped,
		"labels":          labels,
		"coverage": map[string]any{
			"classes":               covered,
			"min_class_samples":     minClass,
			"max_class_share":       share,
			"max_class_share_name":  maxClassName,
			"max_class_share_limit": GATE_MAX_CLASS_SHARE,
			"balanced":              balanced,
			"ready":                 readyForTraining(exportable, covered, minClass) && balanced,
		},
		"thresholds": map[string]any{
			"min_samples":       GATE_MIN_SAMPLES,
			"min_classes":       GATE_MIN_CLASSES,
			"min_class_samples": GATE_MIN_CLASS_SAMPLES,
			"max_class_share":   GATE_MAX_CLASS_SHARE,
			"min_accuracy":      GATE_MIN_ACCURACY,
		},
		"model": map[string]any{
			"installed": fileExists(modelPath),
			"path":      modelPath,
		},
		"gate_version": s.gateVersion(),
	}
	if autoErr != nil {
		body["auto_error"] = autoErr.Error()
	}
	if teacherErr != nil {
		body["teacher_error"] = teacherErr.Error()
	}
	return body
}

// readyForTraining 判断是否满足样本门槛与类别覆盖门槛。
func readyForTraining(exportable, covered, minClass int) bool {
	return exportable >= GATE_MIN_SAMPLES && covered >= GATE_MIN_CLASSES && minClass >= GATE_MIN_CLASS_SAMPLES
}

// gateVersion 返回当前门控自报版本（未装配时为空）。
func (s *Server) gateVersion() string {
	if s.assembler == nil || s.assembler.Gate == nil {
		return ""
	}
	return gate.VersionOf(s.assembler.Gate)
}

// gateTeach 校验并追加教师样本；任一条非法则整批拒绝，避免半批落地。
func (s *Server) gateTeach(args gateTeachArgs) (*mcp.CallToolResult, any, error) {
	if len(args.Samples) == 0 {
		return fail("至少提交 1 条教师样本")
	}
	if len(args.Samples) > GATE_TEACH_MAX_SAMPLES {
		return fail("一次最多 %d 条教师样本，当前 %d 条", GATE_TEACH_MAX_SAMPLES, len(args.Samples))
	}
	now := time.Now()
	records := make([]samples.Record, 0, len(args.Samples))
	for index, item := range args.Samples {
		label := strings.TrimSpace(item.TargetExpert)
		if label == gate.PHRASING_EXPERT {
			return fail("第 %d 条：表达偏好不参与分类，不能作为目标专家", index+1)
		}
		if gate.SemanticExpertIndex(label) < 0 {
			return fail("第 %d 条：目标专家 %q 不在 4 类语义专家里（%s）",
				index+1, label, strings.Join(gate.SEMANTIC_EXPERTS, "/"))
		}
		kind := strings.TrimSpace(item.EventKind)
		if kind == "" {
			kind = "turn"
		}
		if !gate.IsKnownEventKind(kind) {
			return fail("第 %d 条：未知事件类型 %q（可选 relationship/conflict/apology/question/knowledge/emotion/self/event/turn）", index+1, kind)
		}
		features := gate.Features{
			EventKind: kind,
			Summary:   strings.TrimSpace(item.Summary),
			Affect:    item.Affect,
			HardState: hardStateFromKeys(item.HardStateKeys),
		}
		record := samples.NewRecord(features, []string{label, gate.PHRASING_EXPERT}, now)
		record.Primary = label
		record.Source = TEACHER_SOURCE
		records = append(records, record)
	}
	s.teachMu.Lock()
	defer s.teachMu.Unlock()
	path := samples.Path(s.dataDir, samples.TEACHER_FILE)
	writer, err := samples.NewWriter(path)
	if err != nil {
		return fail("打开教师样本文件失败: %s", err)
	}
	defer writer.Close()
	for index, record := range records {
		if err := writer.Write(record); err != nil {
			return fail("写入第 %d 条教师样本失败: %s", index+1, err)
		}
	}
	return jsonResult(map[string]any{
		"ok":       true,
		"appended": len(records),
		"path":     path,
		"note":     "教师样本已落地；导出训练集时会与自动样本合并",
	})
}

// hardStateFromKeys 把键列表还原成门控 HardState（值只需存在）。
func hardStateFromKeys(keys []string) map[string]any {
	if len(keys) == 0 {
		return nil
	}
	state := make(map[string]any, len(keys))
	for _, key := range keys {
		trimmed := strings.TrimSpace(key)
		if trimmed != "" {
			state[trimmed] = nil
		}
	}
	if len(state) == 0 {
		return nil
	}
	return state
}

// gateTrain 跑一键训练脚本：默认 dry-run，deploy 才安装权重。
func (s *Server) gateTrain(ctx context.Context, args gateTrainArgs) (*mcp.CallToolResult, any, error) {
	script, root, err := s.trainingScript()
	if err != nil {
		return fail("%s", err)
	}
	trainBinary := filepath.Join(root, "bin", "astrbox-train")
	if !fileExists(trainBinary) {
		return fail("缺少 %s，先在仓库根目录 make build", trainBinary)
	}
	argv := []string{
		script,
		"--data-dir", s.dataDir,
		"--min-samples", strconv.Itoa(clampPositive(args.MinSamples, GATE_MIN_SAMPLES)),
		"--min-classes", strconv.Itoa(clampPositive(args.MinClasses, GATE_MIN_CLASSES)),
		"--min-class-samples", strconv.Itoa(clampPositive(args.MinClassSamples, GATE_MIN_CLASS_SAMPLES)),
		"--max-class-share", strconv.FormatFloat(clampClassShare(args.MaxClassShare), 'f', -1, 64),
		"--min-accuracy", strconv.FormatFloat(clampAccuracy(args.MinAccuracy), 'f', -1, 64),
		"--epochs", strconv.Itoa(clampPositive(args.Epochs, GATE_TRAIN_EPOCHS)),
	}
	if ita := strings.TrimSpace(args.ITADir); ita != "" {
		argv = append(argv, "--ita-dir", ita)
	}
	if !args.Deploy {
		argv = append(argv, "--dry-run")
	}
	s.trainMu.Lock()
	defer s.trainMu.Unlock()
	trainCtx, cancel := context.WithTimeout(ctx, GATE_TRAIN_TIMEOUT)
	defer cancel()
	command := exec.CommandContext(trainCtx, "bash", argv...)
	command.Dir = root
	output, runErr := command.CombinedOutput()
	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	note := "dry-run：未安装权重"
	if args.Deploy {
		note = "权重已安装（门槛全过才会安装）；重启 dsh-mcp 后生效"
	}
	return jsonResult(map[string]any{
		"ok":          runErr == nil,
		"deploy":      args.Deploy,
		"exit_code":   exitCode,
		"script":      script,
		"output_tail": tailLines(string(output), 60),
		"note":        note,
	})
}

// trainingScript 定位仓库里的训练脚本，返回脚本路径与仓库根目录。
func (s *Server) trainingScript() (script string, root string, err error) {
	candidates := make([]string, 0, 4)
	if envRoot := strings.TrimSpace(os.Getenv("NEKO_ROOT")); envRoot != "" {
		candidates = append(candidates, envRoot)
	}
	if executable, execErr := os.Executable(); execErr == nil {
		// 正常安装形态是 <root>/bin/dsh-mcp，父目录即仓库根。
		candidates = append(candidates, filepath.Dir(filepath.Dir(executable)))
	}
	if workingDir, cwdErr := os.Getwd(); cwdErr == nil {
		candidates = append(candidates, workingDir)
	}
	for _, candidate := range candidates {
		path := filepath.Join(candidate, "scripts", "tools", "trainGateModel.sh")
		if fileExists(path) {
			return path, candidate, nil
		}
	}
	return "", "", fmt.Errorf("找不到 scripts/tools/trainGateModel.sh（可用环境变量 NEKO_ROOT 指定仓库根目录）")
}

// clampPositive 把非正参数回落到默认值。
func clampPositive(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

// clampClassShare 把非法类别占比门槛回落到默认值（合法范围 (0,1]）。
func clampClassShare(value float64) float64 {
	if value <= 0 || value > 1 {
		return GATE_MAX_CLASS_SHARE
	}
	return value
}

// clampAccuracy 把非正/越界准确率回落到默认值。
func clampAccuracy(value float64) float64 {
	if value <= 0 || value > 1 {
		return GATE_MIN_ACCURACY
	}
	return value
}

// tailLines 保留输出末尾若干行，避免把整份训练日志塞回工具响应。
func tailLines(text string, limit int) string {
	trimmed := strings.TrimRight(text, "\n")
	if trimmed == "" {
		return ""
	}
	lines := strings.Split(trimmed, "\n")
	if limit > 0 && len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return strings.Join(lines, "\n")
}

// fileExists 是最小存在性判断（工具层用，不区分文件/目录用途）。
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
