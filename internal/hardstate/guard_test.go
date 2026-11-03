// guard_test.go pins the write-source rules for the hard-state subset.
package hardstate

import "testing"

func TestCheckWriteAllowsOnlyStateMachine(t *testing.T) {
	schema := DefaultSchema()
	check := CheckWrite(schema, "猫猫", "状态.情绪", SOURCE_STATE_MACHINE)
	if !check.HardState || !check.Allowed {
		t.Fatalf("状态机写硬状态应放行：%+v", check)
	}
	for _, source := range []string{SOURCE_MCP, SOURCE_SCRIPT, SOURCE_CONTROL, SOURCE_UNKNOWN, ""} {
		check := CheckWrite(schema, "猫猫", "状态.情绪", source)
		if !check.HardState || check.Allowed {
			t.Fatalf("来源 %q 写硬状态应判违规：%+v", source, check)
		}
		if check.Reason == "" {
			t.Fatalf("来源 %q 缺少违规原因", source)
		}
	}
}

func TestCheckWriteIgnoresNonHardStatePaths(t *testing.T) {
	schema := DefaultSchema()
	for _, path := range []string{"标签", "状态定义.情绪", "身体.体力"} {
		check := CheckWrite(schema, "猫猫", path, SOURCE_MCP)
		if check.HardState || check.Allowed {
			t.Fatalf("路径 %q 不应算硬状态：%+v", path, check)
		}
	}
	check := CheckWrite(schema, "猫猫", "状态", SOURCE_MCP)
	if !check.HardState || check.Allowed {
		t.Fatalf("状态组本身应算硬状态：%+v", check)
	}
}

func TestCheckWriteRespectsEntityFilterAndDisabledSchema(t *testing.T) {
	schema := DefaultSchema().WithEntities("梦梦")
	if check := CheckWrite(schema, "猫猫", "状态.情绪", SOURCE_MCP); check.HardState {
		t.Fatalf("被过滤实体不应受守卫：%+v", check)
	}
	if check := CheckWrite(Schema{}, "猫猫", "状态.情绪", SOURCE_MCP); check.HardState {
		t.Fatalf("未配置 schema 时守卫应停用：%+v", check)
	}
}

func TestSchemaCovers(t *testing.T) {
	schema := DefaultSchema()
	if !schema.Covers("状态.情绪") || !schema.Covers("状态") {
		t.Fatal("状态组路径应被 Covers 命中")
	}
	if schema.Covers("状态定义.情绪") || schema.Covers("身体.体力") {
		t.Fatal("组外路径不应被 Covers 命中")
	}
}
