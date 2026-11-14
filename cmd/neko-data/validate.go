// validate：检查模板文件、父链、实例字段与数据文件版本是否一致；失败返回非零。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/RuiCat/Neko/internal/bootstrap"
)

func runValidate(runtime *bootstrap.Runtime) error {
	problems := []string{}

	// 1) 模板文件齐全 + 父链 sidecar 齐全
	names := make([]string, 0, len(runtime.Store.Templates()))
	for name := range runtime.Store.Templates() {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		file := filepath.Join(runtime.DataDir, "templates", strings.ReplaceAll(name, ":", "_")+".json")
		if _, err := os.Stat(file); err != nil {
			problems = append(problems, fmt.Sprintf("模板文件缺失: %s", file))
		}
		if parents := runtime.Store.GetTemplateParents(name); len(parents) > 0 {
			sidecar := strings.TrimSuffix(file, ".json") + ".parents.json"
			if _, err := os.Stat(sidecar); err != nil {
				problems = append(problems, fmt.Sprintf("父链 sidecar 缺失: %s", sidecar))
			}
		}
	}

	// 2) 每个实体都能解析类型、且实例无缺失字段
	for _, ent := range runtime.Store.ListAll() {
		if len(runtime.Store.ResolveOrder(ent.Type)) == 0 {
			problems = append(problems, fmt.Sprintf("实体 %s 的类型 %s 无模板链", ent.ID, ent.Type))
			continue
		}
		present := leafPaths(ent.Data)
		required := requiredPaths(runtime, ent.Type)
		missing := 0
		for path := range required {
			if !present[path] {
				missing++
			}
		}
		if missing > 0 {
			problems = append(problems, fmt.Sprintf("实体 %s 缺失 %d 个实例字段", ent.ID, missing))
		}
	}

	// 3) 数据文件版本
	for _, file := range []string{
		filepath.Join(runtime.DataDir, "entities.json"),
		filepath.Join(runtime.DataDir, "user", "entities.json"),
	} {
		raw, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		var payload struct {
			Version int `json:"version"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			problems = append(problems, fmt.Sprintf("%s 不是合法 JSON", file))
			continue
		}
		if payload.Version == 0 {
			problems = append(problems, fmt.Sprintf("%s 缺少 version 字段", file))
		}
	}

	if len(problems) == 0 {
		fmt.Printf("validate ok: 模板 %d，实体 %d\n", len(names), len(runtime.Store.ListAll()))
		return nil
	}
	for _, problem := range problems {
		fmt.Printf("FAIL %s\n", problem)
	}
	return fmt.Errorf("校验失败 %d 项", len(problems))
}
