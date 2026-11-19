// instanceHeader_test.go covers the @实例 header: classification path + type list.
package neko

import (
	"strings"
	"testing"
)

func TestParseInstanceHeaderKeepsClassificationPath(t *testing.T) {
	name, types, err := ParseInstanceHeader("角色:猫猫 [角色定义:猫娘]")
	if err != nil {
		t.Fatal(err)
	}
	if name != "角色:猫猫" {
		t.Fatalf("name = %q, want 角色:猫猫", name)
	}
	if len(types) != 1 || types[0] != "角色定义:猫娘" {
		t.Fatalf("types = %v, want [角色定义:猫娘]", types)
	}
}

func TestParseInstanceHeaderRejectsBadBrackets(t *testing.T) {
	for _, header := range []string{"角色:猫猫 [角色定义:猫娘", "角色:猫猫 [] 多余"} {
		if _, _, err := ParseInstanceHeader(header); err == nil {
			t.Fatalf("%q 应当报错", header)
		}
	}
}

func TestParseInstanceBracketsReachAST(t *testing.T) {
	ast := Parse("@实例 角色:猫猫 [角色定义:猫娘] {\n  \"基础\": {\"名字\": \"猫猫\"}\n}\n")
	if len(ast.Errors) != 0 {
		t.Fatalf("errors = %v", ast.Errors)
	}
	if len(ast.DataDefs) != 1 {
		t.Fatalf("data defs = %d", len(ast.DataDefs))
	}
	def := ast.DataDefs[0]
	if def.TypeName != "角色:猫猫" || len(def.Parents) != 1 || def.Parents[0] != "角色定义:猫娘" {
		t.Fatalf("def = %+v", def)
	}
}

func TestDefinitionRejectsBracketList(t *testing.T) {
	ast := Parse("@定义 猫娘 [角色基础] {\n  \"种族标识\": \"猫娘\"\n}\n")
	if len(ast.Errors) == 0 || !strings.Contains(ast.Errors[0].Error(), "不接受方括号列表") {
		t.Fatalf("expected bracket error, got %v", ast.Errors)
	}
}
