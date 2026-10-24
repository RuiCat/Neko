package registry

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ruicat/astrbox/internal/compiler"
	"github.com/ruicat/astrbox/internal/diagnostics"
	"github.com/ruicat/astrbox/internal/neko"
	"github.com/ruicat/astrbox/internal/safety"
)

// preparedScript is the side-effect-free result of parsing, safety-checking and
// compiling one script. It is shared by Apply, WriteScript and ValidateScript so
// the three paths cannot drift apart.
type preparedScript struct {
	Hash     string
	Compiled *compiler.Result
	Errors   []ApplyError
	Warnings []ApplyError
}

// newApplyResult builds an empty successful result for one script.
func newApplyResult(name string) *ApplyResult {
	return &ApplyResult{OK: true, Script: name, Errors: []ApplyError{}, Warnings: []ApplyError{}}
}

// prepareScript parses, safety-checks and compiles text without touching state.
func (r *Registry) prepareScript(name, text string, skipSafety bool) *preparedScript {
	prepared := &preparedScript{Hash: textHash(text)}
	ast := neko.Parse(text)
	for _, err := range ast.Errors {
		// Mirrors the Python registry: a malformed data block is reported but does
		// not stop the rules in the same script from being applied.
		prepared.Warnings = append(prepared.Warnings, ApplyError{Severity: "warning", Message: err.Error()})
	}
	if !skipSafety && safety.Enabled() {
		violations := safety.CheckAST(ast)
		if len(violations) == 0 {
			violations = safety.CheckScriptText(text, name)
		}
		if len(violations) > 0 {
			prepared.Errors = append(prepared.Errors, ApplyError{
				Severity: "error", Block: "R18G安全检查",
				Message: fmt.Sprintf("脚本 '%s' 含 R18G 敏感内容，已拒绝应用（%d 处）", name, len(violations)),
			})
			return prepared
		}
	}
	compiled := compiler.CompileScript(ast)
	if len(compiled.Errors) > 0 {
		for _, err := range compiled.Errors {
			prepared.Errors = append(prepared.Errors, ApplyError{Severity: "error", Message: err.Error()})
		}
		return prepared
	}
	prepared.Compiled = compiled
	return prepared
}

// commitLocked registers a prepared script. The caller must hold r.mu.
func (r *Registry) commitLocked(result *ApplyResult, name, text, newHash string, prepared *preparedScript, persist bool) {
	if old := r.entries[name]; old != nil {
		delete(r.entries, name)
		r.releaseLocked(old, result, persist)
	}
	newEntry := &Entry{
		Hash: newHash, Templates: map[string]int{}, Rules: map[string]int{},
		Funcs: []string{}, Events: []string{},
		FuncDefs: toFuncRecords(prepared.Compiled.Funcs), EventDefs: prepared.Compiled.Events,
	}
	names := make([]string, 0, len(prepared.Compiled.Templates))
	for typeName := range prepared.Compiled.Templates {
		names = append(names, typeName)
	}
	sort.Strings(names)
	for _, typeName := range names {
		r.store.AddTemplate(typeName, prepared.Compiled.Templates[typeName])
		if persist {
			if err := r.store.SaveTemplateFile(typeName); err != nil {
				result.Warnings = append(result.Warnings, ApplyError{Severity: "warning", Message: "模板落盘失败: " + err.Error()})
			}
		}
		newEntry.Templates[typeName] = 1
		result.Applied.Templates = append(result.Applied.Templates, typeName)
	}
	for _, rule := range prepared.Compiled.Rules {
		r.engine.AddRule(rule)
		newEntry.Rules[rule.ID] = 1
		result.Applied.Rules = append(result.Applied.Rules, rule.ID)
	}
	for _, def := range prepared.Compiled.Funcs {
		newEntry.Funcs = append(newEntry.Funcs, def.Name)
	}
	for _, def := range prepared.Compiled.Events {
		newEntry.Events = append(newEntry.Events, def.Name)
	}
	sort.Strings(newEntry.Funcs)
	sort.Strings(newEntry.Events)
	if _, exists := r.entries[name]; !exists {
		r.order = append(r.order, name)
	}
	r.entries[name] = newEntry
	if r.sink != nil {
		r.sink.RegisterFuncs(prepared.Compiled.Funcs)
		r.sink.RegisterEvents(prepared.Compiled.Events)
		r.sink.Refresh()
	}
	if persist {
		if err := r.Save(); err != nil {
			result.Errors = append(result.Errors, ApplyError{Severity: "error", Message: "保存注册表失败: " + err.Error()})
			result.OK = false
		}
		if err := r.writeSourceLocked(name, text); err != nil {
			result.Warnings = append(result.Warnings, ApplyError{Severity: "warning", Message: "源文件写入失败: " + err.Error()})
		}
	}
}

// recordApplyResult mirrors one apply outcome into the diagnostic store.
func (r *Registry) recordApplyResult(result *ApplyResult, persist bool) {
	if r.diagnostics == nil || result == nil {
		return
	}
	if result.OK && !result.Noop {
		r.diagnostics.ClearByScript(result.Script)
	}
	for _, item := range result.Errors {
		code := "script.apply.failed"
		if !persist {
			code = "script.replay.failed"
		}
		if strings.Contains(item.Block, "安全") {
			code = "script.safety.blocked"
		}
		r.diagnostics.Add(diagnostics.Record{
			Code: code, Severity: diagnostics.SEVERITY_ERROR, Source: "registry",
			Script: result.Script, Line: item.Line, Message: item.Message, Hint: item.Hint,
		})
	}
	for _, item := range result.Warnings {
		r.diagnostics.Add(diagnostics.Record{
			Code: "script.apply.warning", Severity: diagnostics.SEVERITY_WARNING, Source: "registry",
			Script: result.Script, Line: item.Line, Message: item.Message, Hint: item.Hint,
		})
	}
}

// recordSkipped reports a registered script whose source file is missing.
func (r *Registry) recordSkipped(name string, err error) {
	if r.diagnostics == nil {
		return
	}
	message := "源文件缺失"
	if err != nil {
		message += ": " + err.Error()
	}
	r.diagnostics.Add(diagnostics.Record{
		Code: "script.replay.skipped", Severity: diagnostics.SEVERITY_ERROR, Source: "registry",
		Script: name, Message: message, Hint: "恢复源文件或重新 apply",
	})
}
