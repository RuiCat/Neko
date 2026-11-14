// neko-data：数据层只读核对与实例化 CLI（M0）。
// 用法：
//
//	neko-data -data-dir data -scripts-dir scripts audit
//	neko-data -data-dir data -scripts-dir scripts create -type 场景 -name S1 -data '{"名称":"客厅"}'
//	neko-data -data-dir data -scripts-dir scripts apply [-dry-run] [-overwrite]
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/RuiCat/Neko/internal/bootstrap"
)

func usage() {
	fmt.Fprintln(os.Stderr, "用法: neko-data [-data-dir data] [-scripts-dir scripts] [-config path] <audit|create|delete|validate|apply> [子参数]")
	os.Exit(2)
}

func main() {
	fs := flag.NewFlagSet("neko-data", flag.ExitOnError)
	dataDir := fs.String("data-dir", "data", "数据目录")
	scriptsDir := fs.String("scripts-dir", "scripts", "脚本目录")
	configPath := fs.String("config", "", "配置 JSON（可空）")
	_ = fs.Parse(os.Args[1:])
	args := fs.Args()
	if len(args) == 0 {
		usage()
	}

	runtime, err := bootstrap.New(bootstrap.Options{
		DataDir:    *dataDir,
		ScriptsDir: *scriptsDir,
		ConfigPath: *configPath,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "装配运行时失败: %v\n", err)
		os.Exit(1)
	}

	switch args[0] {
	case "audit":
		err = runAudit(runtime)
	case "create":
		err = runCreate(runtime, args[1:])
	case "validate":
		err = runValidate(runtime)
	case "delete":
		err = runDelete(runtime, args[1:])
	case "apply":
		err = runApply(runtime, args[1:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "失败: %v\n", err)
		os.Exit(1)
	}
}
