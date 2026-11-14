// create：按类型实例化一个实体并落盘。
package main

import (
	"encoding/json"
	"flag"
	"fmt"

	"github.com/RuiCat/Neko/internal/bootstrap"
)

func runCreate(runtime *bootstrap.Runtime, args []string) error {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	typeName := fs.String("type", "", "类型名（需存在模板或继承链）")
	name := fs.String("name", "", "实体 ID")
	dataRaw := fs.String("data", "{}", "初始数据 JSON")
	initFlag := fs.Bool("init", false, "创建后立刻跑一次 tick（B 层初始化）")
	_ = fs.Parse(args)
	if *typeName == "" || *name == "" {
		return fmt.Errorf("create 需要 -type 与 -name")
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(*dataRaw), &data); err != nil {
		return fmt.Errorf("data 不是合法 JSON: %w", err)
	}
	if runtime.Store.GetTemplate(*typeName) == nil && len(runtime.Store.Ancestors(*typeName)) == 0 {
		return fmt.Errorf("类型 %q 没有模板或继承链", *typeName)
	}
	ent, err := runtime.Store.Create(*name, *typeName, data, nil)
	if err != nil {
		return err
	}
	if *initFlag {
		result := runtime.Machine.Tick(*name)
		if result.Status != "ok" {
			return fmt.Errorf("初始化 tick 失败: %s %v", result.Status, result.Errors)
		}
		fmt.Printf("init %s: transitions=%d conflicts=%d\n", *name, len(result.Transitions), len(result.Conflicts))
	}
	if err := runtime.Store.Save(); err != nil {
		return err
	}
	fmt.Printf("created %s (type=%s)\n", ent.ID, ent.Type)
	return nil
}
