// audit：只读核对模板、实体与缺失字段矩阵；不写盘。
package main

import (
	"fmt"
	"sort"

	"github.com/RuiCat/Neko/internal/bootstrap"
)

// flatten 把嵌套 map 展开成叶子路径集合；列表/标量算叶子。
func flatten(prefix string, value any, out map[string]bool) {
	if m, ok := value.(map[string]any); ok {
		for key, child := range m {
			if key == "__annotations__" || key == "__parents__" {
				continue
			}
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			flatten(path, child, out)
		}
		return
	}
	if prefix != "" {
		out[prefix] = true
	}
}

func leafPaths(value any) map[string]bool {
	out := map[string]bool{}
	flatten("", value, out)
	return out
}

// requiredPaths 返回一个类型（含祖先模板）声明的全部叶子路径。
func requiredPaths(runtime *bootstrap.Runtime, typeName string) map[string]bool {
	required := map[string]bool{}
	for _, name := range runtime.Store.ResolveOrder(typeName) {
		if template := runtime.Store.GetTemplate(name); template != nil {
			for path := range leafPaths(template) {
				required[path] = true
			}
		}
	}
	return required
}

func runAudit(runtime *bootstrap.Runtime) error {
	templates := runtime.Store.Templates()
	names := make([]string, 0, len(templates))
	for name := range templates {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Printf("模板 %d 个:\n", len(names))
	for _, name := range names {
		fmt.Printf("  %-14s 字段 %d\n", name, len(leafPaths(templates[name])))
	}

	registered := runtime.Registry.List()
	hasAttach := false
	for _, name := range registered {
		if name == "metabolismAttach" {
			hasAttach = true
		}
	}
	fmt.Printf("\n注册脚本 %d 个，metabolismAttach=%v\n", len(registered), hasAttach)
	fmt.Printf("NPC 父模板: %v\n", runtime.Store.GetTemplateParents("角色基础"))
	fmt.Printf("角色基础:猫娘 解析顺序: %v\n", runtime.Store.ResolveOrder("角色基础:猫娘"))

	entities := runtime.Store.ListAll()
	fmt.Printf("\n实体 %d 个:\n", len(entities))
	type summary struct {
		id      string
		typ     string
		missing []string
	}
	rows := make([]summary, 0, len(entities))
	for _, ent := range entities {
		present := leafPaths(ent.Data)
		required := requiredPaths(runtime, ent.Type)
		missing := make([]string, 0)
		for path := range required {
			if !present[path] {
				missing = append(missing, path)
			}
		}
		sort.Strings(missing)
		rows = append(rows, summary{ent.ID, ent.Type, missing})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].id < rows[j].id })
	totalMissing := 0
	for _, row := range rows {
		totalMissing += len(row.missing)
		fmt.Printf("  %-12s type=%-16s 缺失 %d\n", row.id, row.typ, len(row.missing))
		for index, path := range row.missing {
			if index >= 8 {
				fmt.Printf("      … 其余 %d 项\n", len(row.missing)-index)
				break
			}
			fmt.Printf("      - %s\n", path)
		}
	}
	if len(rows) == 0 {
		fmt.Println("  （无实体；data/ 为空时属预期）")
	}
	fmt.Printf("\n汇总: 模板 %d，实体 %d，实例缺失字段 %d\n", len(names), len(rows), totalMissing)
	return nil
}
