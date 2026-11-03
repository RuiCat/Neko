// Package bootstrap assembles the shared AstrBox runtime for every MCP entrypoint.
package bootstrap

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/RuiCat/Neko/internal/blackboard"
	"github.com/RuiCat/Neko/internal/compensation"
	"github.com/RuiCat/Neko/internal/config"
	"github.com/RuiCat/Neko/internal/diagnostics"
	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/entity"
	"github.com/RuiCat/Neko/internal/events"
	"github.com/RuiCat/Neko/internal/expert"
	"github.com/RuiCat/Neko/internal/function"
	"github.com/RuiCat/Neko/internal/gate"
	"github.com/RuiCat/Neko/internal/hardstate"
	"github.com/RuiCat/Neko/internal/journal"
	"github.com/RuiCat/Neko/internal/mcpserver"
	"github.com/RuiCat/Neko/internal/registry"
)

// Options selects the runtime locations. Empty strings follow the historical
// flag defaults: the current directory for config, data and scripts.
type Options struct {
	// DataDir holds entities.json, templates/, archives/ and logs/.
	DataDir string
	// ScriptsDir holds scripts.json, *.neko and rules.json.
	ScriptsDir string
	// ConfigPath is the domain config JSON; a missing file means defaults.
	ConfigPath string
	// Journal enables per-turn journaling under <dataDir>/journal. It is opt-in
	// so tests and embedders that do not want a background writer stay
	// unchanged (the writer goroutine would race temp-dir cleanup).
	Journal bool
	// NightlyAt enables the in-process daily decay at local "HH:MM"; empty
	// disables it.
	NightlyAt string
	// NightlyFactor is the decay factor; 0 uses DEFAULT_NIGHTLY_FACTOR.
	NightlyFactor float64
}

// Runtime is the assembled engine shared by astrbox-mcp and dsh-mcp. It owns
// one store, engine, event system, registry, diagnostics store and state logger.
type Runtime struct {
	Config      config.Config
	Store       *entity.Store
	Machine     *engine.StateMachine
	Funcs       *function.Executor
	Events      *events.System
	Registry    *registry.Registry
	Diagnostics *diagnostics.Store
	StateLogger *engine.StateLogger
	Journal     *journal.Writer
	// JournalSequence issues snapshot IDs and is seeded from the journal.
	JournalSequence *hardstate.Sequence
	// HardState defines which fields the snapshot captures.
	HardState hardstate.Schema
	Board     *blackboard.Board
	Experts   *expert.Registry
	// ExpertsPath is where the expert contexts are persisted.
	ExpertsPath string
	Assembler   *compensation.Assembler
	DataDir     string
	ScriptsDir  string
	nightlyStop chan struct{}
	nightlyDone chan struct{}
}

