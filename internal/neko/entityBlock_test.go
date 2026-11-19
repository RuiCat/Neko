// entityBlock_test.go covers the @实体 composition header and block scanner.
package neko

import "testing"

func TestParseEntityBlockWithParents(t *testing.T) {
	source := `@实体 角色基础:猫猫 [角色基础:猫科, 角色基础:成年] {
  "毛色": "橘",
  "体型": "幼"
}`
	ast := Parse(source)
	if len(ast.Errors) != 0 {
		t.Fatalf("parse errors: %v", ast.Errors)
	}
	if len(ast.DataDefs) != 1 {
		t.Fatalf("data defs = %d, want 1", len(ast.DataDefs))
	}
	def := ast.DataDefs[0]
	if def.TypeName != "角色基础:猫猫" {
		t.Fatalf("type = %q", def.TypeName)
	}
	if len(def.Parents) != 2 || def.Parents[0] != "角色基础:猫科" || def.Parents[1] != "角色基础:成年" {
		t.Fatalf("parents = %v", def.Parents)
	}
	if !def.IsEntity {
		t.Fatal("expected entity template")
	}
	if def.Data["毛色"] != "橘" || def.Data["体型"] != "幼" {
		t.Fatalf("data = %v", def.Data)
	}
}

func TestParseEntityBlockWithoutParentsAndFullWidthCommas(t *testing.T) {
	source := `@实体 基底 { "值": 1 }
@实体 组合 [基底，另一基底] { "额外": true }`
	ast := Parse(source)
	if len(ast.Errors) != 0 {
		t.Fatalf("parse errors: %v", ast.Errors)
	}
	if len(ast.DataDefs) != 2 {
		t.Fatalf("data defs = %d, want 2", len(ast.DataDefs))
	}
	if len(ast.DataDefs[0].Parents) != 0 {
		t.Fatalf("base parents = %v", ast.DataDefs[0].Parents)
	}
	got := ast.DataDefs[1].Parents
	if len(got) != 2 || got[0] != "基底" || got[1] != "另一基底" {
		t.Fatalf("parents = %v", got)
	}
}

func TestParseEntityBlockAcrossLines(t *testing.T) {
	source := `@实体 多层 [
  基底,
  另一基底
] {
  "值": 2
}`
	ast := Parse(source)
	if len(ast.Errors) != 0 {
		t.Fatalf("parse errors: %v", ast.Errors)
	}
	if len(ast.DataDefs) != 1 {
		t.Fatalf("data defs = %d, want 1", len(ast.DataDefs))
	}
	if parents := ast.DataDefs[0].Parents; len(parents) != 2 {
		t.Fatalf("parents = %v", parents)
	}
}

func TestParseEntityHeaderRejectsDuplicateParents(t *testing.T) {
	_, _, err := ParseEntityHeader("类型 [A, A]")
	if err == nil {
		t.Fatal("expected duplicate parent error")
	}
}
