// apply_test.go covers the content loader: instance name parsing, materializing
// @实例 blocks, idempotent re-runs and the overwrite option.
package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/RuiCat/Neko/internal/bootstrap"
)

func TestSplitInstanceName(t *testing.T) {
	cases := []struct {
		raw      string
		wantType string
		wantID   string
	}{
		{"食物:苹果", "食物", "苹果"},
		{"角色基础:猫娘:猫猫", "角色基础:猫娘", "猫猫"},
		{"场景", "", ""},
		{":苹果", "", ""},
		{"食物:", "", ""},
	}
	for _, testCase := range cases {
		typeName, id := splitInstanceName(testCase.raw)
		if typeName != testCase.wantType || id != testCase.wantID {
			t.Fatalf("splitInstanceName(%q) = (%q,%q)，期望 (%q,%q)",
				testCase.raw, typeName, id, testCase.wantType, testCase.wantID)
		}
	}
}

// newApplyRuntime assembles a runtime over the repo's kernel scripts and a temp
// data dir. Content always comes from a temp blueprint dir: the repo holds no
// entity data.
func newApplyRuntime(t *testing.T) *bootstrap.Runtime {
	t.Helper()
	runtime, err := bootstrap.New(bootstrap.Options{
		DataDir:    t.TempDir(),
		ScriptsDir: filepath.Join("..", "..", "scripts"),
	})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	return runtime
}

// writeAppleContent writes one content blueprint file into a temp dir.
func writeAppleContent(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	payload := "@实例 食物:苹果 {\n  \"名称\": \"苹果\",\n  \"类别\": \"食物\",\n  \"数量\": 3,\n  \"价格\": 5,\n  \"食物\": { \"热量\": 95, \"可食\": true }\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "food.neko"), []byte(payload), 0o644); err != nil {
		t.Fatalf("write content: %v", err)
	}
	return dir
}

func TestApplyCreatesInstancesAndIsIdempotent(t *testing.T) {
	runtime := newApplyRuntime(t)
	contentDir := writeAppleContent(t)
	if err := runApply(runtime, []string{"-content", contentDir}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	apple := runtime.Store.Get("苹果")
	if apple == nil {
		t.Fatal("苹果 没有创建")
	}
	if name, _ := apple.Get("名称"); name != "苹果" {
		t.Fatalf("名称 = %#v", name)
	}
	if price, _ := apple.Get("价格"); toNum(price) != 5 {
		t.Fatalf("价格 = %#v，期望 5", price)
	}
	if count, _ := apple.Get("数量"); toNum(count) != 3 {
		t.Fatalf("数量 = %#v，期望 3", count)
	}

	// 运行时改状态后再次 apply：默认跳过已有实体，不能把数量洗回 3。
	if err := apple.Set("数量", 1); err != nil {
		t.Fatalf("set 数量: %v", err)
	}
	if err := runApply(runtime, []string{"-content", contentDir}); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if count, _ := apple.Get("数量"); toNum(count) != 1 {
		t.Fatalf("第二次 apply 覆盖了运行时状态：数量 = %#v", count)
	}
}

func TestApplyOverwriteOnlyTouchesContentFields(t *testing.T) {
	runtime := newApplyRuntime(t)
	contentDir := writeAppleContent(t)
	if err := runApply(runtime, []string{"-content", contentDir}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	apple := runtime.Store.Get("苹果")
	if err := apple.Set("数量", 1); err != nil {
		t.Fatalf("set 数量: %v", err)
	}
	if err := apple.Set("持有人", "猫猫"); err != nil {
		t.Fatalf("set 持有人: %v", err)
	}
	if err := runApply(runtime, []string{"-content", contentDir, "-overwrite"}); err != nil {
		t.Fatalf("overwrite apply: %v", err)
	}
	if count, _ := apple.Get("数量"); toNum(count) != 3 {
		t.Fatalf("overwrite 应把内容字段 数量 写回 3，得到 %#v", count)
	}
	if holder, _ := apple.Get("持有人"); holder != "猫猫" {
		t.Fatalf("overwrite 不该动内容块之外的字段，持有人 = %#v", holder)
	}
}

func TestApplyRejectsDuplicateIDs(t *testing.T) {
	runtime := newApplyRuntime(t)
	contentDir := t.TempDir()
	payload := "@实例 食物:苹果 {\n  \"名称\": \"苹果\"\n}\n@实例 食物:苹果 {\n  \"名称\": \"苹果2\"\n}\n"
	if err := os.WriteFile(filepath.Join(contentDir, "dup.neko"), []byte(payload), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := runApply(runtime, []string{"-content", contentDir, "-dry-run"}); err == nil {
		t.Fatal("重复实体 ID 必须报错")
	}
}

func TestApplyRejectsUnknownType(t *testing.T) {
	runtime := newApplyRuntime(t)
	contentDir := t.TempDir()
	payload := "@实例 不存在类型:某物 {\n  \"名称\": \"某物\"\n}\n"
	if err := os.WriteFile(filepath.Join(contentDir, "bad.neko"), []byte(payload), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := runApply(runtime, []string{"-content", contentDir, "-dry-run"}); err == nil {
		t.Fatal("未知类型的实体必须报错")
	}
}

// toNum 把测试里读到的数值统一成 float64（内容块来自 JSON，数字是 float64）。
func toNum(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return -1
	}
}
