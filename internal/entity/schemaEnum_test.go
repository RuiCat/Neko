package entity

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestEmptyEnumAllowsAnyValue pins the contract that an empty enum (nil,
// typed-nil or zero-length slice) imposes no enum constraint.
func TestEmptyEnumAllowsAnyValue(t *testing.T) {
	cases := []struct {
		name   string
		schema *FieldSchema
	}{
		{"nil enum", &FieldSchema{FieldName: "清洁", Type: "int", Enum: nil}},
		{"typed-nil enum", &FieldSchema{FieldName: "清洁", Type: "int", Enum: []any(nil)}},
		{"empty slice enum", &FieldSchema{FieldName: "清洁", Type: "int", Enum: []any{}}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			for _, value := range []any{65, 0, -1} {
				if got := testCase.schema.Validate(value); got != "" {
					t.Fatalf("Validate(%v) = %q, want empty (no enum constraint)", value, got)
				}
			}
		})
	}
}

// TestNonEmptyEnumStillRejects pins that a real enum keeps rejecting values
// outside the allowed set.
func TestNonEmptyEnumStillRejects(t *testing.T) {
	schema := &FieldSchema{FieldName: "情绪", Type: "str", Enum: []any{"平静", "开心"}}
	if got := schema.Validate("平静"); got != "" {
		t.Fatalf("Validate(平静) = %q, want empty", got)
	}
	if got := schema.Validate("愤怒"); !strings.Contains(got, "不在枚举中") {
		t.Fatalf("Validate(愤怒) = %q, want enum rejection", got)
	}
}

// TestAddFromAnnotationsNormalizesEmptyEnum pins that the annotation reader
// ignores empty enums in every shape JSON can deliver (null / [] / []any{}).
func TestAddFromAnnotationsNormalizesEmptyEnum(t *testing.T) {
	var nullEntry map[string]any
	if err := json.Unmarshal([]byte(`{"type":"int","enum":null}`), &nullEntry); err != nil {
		t.Fatalf("unmarshal enum null: %v", err)
	}
	var emptyEntry map[string]any
	if err := json.Unmarshal([]byte(`{"type":"int","enum":[]}`), &emptyEntry); err != nil {
		t.Fatalf("unmarshal enum empty: %v", err)
	}
	cases := []struct {
		name  string
		entry map[string]any
	}{
		{"json null", nullEntry},
		{"json empty array", emptyEntry},
		{"typed-nil slice", map[string]any{"type": "int", "enum": []any(nil)}},
		{"empty slice", map[string]any{"type": "int", "enum": []any{}}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			schema := NewEntitySchema()
			schema.AddFromAnnotations(map[string]any{"__annotations__": map[string]any{
				"身体.清洁": testCase.entry,
			}})
			field := schema.Get("身体.清洁")
			if field == nil {
				t.Fatal("身体.清洁 schema missing")
			}
			if field.Enum != nil {
				t.Fatalf("Enum = %#v, want nil (empty enum normalized away)", field.Enum)
			}
			if got := schema.Validate("身体.清洁", 65); got != "" {
				t.Fatalf("Validate(身体.清洁, 65) = %q, want empty", got)
			}
		})
	}
}

// TestRealEnumFromAnnotationsStillRejects keeps the annotation path honest for
// a populated enum.
func TestRealEnumFromAnnotationsStillRejects(t *testing.T) {
	schema := NewEntitySchema()
	schema.AddFromAnnotations(map[string]any{"__annotations__": map[string]any{
		"心情.当前": map[string]any{"type": "str", "enum": []any{"平静", "开心"}},
	}})
	if got := schema.Validate("心情.当前", "满足"); !strings.Contains(got, "不在枚举中") {
		t.Fatalf("Validate(满足) = %q, want enum rejection", got)
	}
}

// TestDeepCopyPreservesTypedNilSlice pins that deep copies (and DeepMerge, the
// production materialization point) never turn a typed-nil slice into a
// non-nil empty one.
func TestDeepCopyPreservesTypedNilSlice(t *testing.T) {
	typedNil := []any(nil)
	copied, ok := deepCopy(typedNil).([]any)
	if !ok || copied != nil {
		t.Fatalf("deepCopy(typed-nil) = %#v, want typed-nil []any", deepCopy(typedNil))
	}
	// Non-nil empty slices keep their shape: only typed-nil is repaired here
	// (turning every [] into null would change unrelated arrays like 场景.物品).
	if empty, ok := deepCopy([]any{}).([]any); !ok || empty == nil {
		t.Fatalf("deepCopy(empty slice) = %#v, want non-nil empty []any", deepCopy([]any{}))
	}
	merged := DeepMerge(map[string]any{"enum": typedNil}, map[string]any{})
	mergedEnum, ok := merged["enum"].([]any)
	if !ok || mergedEnum != nil {
		t.Fatalf("DeepMerge enum = %#v, want typed-nil []any", merged["enum"])
	}
}

// TestAnnotationRangeEnforced pins that numeric min/max from annotations are
// applied by Validate (JSON-shaped numbers, not *float64).
func TestAnnotationRangeEnforced(t *testing.T) {
	schema := NewEntitySchema()
	schema.AddFromAnnotations(map[string]any{"__annotations__": map[string]any{
		"探索.好奇心": map[string]any{"type": "int", "min": 0.0, "max": 100.0},
	}})
	if got := schema.Validate("探索.好奇心", 100); got != "" {
		t.Fatalf("Validate(100) = %q, want empty (at upper bound)", got)
	}
	if got := schema.Validate("探索.好奇心", 6108); !strings.Contains(got, "超过最大值 100") {
		t.Fatalf("Validate(6108) = %q, want upper-bound rejection", got)
	}
	if got := schema.Validate("探索.好奇心", -1); !strings.Contains(got, "低于最小值 0") {
		t.Fatalf("Validate(-1) = %q, want lower-bound rejection", got)
	}
}
