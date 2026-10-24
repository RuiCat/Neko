// Package registry applies, removes and replays .neko scripts with reference
// counting, persisting entries in scripts.json.
package registry

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/ruicat/astrbox/internal/compiler"
	"github.com/ruicat/astrbox/internal/engine"
	"github.com/ruicat/astrbox/internal/entity"
	"github.com/ruicat/astrbox/internal/neko"
	"github.com/ruicat/astrbox/internal/safety"
)

// Entry is one persisted script registration.
type Entry struct {
	Hash      string          `json:"hash"`
	Templates map[string]int  `json:"templates"`
	Rules     map[string]int  `json:"rules"`
	Funcs     []string        `json:"funcs"`
	Events    []string        `json:"events"`
	FuncDefs  []FuncDefRecord `json:"func_defs"`
	EventDefs []neko.EventDef `json:"event_defs"`
}

// FuncDefRecord is the persisted form of a function definition. Params use the
// Python-compatible [[name, type], ...] encoding.
type FuncDefRecord struct {
	Name       string     `json:"name"`
	Params     [][]string `json:"params"`
	ReturnType string     `json:"return_type,omitempty"`
	Body       string     `json:"body"`
}

// ApplyError is a JSON-friendly error report.
type ApplyError struct {
	Severity string `json:"severity"`
	Line     int    `json:"line,omitempty"`
	Block    string `json:"block,omitempty"`
	Message  string `json:"message"`
	Hint     string `json:"fix_hint,omitempty"`
}

// ApplyResult mirrors the Python registry result shape.
type ApplyResult struct {
	OK       bool          `json:"ok"`
	Script   string        `json:"script"`
	Applied  AppliedCounts `json:"applied"`
	Removed  AppliedCounts `json:"removed"`
	Errors   []ApplyError  `json:"errors"`
	Warnings []ApplyError  `json:"warnings"`
	Noop     bool          `json:"noop"`
}

// AppliedCounts lists the names touched by an apply or remove.
type AppliedCounts struct {
	Templates []string `json:"templates"`
	Rules     []string `json:"rules"`
	Funcs     []string `json:"funcs,omitempty"`
	Events    []string `json:"events,omitempty"`
}

// FunctionSink is the event/function subsystem the registry feeds.
type FunctionSink interface {
	RegisterFuncs(defs []neko.FuncDef)
	UnregisterFuncs(names []string)
	RegisterEvents(defs []neko.EventDef)
	UnregisterEvents(names []string)
	Refresh()
}

// Registry owns the script entries and the source directory.
type Registry struct {
	mu         sync.Mutex
	store      *entity.Store
	engine     *engine.StateMachine
	path       string
	scriptsDir string
	entries    map[string]*Entry
	order      []string
	sources    map[string]string
	sink       FunctionSink
}

var scriptNamePattern = regexp.MustCompile(`^[\p{L}\p{N}_-]+$`)

// New loads scripts.json and returns a registry.
func New(store *entity.Store, machine *engine.StateMachine, path, scriptsDir string) (*Registry, error) {
	reg := &Registry{
		store: store, engine: machine, path: path, scriptsDir: scriptsDir,
		entries: map[string]*Entry{},
	}
	if err := reg.load(); err != nil {
		return nil, err
	}
	reg.indexSources()
	return reg, nil
}

// indexSources maps script names to their source path (scripts may be nested).
func (r *Registry) indexSources() {
	r.sources = map[string]string{}
	_ = filepath.WalkDir(r.scriptsDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(entry.Name(), ".neko") {
			return nil
		}
		name := strings.TrimSuffix(entry.Name(), ".neko")
		if _, exists := r.sources[name]; !exists {
			r.sources[name] = path
		}
		return nil
	})
}

// sourcePath returns the source file for a script, defaulting to the root.
func (r *Registry) sourcePath(name string) string {
	if path, ok := r.sources[name]; ok {
		return path
	}
	return filepath.Join(r.scriptsDir, name+".neko")
}

