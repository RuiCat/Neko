package registry

import (
	"fmt"
	"os"
	"sort"

	"github.com/ruicat/astrbox/internal/diagnostics"
)

// ScanConsistency rebuilds script/source consistency diagnostics and returns
// the number of findings. It clears the previous consistency generation first.
func (r *Registry) ScanConsistency() int {
	if r.diagnostics == nil {
		return 0
	}
	type registered struct{ name, hash string }
	r.mu.Lock()
	entries := make([]registered, 0, len(r.entries))
	for name, entry := range r.entries {
		entries = append(entries, registered{name: name, hash: entry.Hash})
	}
	r.mu.Unlock()
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })

	r.sourcesMu.RLock()
	orphans := make([]string, 0, len(r.sources))
	for name := range r.sources {
		orphans = append(orphans, name)
	}
	r.sourcesMu.RUnlock()
	sort.Strings(orphans)

	known := make(map[string]bool, len(entries))
	r.diagnostics.ClearBySource("consistency")
	count := 0
	for _, item := range entries {
		known[item.name] = true
		path := r.sourcePath(item.name)
		info, err := os.Lstat(path)
		switch {
		case err != nil:
			r.addConsistency("script.source.missing", diagnostics.SEVERITY_ERROR, item.name,
				"scripts.json 有条目但源文件缺失", "恢复源文件或重新 apply")
			count++
			continue
		case info.Mode()&os.ModeSymlink != 0:
			r.addConsistency("script.source.symlink", diagnostics.SEVERITY_ERROR, item.name,
				"脚本源是符号链接，已拒绝读取", "改回普通文件")
			count++
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			r.addConsistency("script.source.unreadable", diagnostics.SEVERITY_ERROR, item.name,
				"脚本源不可读: "+err.Error(), "检查文件权限")
			count++
			continue
		}
		if actual := textHash(string(raw)); actual != item.hash {
			r.addConsistency("script.hash.mismatch", diagnostics.SEVERITY_WARNING, item.name,
				fmt.Sprintf("注册 hash %s 与源文件 %s 不一致", item.hash, actual), "重新 apply 或读取 actual_hash")
			count++
		}
	}
	for _, name := range orphans {
		if known[name] {
			continue
		}
		r.addConsistency("script.source.orphan", diagnostics.SEVERITY_WARNING, name,
			"源文件存在但未在 scripts.json 注册", "重新 apply 该脚本或移出 scripts/")
		count++
	}
	return count
}

// CheckScript re-checks one script after a write and records any finding.
func (r *Registry) CheckScript(name string) {
	if r.diagnostics == nil {
		return
	}
	r.mu.Lock()
	entry := r.entries[name]
	r.mu.Unlock()
	if entry == nil {
		return
	}
	path := r.sourcePath(name)
	info, err := os.Lstat(path)
	if err != nil {
		r.addConsistency("script.source.missing", diagnostics.SEVERITY_ERROR, name,
			"scripts.json 有条目但源文件缺失", "恢复源文件或重新 apply")
		return
	}
	if info.Mode()&os.ModeSymlink != 0 {
		r.addConsistency("script.source.symlink", diagnostics.SEVERITY_ERROR, name,
			"脚本源是符号链接，已拒绝读取", "改回普通文件")
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	if actual := textHash(string(raw)); actual != entry.Hash {
		r.addConsistency("script.hash.mismatch", diagnostics.SEVERITY_WARNING, name,
			fmt.Sprintf("注册 hash %s 与源文件 %s 不一致", entry.Hash, actual), "重新 apply 或读取 actual_hash")
	}
}

// addConsistency records one consistency finding.
func (r *Registry) addConsistency(code string, severity diagnostics.Severity, script, message, hint string) {
	r.diagnostics.Add(diagnostics.Record{
		Code: code, Severity: severity, Source: "consistency",
		Script: script, Message: message, Hint: hint,
	})
}
