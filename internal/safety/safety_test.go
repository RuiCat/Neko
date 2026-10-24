package safety

import (
	"strings"
	"testing"

	"github.com/ruicat/astrbox/internal/config"
	"github.com/ruicat/astrbox/internal/neko"
)

func enable(t *testing.T) {
	t.Helper()
	if err := config.Set(config.Config{
		StateGroup: "状态", StateDefinitionField: "状态定义",
		DefaultStateSubgroup: "身体", SafetyEnabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(config.Reset)
}

func TestDisabledByDefault(t *testing.T) {
	config.Reset()
	if hits := CheckText("高潮 射精", ""); len(hits) != 0 {
		t.Fatalf("disabled scan returned hits: %v", hits)
	}
}

func TestCheckTextWhenEnabled(t *testing.T) {
	enable(t)
	hits := CheckText("这是一段高潮内容", "")
	if len(hits) != 1 || hits[0] != "高潮" {
		t.Fatalf("hits = %v", hits)
	}
	if hits := CheckText("正常内容", ""); len(hits) != 0 {
		t.Fatalf("false positive: %v", hits)
	}
	nameHits := CheckText("正常", "cat_eversion_rules")
	if len(nameHits) == 0 || !strings.Contains(nameHits[0], "文件名命中") {
		t.Fatalf("name hits = %v", nameHits)
	}
}

func TestCheckAST(t *testing.T) {
	enable(t)
	ast := neko.Parse(`@规则[$] {
  动作: 状态.触发 = "高潮"
}`)
	violations := CheckAST(ast)
	if len(violations) == 0 {
		t.Fatal("expected AST violation")
	}
	if violations[0].Type != "rule" {
		t.Fatalf("type = %s", violations[0].Type)
	}
	clean := neko.Parse(`@规则[$] {
  动作: 状态.触发 = "平静"
}`)
	if violations := CheckAST(clean); len(violations) != 0 {
		t.Fatalf("clean script flagged: %v", violations)
	}
}
