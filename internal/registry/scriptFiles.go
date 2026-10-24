package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MAX_SCRIPT_BYTES bounds one script source read or written through MCP.
const MAX_SCRIPT_BYTES = 256 * 1024

// ScriptView is one script source plus its registration metadata.
type ScriptView struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Source     string `json:"source"`
	Hash       string `json:"hash"`
	ActualHash string `json:"actual_hash"`
	Entry      *Entry `json:"entry,omitempty"`
}

// ScriptInfo is one row of the script inventory.
type ScriptInfo struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Hash      string `json:"hash"`
	Rules     int    `json:"rules"`
	Templates int    `json:"templates"`
	Funcs     int    `json:"funcs"`
	Events    int    `json:"events"`
}

// ValidateResult is the side-effect-free result of compiling one script.
type ValidateResult struct {
	OK        bool         `json:"ok"`
	Errors    []ApplyError `json:"errors"`
	Warnings  []ApplyError `json:"warnings"`
	Templates []string     `json:"templates"`
	Rules     []string     `json:"rules"`
	Funcs     []string     `json:"funcs"`
	Events    []string     `json:"events"`
}

// ReadScript returns one script source without mutating the registry.
func (r *Registry) ReadScript(name string) (ScriptView, error) {
	path, err := r.safeSourcePath(name)
	if err != nil {
		return ScriptView{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return ScriptView{}, fmt.Errorf("读取脚本 '%s' 失败: %w", name, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return ScriptView{}, fmt.Errorf("拒绝读取符号链接脚本: %s", name)
	}
	if info.Size() > MAX_SCRIPT_BYTES {
		return ScriptView{}, fmt.Errorf("脚本 '%s' 超过 %d 字节上限", name, MAX_SCRIPT_BYTES)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return ScriptView{}, fmt.Errorf("读取脚本 '%s' 失败: %w", name, err)
	}
	r.mu.Lock()
	entry := r.entries[name]
	r.mu.Unlock()
	view := ScriptView{
		Name: name, Source: string(raw), ActualHash: textHash(string(raw)),
		Path: relativePath(r.scriptsDir, path),
	}
	if entry != nil {
		view.Hash = entry.Hash
		view.Entry = entry
	}
	return view, nil
}

// ListScripts returns registration metadata for every registered script.
func (r *Registry) ListScripts() []ScriptInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	names := make([]string, 0, len(r.entries))
	for name := range r.entries {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]ScriptInfo, 0, len(names))
	for _, name := range names {
		entry := r.entries[name]
		info := ScriptInfo{Name: name, Path: relativePath(r.scriptsDir, r.sourcePath(name))}
		if entry != nil {
			info.Hash = entry.Hash
			info.Rules = len(entry.Rules)
			info.Templates = len(entry.Templates)
			info.Funcs = len(entry.Funcs)
			info.Events = len(entry.Events)
		}
		out = append(out, info)
	}
	return out
}

// ValidateScript parses, safety-checks and compiles text without registering it.
func (r *Registry) ValidateScript(name, text string, skipSafety bool) *ValidateResult {
	result := &ValidateResult{OK: true, Errors: []ApplyError{}, Warnings: []ApplyError{}}
	if name != "" {
		if message := validateName(name); message != "" {
			result.OK = false
			result.Errors = append(result.Errors, ApplyError{Severity: "error", Block: "安全校验", Message: message})
			return result
		}
	}
	prepared := r.prepareScript(name, text, skipSafety)
	result.Warnings = append(result.Warnings, prepared.Warnings...)
	if len(prepared.Errors) > 0 {
		result.OK = false
		result.Errors = append(result.Errors, prepared.Errors...)
		return result
	}
	for typeName := range prepared.Compiled.Templates {
		result.Templates = append(result.Templates, typeName)
	}
	for _, rule := range prepared.Compiled.Rules {
		result.Rules = append(result.Rules, rule.ID)
	}
	for _, def := range prepared.Compiled.Funcs {
		result.Funcs = append(result.Funcs, def.Name)
	}
	for _, def := range prepared.Compiled.Events {
		result.Events = append(result.Events, def.Name)
	}
	sort.Strings(result.Templates)
	sort.Strings(result.Rules)
	sort.Strings(result.Funcs)
	sort.Strings(result.Events)
	return result
}

// WriteScript applies text only when expectedHash matches the registered hash.
// It is the transactional script-file entry point used by MCP clients.
func (r *Registry) WriteScript(name, text, expectedHash string) (result *ApplyResult) {
	result = newApplyResult(name)
	defer func() {
		if recovered := recover(); recovered != nil {
			result.OK = false
			result.Errors = append(result.Errors, ApplyError{Severity: "error", Message: fmt.Sprintf("写入脚本时 panic: %v", recovered)})
		}
		r.recordApplyResult(result, true)
	}()
	if message := validateName(name); message != "" {
		result.OK = false
		result.Errors = append(result.Errors, ApplyError{Severity: "error", Block: "安全校验", Message: message})
		return result
	}
	if path, err := r.safeSourcePath(name); err != nil {
		result.OK = false
		result.Errors = append(result.Errors, ApplyError{Severity: "error", Message: err.Error()})
		return result
	} else if info, statErr := os.Lstat(path); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		result.OK = false
		result.Errors = append(result.Errors, ApplyError{Severity: "error", Message: "拒绝写入符号链接脚本: " + name})
		return result
	}
	newHash := textHash(text)
	prepared := r.prepareScript(name, text, false)
	result.Warnings = append(result.Warnings, prepared.Warnings...)
	if len(prepared.Errors) > 0 {
		result.OK = false
		result.Errors = append(result.Errors, prepared.Errors...)
		return result
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	old := r.entries[name]
	switch {
	case old == nil && expectedHash != "":
		result.OK = false
		result.Errors = append(result.Errors, ApplyError{Severity: "error", Message: "脚本不存在，不能提供 expected_hash"})
		return result
	case old == nil:
		// New script: continue.
	case expectedHash == "":
		result.OK = false
		result.Errors = append(result.Errors, ApplyError{Severity: "error", Message: "脚本已存在，必须提供 expected_hash（当前 " + old.Hash + "）"})
		return result
	case expectedHash != old.Hash:
		result.OK = false
		result.Errors = append(result.Errors, ApplyError{Severity: "error", Message: "expected_hash 不匹配（当前 " + old.Hash + "）", Hint: "重新读取脚本后重试"})
		return result
	case old.Hash == newHash:
		result.Noop = true
		return result
	}
	r.commitLocked(result, name, text, newHash, prepared, true)
	return result
}

// safeSourcePath resolves a validated name and rejects paths outside scriptsDir.
func (r *Registry) safeSourcePath(name string) (string, error) {
	if r.scriptsDir == "" {
		return "", fmt.Errorf("未配置脚本目录")
	}
	if message := validateName(name); message != "" {
		return "", fmt.Errorf("%s", message)
	}
	absolute, err := filepath.Abs(r.sourcePath(name))
	if err != nil {
		return "", err
	}
	root, err := filepath.Abs(r.scriptsDir)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("脚本路径越界: %s", name)
	}
	return absolute, nil
}

// relativePath renders a source path relative to the scripts root.
func relativePath(root, path string) string {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return path
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	relative, err := filepath.Rel(absoluteRoot, absolutePath)
	if err != nil {
		return path
	}
	return relative
}
