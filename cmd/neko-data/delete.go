// delete：删除一个实体并落盘。
package main

import (
	"flag"
	"fmt"

	"github.com/RuiCat/Neko/internal/bootstrap"
)

func runDelete(runtime *bootstrap.Runtime, args []string) error {
	fs := flag.NewFlagSet("delete", flag.ExitOnError)
	name := fs.String("name", "", "实体 ID")
	_ = fs.Parse(args)
	if *name == "" {
		return fmt.Errorf("delete 需要 -name")
	}
	if !runtime.Store.Delete(*name) {
		return fmt.Errorf("实体 %q 不存在", *name)
	}
	if err := runtime.Store.Save(); err != nil {
		return err
	}
	fmt.Printf("deleted %s\n", *name)
	return nil
}
