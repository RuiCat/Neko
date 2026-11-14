package compiler

import (
	"strings"
	"testing"

	"github.com/RuiCat/Neko/internal/entity"
)

// TestExpandAnnotationsEmitsScalarMinMax pins that annotationMap renders min/max
// as JSON-shaped numbers (float64 / nil) instead of *float64, which the entity
// schema and the engine clamp reader both fail to decode.
func TestExpandAnnotationsEmitsScalarMinMax(t *testing.T) {
	expanded, annotations := ExpandAnnotations(map[string]any{
		"身体": map[string]any{"清洁[int,min:0,max:100]: 64": 64},
	})
	entry, ok := annotations["身体.清洁"].(map[string]any)
	if !ok {
		t.Fatalf("annotations[身体.清洁] = %#v, want map", annotations["身体.清洁"])
	}
	for _, key := range []string{"min", "max"} {
		value, ok := entry[key]
		if !ok {
			t.Fatalf("annotations[身体.清洁][%s] missing", key)
		}
		if _, isNumber := value.(float64); !isNumber {
			t.Fatalf("annotations[身体.清洁][%s] = %#v (%T), want float64", key, value, value)
		}
	}

	_, unset := ExpandAnnotations(map[string]any{
		"心情": map[string]any{"当前[str]: 满足": "满足"},
	})
	unsetEntry, ok := unset["心情.当前"].(map[string]any)
	if !ok {
		t.Fatalf("annotations[心情.当前] = %#v, want map", unset["心情.当前"])
	}
	if unsetEntry["min"] != nil || unsetEntry["max"] != nil {
		t.Fatalf("unset range = (min=%#v, max=%#v), want nils", unsetEntry["min"], unsetEntry["max"])
	}

	// The compiled annotations must enforce the range once read back.
	schema := entity.NewEntitySchema()
	schema.AddFromAnnotations(expanded)
	if got := schema.Validate("身体.清洁", 100); got != "" {
		t.Fatalf("Validate(100) = %q, want empty", got)
	}
	if got := schema.Validate("身体.清洁", 101); !strings.Contains(got, "超过最大值 100") {
		t.Fatalf("Validate(101) = %q, want upper-bound rejection", got)
	}
	if got := schema.Validate("身体.清洁", -1); !strings.Contains(got, "低于最小值 0") {
		t.Fatalf("Validate(-1) = %q, want lower-bound rejection", got)
	}
}