// New loads config, data and scripts, replays scripts and assembles a Runtime.
// Fatal load failures are returned; recoverable ones are logged and skipped,
// matching the historical astrbox-mcp startup behavior.
func New(options Options) (*Runtime, error) {
	domainConfig, err := config.Load(options.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("配置加载失败: %v", err)
	}
	if err := config.Set(domainConfig); err != nil {
		return nil, fmt.Errorf("配置无效: %v", err)
	}

	store := entity.NewStore(options.DataDir)
	if err := store.Load(); err != nil {
		return nil, fmt.Errorf("加载数据目录失败: %v", err)
	}
	if err := store.MigrateUserEntities(); err != nil {
		log.Printf("astrbox: 拆分用户实体失败: %v", err)
	}
	machine := engine.New(store)
	machine.SetDecisionLog(filepath.Join(options.DataDir, "llm_decisions.json"))
	funcs := function.New(store)
	eventSystem := events.NewSystem(store, funcs)
	machine.SetFuncCaller(funcs)

	if rules, err := loadRules(filepath.Join(options.ScriptsDir, "rules.json")); err != nil {
		log.Printf("astrbox: rules.json 跳过: %v", err)
	} else if len(rules) > 0 {
		machine.AddRules(rules)
	}

	reg, err := registry.New(store, machine, filepath.Join(options.DataDir, "scripts.json"), options.ScriptsDir)
	if err != nil {
		return nil, fmt.Errorf("加载脚本注册表失败: %v", err)
	}
	reg.SetFunctionSink(eventSystem)

	diagStore := diagnostics.New(diagnostics.DEFAULT_CAPACITY)
	reg.SetDiagnostics(diagStore)

	replay := reg.ReplayAll()
	if len(reg.List()) == 0 {
		seeded, err := reg.BootstrapFromSources()
		if err != nil {
			log.Printf("astrbox: 从 scripts/ 引导失败: %v", err)
		} else if len(seeded.Replayed) > 0 || len(seeded.Failed) > 0 {
			log.Printf("astrbox: 注册表为空，已从 scripts/ 引导 %d 个脚本 (failed=%d)",
				len(seeded.Replayed), len(seeded.Failed))
		}
	}
	log.Printf("astrbox: 实体 %d, 模板 %d, 规则 %d, 脚本 %d (replayed=%d skipped=%d failed=%d)",
		store.Count(), len(store.ListTemplates()), len(machine.Rules()), len(reg.List()),
		len(replay.Replayed), len(replay.Skipped), len(replay.Failed))
	if len(replay.Failed) > 0 {
		log.Printf("astrbox: 重放失败脚本: %v", replay.Failed)
		for name, details := range replay.Details {
			log.Printf("  - %s: %v", name, details)
		}
	}
	if findings := reg.ScanConsistency(); findings > 0 {
		log.Printf("astrbox: 脚本一致性检查发现 %d 个问题（可通过 diagnostics_query 查看）", findings)
	}

	journalDir := filepath.Join(options.DataDir, "journal")
	sequence := hardstate.NewSequence()
	if maxSeq, err := journal.MaxSequence(journalDir, time.Now()); err != nil {
		log.Printf("astrbox: 恢复交互日志序号失败: %v", err)
	} else {
		sequence.SetLast(maxSeq)
	}
	var journalWriter *journal.Writer
	if options.Journal {
		journalWriter = journal.NewWriter(journalDir)
	}
	board := blackboard.New()
	experts := expert.NewDefaultRegistry(expert.DEFAULT_CONTEXT_CAPACITY)
	expertsPath := filepath.Join(options.DataDir, "experts.json")
	if err := expert.LoadFile(expertsPath, experts); err != nil {
		log.Printf("astrbox: 恢复专家上下文失败: %v", err)
	}
	assembler := compensation.NewAssembler(gate.NewRuleGate(), experts)

	runtime := &Runtime{
		Config:          domainConfig,
		Store:           store,
		Machine:         machine,
		Funcs:           funcs,
		Events:          eventSystem,
		Registry:        reg,
		Diagnostics:     diagStore,
		StateLogger:     engine.NewStateLogger(filepath.Join(options.DataDir, "logs"), 7),
		Journal:         journalWriter,
		JournalSequence: sequence,
		HardState:       hardstate.DefaultSchema(),
		Board:           board,
		Experts:         experts,
		ExpertsPath:     expertsPath,
		Assembler:       assembler,
		DataDir:         options.DataDir,
		ScriptsDir:      options.ScriptsDir,
	}
	runtime.startNightly(options)
	return runtime, nil
}

// Close persists expert contexts and flushes the journal writer. Callers that
// own the runtime should call it on shutdown; records queued after Close are
// rejected with an error.
func (r *Runtime) Close() error {
	r.stopNightly()
	if err := r.SaveExperts(); err != nil {
		return err
	}
	if r.Journal == nil {
		return nil
	}
	return r.Journal.Close()
}

// SaveExperts persists the experts' working memory to <data-dir>/experts.json.
func (r *Runtime) SaveExperts() error {
	if r.Experts == nil || r.ExpertsPath == "" {
		return nil
	}
	return expert.SaveFile(r.ExpertsPath, r.Experts)
}

// NewMCPServer wires the runtime into an MCP tool server, keeping the tool
// registry and result conventions of astrbox-mcp unchanged.
func (r *Runtime) NewMCPServer() *mcpserver.Server {
	server := mcpserver.New(r.Store, r.Machine, r.Registry)
	server.SetDataDir(r.DataDir)
	server.SetEventBridge(r.Events.Bridge())
	server.SetEventBus(r.Events.Bus())
	server.SetDiagnostics(r.Diagnostics)
	server.SetStateLogger(r.StateLogger)
	if r.Journal != nil {
		server.SetJournal(r.Journal, r.JournalSequence, r.HardState)
	}
	if r.Assembler != nil {
		server.SetLearningLoop(r.Board, r.Assembler)
	}
	return server
}

func loadRules(path string) ([]engine.Rule, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return engine.ParseRules(raw)
}
