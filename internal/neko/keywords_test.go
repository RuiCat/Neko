// 回归：顶层关键字的唯一拼写。
// 规则：
//
//	@定义 / @实体 → 模板（IsEntity=true）
//	@实例         → 数据实例（IsEntity=false）
//	@Data / @data 已彻底移除，必须报错并给出替代写法
//	任何未知 `@xxx` 必须报错，不能静默跳过（静默跳过会让拼错整块丢失内容）
package neko

import (
	"strings"
	"testing"
)

func TestKeywordDispatch(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		wantDefs int
		wantEnt  bool
		wantErr  string // 期望的错误片段（空 = 不应报错）
	}{
		{"定义 = 模板", "@定义 角色基础:猫娘 {\n \"a\": 1\n}", 1, true, ""},
		{"实体 = 组合模板", "@实体 角色基础:猫娘 [角色基础] {\n \"a\": 1\n}", 1, true, ""},
		{"实例 = 数据实例", "@实例 角色基础:猫娘:猫猫 {\n \"a\": 1\n}", 1, false, ""},
		{"@描述 占位不报错", "@描述 模块 {\n 说明: x\n}", 0, false, ""},
		{"@Data 已移除", "@Data 角色基础:猫娘 {\n \"a\": 1\n}", 0, false, "@Data 已移除"},
		{"@data 已移除", "@data 角色基础:猫娘:猫猫 {\n \"a\": 1\n}", 0, false, "@data 已移除"},
		{"未知关键字报错", "@dta 角色基础:人族:小海 {\n \"a\": 1\n}", 0, false, "未知的顶层关键字"},
		{"繁体拼错报错", "@實體 某某 {\n \"a\": 1\n}", 0, false, "未知的顶层关键字"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ast := Parse(c.src)
			if len(ast.DataDefs) != c.wantDefs {
				t.Fatalf("DataDefs = %d, want %d", len(ast.DataDefs), c.wantDefs)
			}
			if c.wantDefs == 1 && ast.DataDefs[0].IsEntity != c.wantEnt {
				t.Errorf("IsEntity = %v, want %v", ast.DataDefs[0].IsEntity, c.wantEnt)
			}
			if c.wantErr == "" {
				if len(ast.Errors) != 0 {
					t.Fatalf("不应报错: %v", ast.Errors)
				}
				return
			}
			if len(ast.Errors) == 0 {
				t.Fatalf("期望错误包含 %q，实际没有错误", c.wantErr)
			}
			if !strings.Contains(ast.Errors[0].Error(), c.wantErr) {
				t.Errorf("错误 = %q, want 含 %q", ast.Errors[0].Error(), c.wantErr)
			}
		})
	}
}

// 关键字前缀不能互相误吞：@实体 与 @实例 都是 '@' + 两字，必须各归各位。
func TestKeywordPrefixIsolation(t *testing.T) {
	ast := Parse("@实体 角色基础:猫娘 [角色基础] {\n \"a\": 1\n}\n@实例 角色基础:猫娘:猫猫 {\n \"b\": 2\n}")
	if len(ast.Errors) != 0 {
		t.Fatalf("不应报错: %v", ast.Errors)
	}
	if len(ast.DataDefs) != 2 {
		t.Fatalf("DataDefs = %d, want 2", len(ast.DataDefs))
	}
	if !ast.DataDefs[0].IsEntity || ast.DataDefs[1].IsEntity {
		t.Fatalf("IsEntity 错位: %v / %v", ast.DataDefs[0].IsEntity, ast.DataDefs[1].IsEntity)
	}
	if ast.DataDefs[0].TypeName != "角色基础:猫娘" || ast.DataDefs[1].TypeName != "角色基础:猫娘:猫猫" {
		t.Fatalf("TypeName 错位: %q / %q", ast.DataDefs[0].TypeName, ast.DataDefs[1].TypeName)
	}
}

// @实例 是「已算好的值」，注解只在模板里展开。若允许注解通过，键会被原样写成
// 字面字段名（`体力[int,...]`），真正的 `体力` 反而没赋值，且不报错——
// 等于静默写坏存档。这里锁定「@实例 拒绝注解 / 模板照常接受」。
func TestInstanceRejectsAnnotations(t *testing.T) {
	annotated := `@实例 角色基础:猫娘:猫猫 {
  "基础": {
    "名字": "猫猫",
    "体力[int,min:0,max:100]": 75
  }
}`
	ast := Parse(annotated)
	if len(ast.Errors) == 0 {
		t.Fatal("@实例 里的字段注解应报错")
	}
	if !strings.Contains(ast.Errors[0].Error(), "不能使用字段注解") {
		t.Errorf("错误 = %q, want 含「不能使用字段注解」", ast.Errors[0].Error())
	}
	if !strings.Contains(ast.Errors[0].Error(), "基础.体力") {
		t.Errorf("错误应指出具体路径，实际 = %q", ast.Errors[0].Error())
	}
	if len(ast.DataDefs) != 0 {
		t.Errorf("报错后不应产出 DataDef，实际 %d 条", len(ast.DataDefs))
	}

	// 模板（@定义）里的注解必须照常通过。
	tmpl := Parse(`@定义 角色基础:测试 {
  "基础": {"体力[int,min:0,max:100]": 75}
}`)
	if len(tmpl.Errors) != 0 {
		t.Fatalf("模板注解不应报错: %v", tmpl.Errors)
	}

	// 普通带方括号的字符串值不算注解，不能被误杀。
	plain := Parse(`@实例 场景:客厅 {
  "名称": "客厅[主]",
  "物品": ["桌子"]
}`)
	if len(plain.Errors) != 0 {
		t.Fatalf("值里的方括号不应触发注解检查: %v", plain.Errors)
	}
}
