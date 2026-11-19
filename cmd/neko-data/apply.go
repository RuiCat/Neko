// apply：把内容蓝图目录（默认 <repo>/private）下的 @实例 块物化成实体
// （幂等，默认不覆盖已有实体）。
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/RuiCat/Neko/internal/bootstrap"
	"github.com/RuiCat/Neko/internal/neko"
)

// contentInstance is one @实例 block resolved to a type and an entity id.
type contentInstance struct {
	Source string
	Type   string
	Types  []string
	ID     string
	Data   map[string]any
}

// runApply reads every .neko under the content directory and materializes its
// @实例 (non-template) blocks as entities. Existing entities are left alone
// unless -overwrite is given, so re-running never resets runtime state.
func runApply(runtime *bootstrap.Runtime, args []string) error {
	fs := flag.NewFlagSet("apply", flag.ExitOnError)
	contentDir := fs.String("content", "", "内容目录（留空 = 仓库根的 private/）")
	dryRun := fs.Bool("dry-run", false, "只校验，不写盘")
	overwrite := fs.Bool("overwrite", false, "已有实体也写入内容块里的字段（其余状态保留）")
	_ = fs.Parse(args)

	// 内容蓝图不进 scripts/：默认只读仓库根的 private/（存在才读），或用 -content 指定。
	roots := []string{filepath.Join(filepath.Dir(runtime.ScriptsDir), "private")}
	if *contentDir != "" {
		roots = []string{*contentDir}
	}
	instances := []contentInstance{}
	for _, root := range roots {
		found, err := collectContent(root)
		if err != nil {
			return err
		}
		instances = append(instances, found...)
	}
	if len(instances) == 0 {
		fmt.Printf("内容为空：%v\n", roots)
		return nil
	}

	seen := map[string]string{}
	for _, instance := range instances {
		if previous, exists := seen[instance.ID]; exists {
			return fmt.Errorf("%s: 实体 ID %q 与 %s 重复", instance.Source, instance.ID, previous)
		}
		seen[instance.ID] = instance.Source
	}
	created, updated, skipped := 0, 0, 0
	for _, instance := range instances {
		// The classification path (instance.Type) is a namespace and need not be a
		// template; every listed type must resolve, or the fields cannot be built.
		if len(instance.Types) == 0 {
			if runtime.Store.GetTemplate(instance.Type) == nil && len(runtime.Store.Ancestors(instance.Type)) == 0 {
				return fmt.Errorf("%s: 类型 %q 没有模板或继承链（也没有 [] 类型列表）", instance.Source, instance.Type)
			}
		}
		for _, typeName := range instance.Types {
			if runtime.Store.GetTemplate(typeName) == nil && len(runtime.Store.Ancestors(typeName)) == 0 {
				return fmt.Errorf("%s: [] 里的类型 %q 没有模板或继承链", instance.Source, typeName)
			}
		}
		existing := runtime.Store.Get(instance.ID)
		switch {
		case existing == nil:
			if *dryRun {
				fmt.Printf("would create %s (%s; types=%v) <- %s\n", instance.ID, instance.Type, instance.Types, instance.Source)
				created++
				continue
			}
			composed := runtime.Store.ComposeDefaults(append([]string{instance.Type}, instance.Types...), instance.Data)
			createdEntity, err := runtime.Store.Create(instance.ID, instance.Type, composed, nil)
			if err != nil {
				return fmt.Errorf("%s: 创建 %s 失败: %w", instance.Source, instance.ID, err)
			}
			createdEntity.Types = append([]string(nil), instance.Types...)
			fmt.Printf("created %s (%s; types=%v)\n", instance.ID, instance.Type, instance.Types)
			created++
		case !*overwrite:
			fmt.Printf("skip %s (已存在)\n", instance.ID)
			skipped++
		case *dryRun:
			fmt.Printf("would update %s (%s)\n", instance.ID, instance.Type)
			updated++
		default:
			if err := applyFields(existing, instance.Data); err != nil {
				return fmt.Errorf("%s: 更新 %s 失败: %w", instance.Source, instance.ID, err)
			}
			fmt.Printf("updated %s (%s)\n", instance.ID, instance.Type)
			updated++
		}
	}
	if !*dryRun && (created > 0 || updated > 0) {
		if err := runtime.Store.Save(); err != nil {
			return fmt.Errorf("保存失败: %w", err)
		}
	}
	fmt.Printf("apply 完成：created=%d updated=%d skipped=%d dryRun=%v\n", created, updated, skipped, *dryRun)
	return nil
}

// collectContent parses every content file and returns its instance blocks in
// file order. A duplicate entity id across files is an error: content stays
// reviewable only when ids are unique.
func collectContent(dir string) ([]contentInstance, error) {
	files := []string{}
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".neko") {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	sort.Strings(files)
	instances := []contentInstance{}
	seen := map[string]string{}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		ast := neko.Parse(string(raw))
		if len(ast.Errors) > 0 {
			return nil, fmt.Errorf("%s: 解析失败: %v", file, ast.Errors)
		}
		for _, def := range ast.DataDefs {
			if def.IsEntity {
				continue
			}
			typeName, id := splitInstanceName(def.TypeName)
			if typeName == "" || id == "" {
				return nil, fmt.Errorf("%s: @实例 %q 需要写成 `类型:实体ID`", file, def.TypeName)
			}
			if previous, exists := seen[id]; exists {
				return nil, fmt.Errorf("%s: 实体 ID %q 与 %s 重复", file, id, previous)
			}
			seen[id] = file
			markExplicitAgeKnown(def.Data)
			instances = append(instances, contentInstance{
				Source: file, Type: typeName, Types: append([]string(nil), def.Parents...),
				ID: id, Data: def.Data,
			})
		}
	}
	return instances, nil
}

// splitInstanceName splits `类型:实体ID` at the last colon so template names
// that already contain a colon (for example 角色基础:猫娘) keep working.
func splitInstanceName(raw string) (string, string) {
	trimmed := strings.TrimSpace(raw)
	index := strings.LastIndex(trimmed, ":")
	if index <= 0 || index == len(trimmed)-1 {
		return "", ""
	}
	return strings.TrimSpace(trimmed[:index]), strings.TrimSpace(trimmed[index+1:])
}

// markExplicitAgeKnown flags a content block that sets 基础.年龄已知 itself, so the
// bInitAgeKnown derivation (which defaults 年龄已知 from 年龄 > 0) leaves it alone.
// Written as a sibling leaf under 基础 so the value survives ticks.
func markExplicitAgeKnown(data map[string]any) {
	base, ok := data["基础"].(map[string]any)
	if !ok {
		return
	}
	if _, explicit := base["年龄已知"]; explicit {
		base["年龄已知显式"] = true
	}
}

// applyFields writes every leaf of one content block onto an existing entity.
// Only the fields named by the content file are touched.
func applyFields(entity interface {
	Set(path string, value any) error
}, data map[string]any) error {
	return writeLeaves("", data, entity.Set)
}

// writeLeaves walks a content block and calls set for every leaf path.
func writeLeaves(prefix string, node map[string]any, set func(path string, value any) error) error {
	keys := make([]string, 0, len(node))
	for key := range node {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		if nested, ok := node[key].(map[string]any); ok {
			if err := writeLeaves(path, nested, set); err != nil {
				return err
			}
			continue
		}
		if err := set(path, node[key]); err != nil {
			return err
		}
	}
	return nil
}
