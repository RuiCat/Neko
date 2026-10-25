// Package bootstrap assembles the shared AstrBox runtime for every MCP entrypoint.
package bootstrap

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/ruicat/astrbox/internal/config"
	"github.com/ruicat/astrbox/internal/diagnostics"
	"github.com/ruicat/astrbox/internal/engine"
	"github.com/ruicat/astrbox/internal/entity"
	"github.com/ruicat/astrbox/internal/events"
	"github.com/ruicat/astrbox/internal/function"
	"github.com/ruicat/astrbox/internal/mcpserver"
	"github.com/ruicat/astrbox/internal/registry"
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
	DataDir     string
	ScriptsDir  string
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

	return &Runtime{
		Config:      domainConfig,
		Store:       store,
		Machine:     machine,
		Funcs:       funcs,
		Events:      eventSystem,
		Registry:    reg,
		Diagnostics: diagStore,
		StateLogger: engine.NewStateLogger(filepath.Join(options.DataDir, "logs"), 7),
		DataDir:     options.DataDir,
		ScriptsDir:  options.ScriptsDir,
	}, nil
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
	return server
}

func loadRules(path string) ([]engine.Rule, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return engine.ParseRules(raw)
}