func (r *Registry) load() error {
	raw, err := os.ReadFile(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var payload struct {
		Scripts map[string]*Entry `json:"scripts"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("scripts.json 解析失败（未修改原文件）: %w", err)
	}
	if payload.Scripts != nil {
		r.entries = payload.Scripts
	}
	r.order = scriptOrder(raw)
	return nil
}

// Save atomically writes scripts.json.
func (r *Registry) Save() error {
	raw, err := marshalScripts(r.order, r.entries)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, r.path)
}

// List returns the sorted script names.
func (r *Registry) List() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	names := make([]string, 0, len(r.entries))
	for name := range r.entries {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// SetFunctionSink wires @函数/@事件 registration (optional).
func (r *Registry) SetFunctionSink(sink FunctionSink) { r.sink = sink }

// Get returns one entry.
func (r *Registry) Get(name string) *Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.entries[name]
}

// Apply parses and compiles a script, replacing any previous registration.
func (r *Registry) Apply(name, text string, force, skipSafety bool) *ApplyResult {
	return r.applyScript(name, text, force, skipSafety, true)
}

// applyScript is the shared implementation; persist=false is used by replay so
// a read-only startup does not rewrite scripts.json.
func (r *Registry) applyScript(name, text string, force, skipSafety, persist bool) (result *ApplyResult) {
	result = &ApplyResult{OK: true, Script: name, Errors: []ApplyError{}, Warnings: []ApplyError{}}
	defer func() {
		if recovered := recover(); recovered != nil {
			result.OK = false
			result.Errors = append(result.Errors, ApplyError{Severity: "error", Message: fmt.Sprintf("编译或应用脚本时 panic: %v", recovered)})
		}
	}()
	if err := validateName(name); err != "" {
		result.OK = false
		result.Errors = append(result.Errors, ApplyError{Severity: "error", Block: "安全校验", Message: err})
		return result
	}
	newHash := textHash(text)
	r.mu.Lock()
	old := r.entries[name]
	r.mu.Unlock()
	if old != nil && old.Hash == newHash && !force {
		result.Noop = true
		return result
	}
	ast := neko.Parse(text)
	for _, err := range ast.Errors {
		// Mirrors the Python registry: a malformed data block is reported but does
		// not stop the rules in the same script from being applied.
		result.Warnings = append(result.Warnings, ApplyError{Severity: "warning", Message: err.Error()})
	}
	if !skipSafety && safety.Enabled() {
		violations := safety.CheckAST(ast)
		if len(violations) == 0 {
			violations = safety.CheckScriptText(text, name)
		}
		if len(violations) > 0 {
			result.OK = false
			result.Errors = append(result.Errors, ApplyError{
				Severity: "error", Block: "R18G安全检查",
				Message: fmt.Sprintf("脚本 '%s' 含 R18G 敏感内容，已拒绝应用（%d 处）", name, len(violations)),
			})
			return result
		}
	}
	compiled := compiler.CompileScript(ast)
	if len(compiled.Errors) > 0 {
		result.OK = false
		for _, err := range compiled.Errors {
			result.Errors = append(result.Errors, ApplyError{Severity: "error", Message: err.Error()})
		}
		return result
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if old != nil {
		delete(r.entries, name)
		r.releaseLocked(old, result, persist)
	}
	newEntry := &Entry{
		Hash: newHash, Templates: map[string]int{}, Rules: map[string]int{},
		Funcs: []string{}, Events: []string{},
		FuncDefs: toFuncRecords(compiled.Funcs), EventDefs: compiled.Events,
	}
	for typeName, data := range compiled.Templates {
		r.store.AddTemplate(typeName, data)
		if persist {
			if err := r.store.SaveTemplateFile(typeName); err != nil {
				result.Warnings = append(result.Warnings, ApplyError{Severity: "warning", Message: "模板落盘失败: " + err.Error()})
			}
		}
		newEntry.Templates[typeName] = 1
		result.Applied.Templates = append(result.Applied.Templates, typeName)
	}
	for _, rule := range compiled.Rules {
		r.engine.AddRule(rule)
		newEntry.Rules[rule.ID] = 1
		result.Applied.Rules = append(result.Applied.Rules, rule.ID)
	}
	for _, def := range compiled.Funcs {
		newEntry.Funcs = append(newEntry.Funcs, def.Name)
	}
	for _, def := range compiled.Events {
		newEntry.Events = append(newEntry.Events, def.Name)
	}
	sort.Strings(newEntry.Funcs)
	sort.Strings(newEntry.Events)
	if _, exists := r.entries[name]; !exists {
		r.order = append(r.order, name)
	}
	r.entries[name] = newEntry
	if r.sink != nil {
		r.sink.RegisterFuncs(compiled.Funcs)
		r.sink.RegisterEvents(compiled.Events)
		r.sink.Refresh()
	}
	if persist {
		if err := r.Save(); err != nil {
			result.Errors = append(result.Errors, ApplyError{Severity: "error", Message: "保存注册表失败: " + err.Error()})
			result.OK = false
		}
		r.writeSourceLocked(name, text)
	}
	return result
}

// Remove drops a script and releases its templates and rules.
func (r *Registry) Remove(name string) *ApplyResult {
	result := &ApplyResult{OK: true, Script: name, Errors: []ApplyError{}, Warnings: []ApplyError{}}
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.entries[name]
	if !ok {
		return result
	}
	delete(r.entries, name)
	r.order = removeName(r.order, name)
	r.releaseLocked(entry, result, true)
	if r.sink != nil {
		r.sink.UnregisterFuncs(entry.Funcs)
		r.sink.UnregisterEvents(entry.Events)
		r.sink.Refresh()
	}
	_ = r.Save()
	_ = os.Remove(r.sourcePath(name))
	return result
}

func (r *Registry) releaseLocked(entry *Entry, result *ApplyResult, persist bool) {
	for typeName := range entry.Templates {
		if r.refCountLocked("template", typeName) == 0 {
			if r.store.RemoveTemplate(typeName) {
				if persist {
					_ = r.store.DeleteTemplateFile(typeName)
				}
				result.Removed.Templates = append(result.Removed.Templates, typeName)
			}
		}
	}
	for ruleID := range entry.Rules {
		if r.refCountLocked("rule", ruleID) == 0 {
			if r.engine.RemoveRule(ruleID) {
				result.Removed.Rules = append(result.Removed.Rules, ruleID)
			}
		}
	}
}

func (r *Registry) refCountLocked(kind, name string) int {
	count := 0
	for _, entry := range r.entries {
		if kind == "template" {
			count += entry.Templates[name]
		} else {
			count += entry.Rules[name]
		}
	}
	return count
}

func (r *Registry) writeSourceLocked(name, text string) {
	if r.scriptsDir == "" || validateName(name) != "" {
		return
	}
	if err := os.MkdirAll(r.scriptsDir, 0o755); err != nil {
		return
	}
	path := r.sourcePath(name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(text), 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}

// BootstrapFromSources registers every top-level .neko source when the registry
// is empty (fresh clone: data/ is local-only, sources are versioned).
func (r *Registry) BootstrapFromSources() (ReplayResult, error) {
	out := ReplayResult{Replayed: []string{}, Skipped: []string{}, Failed: []string{}, Details: map[string][]string{}}
	r.indexSources()
	names := make([]string, 0, len(r.sources))
	for name := range r.sources {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, scriptName := range names {
		raw, err := os.ReadFile(r.sourcePath(scriptName))
		if err != nil {
			out.Failed = append(out.Failed, scriptName)
			out.Details[scriptName] = append(out.Details[scriptName], err.Error())
			continue
		}
		result := r.Apply(scriptName, string(raw), true, true)
		if result.OK {
			out.Replayed = append(out.Replayed, scriptName)
			continue
		}
		out.Failed = append(out.Failed, scriptName)
		for _, item := range result.Errors {
			out.Details[scriptName] = append(out.Details[scriptName], item.Message)
		}
	}
	return out, nil
}

// ReplayResult reports a replay run.
type ReplayResult struct {
	Replayed []string            `json:"replayed"`
	Skipped  []string            `json:"skipped"`
	Failed   []string            `json:"failed"`
	Details  map[string][]string `json:"details,omitempty"`
}

// ReplayAll re-applies every registered script from its source file.
func (r *Registry) ReplayAll() ReplayResult {
	out := ReplayResult{Replayed: []string{}, Skipped: []string{}, Failed: []string{}, Details: map[string][]string{}}
	for _, name := range r.List() {
		if validateName(name) != "" {
			out.Failed = append(out.Failed, name)
			continue
		}
		path := r.sourcePath(name)
		raw, err := os.ReadFile(path)
		if err != nil {
			out.Skipped = append(out.Skipped, name)
			continue
		}
		result := r.applyScript(name, string(raw), true, true, false)
		if result.OK {
			out.Replayed = append(out.Replayed, name)
		} else {
			out.Failed = append(out.Failed, name)
			for _, item := range result.Errors {
				out.Details[name] = append(out.Details[name], item.Message)
			}
		}
	}
	return out
}

// scriptOrder extracts the top-level scripts key order from scripts.json.
func scriptOrder(raw []byte) []string {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if _, err := decoder.Token(); err != nil {
		return nil
	}
	order := []string{}
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return order
		}
		key, _ := keyToken.(string)
		if key != "scripts" {
			var skip any
			_ = decoder.Decode(&skip)
			continue
		}
		if _, err := decoder.Token(); err != nil {
			return order
		}
		for decoder.More() {
			nameToken, err := decoder.Token()
			if err != nil {
				return order
			}
			name, _ := nameToken.(string)
			order = append(order, name)
			var skip any
			_ = decoder.Decode(&skip)
		}
	}
	return order
}

// marshalScripts writes scripts.json while preserving the loaded key order.
func marshalScripts(order []string, entries map[string]*Entry) ([]byte, error) {
	names := make([]string, 0, len(entries))
	seen := map[string]bool{}
	for _, name := range order {
		if _, ok := entries[name]; ok && !seen[name] {
			names = append(names, name)
			seen[name] = true
		}
	}
	extras := make([]string, 0)
	for name := range entries {
		if !seen[name] {
			extras = append(extras, name)
		}
	}
	sort.Strings(extras)
	names = append(names, extras...)

	var builder bytes.Buffer
	builder.WriteString("{\n  \"version\": 1,\n  \"scripts\": {")
	for index, name := range names {
		raw, err := json.Marshal(entries[name])
		if err != nil {
			return nil, err
		}
		if index > 0 {
			builder.WriteString(",")
		}
		nameRaw, _ := json.Marshal(name)
		builder.WriteString("\n    ")
		builder.Write(nameRaw)
		builder.WriteString(": ")
		builder.Write(raw)
	}
	builder.WriteString("\n  }\n}\n")
	return builder.Bytes(), nil
}

func removeName(names []string, target string) []string {
	out := names[:0]
	for _, name := range names {
		if name != target {
			out = append(out, name)
		}
	}
	return out
}

func toFuncRecords(defs []neko.FuncDef) []FuncDefRecord {
	out := make([]FuncDefRecord, 0, len(defs))
	for _, def := range defs {
		params := make([][]string, 0, len(def.Params))
		for _, param := range def.Params {
			params = append(params, []string{param.Name, param.Type})
		}
		out = append(out, FuncDefRecord{Name: def.Name, Params: params, ReturnType: def.ReturnType, Body: def.Body})
	}
	return out
}

func textHash(text string) string {
	sum := sha1.Sum([]byte(text))
	return hex.EncodeToString(sum[:])
}

func validateName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "脚本名不能为空"
	}
	if !scriptNamePattern.MatchString(name) {
		return fmt.Sprintf("非法脚本名 '%s'：仅允许字母数字下划线、中文与连字符", name)
	}
	return ""
}
