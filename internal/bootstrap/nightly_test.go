// nightly_test.go pins the scheduling arithmetic and the clean shutdown path.
package bootstrap

import (
	"testing"
	"time"
)

func TestParseNightlyAt(t *testing.T) {
	cases := []struct {
		input  string
		hour   int
		minute int
	}{
		{"03:05", 3, 5},
		{" 23:59 ", 23, 59},
		{"00:00", 0, 0},
	}
	for _, testCase := range cases {
		hour, minute, err := parseNightlyAt(testCase.input)
		if err != nil || hour != testCase.hour || minute != testCase.minute {
			t.Fatalf("%q 解析错误：%d:%d,%v", testCase.input, hour, minute, err)
		}
	}
	for _, invalid := range []string{"24:00", "3:5", "abc", ""} {
		if _, _, err := parseNightlyAt(invalid); err == nil {
			t.Fatalf("%q 应被拒绝", invalid)
		}
	}
}

func TestNextNightly(t *testing.T) {
	base := time.Date(2026, 11, 3, 16, 0, 0, 0, time.Local)
	if got := nextNightly(base, 3, 0); !got.Equal(time.Date(2026, 11, 4, 3, 0, 0, 0, time.Local)) {
		t.Fatalf("已过时刻应排到次日：%v", got)
	}
	if got := nextNightly(base, 23, 0); !got.Equal(time.Date(2026, 11, 3, 23, 0, 0, 0, time.Local)) {
		t.Fatalf("未到时刻应排到当日：%v", got)
	}
	equal := time.Date(2026, 11, 3, 16, 0, 0, 0, time.Local)
	if got := nextNightly(equal, 16, 0); !got.Equal(time.Date(2026, 11, 4, 16, 0, 0, 0, time.Local)) {
		t.Fatalf("恰好等于 now 应排到次日：%v", got)
	}
}

func TestRuntimeCloseStopsNightly(t *testing.T) {
	runtime, err := New(Options{
		DataDir:       t.TempDir(),
		ScriptsDir:    t.TempDir(),
		NightlyAt:     "03:00",
		NightlyFactor: 0.5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.nightlyStop == nil {
		t.Fatal("NightlyAt 设置后应有调度器")
	}
	done := make(chan error, 1)
	go func() { done <- runtime.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("关闭失败：%v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close 未停止夜间调度器")
	}
}

func TestInvalidNightlyAtDisables(t *testing.T) {
	runtime, err := New(Options{DataDir: t.TempDir(), ScriptsDir: t.TempDir(), NightlyAt: "25:00"})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.nightlyStop != nil {
		t.Fatal("非法时间不应启动调度器")
	}
	_ = runtime.Close()
}
