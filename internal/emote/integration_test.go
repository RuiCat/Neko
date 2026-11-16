package emote

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// integrationDir returns the real emote directory when it is present.
func integrationDir(t *testing.T) string {
	t.Helper()
	dir := DefaultDir()
	if dir == "" {
		t.Skip("表情目录未配置")
	}
	if _, err := os.Stat(filepath.Join(dir, INDEX_FILE_NAME)); err != nil {
		t.Skipf("表情数据不存在: %s", dir)
	}
	return dir
}

// integrationClient builds a live client (real Ollama) for the regression set.
func integrationClient(t *testing.T) *Client {
	t.Helper()
	if os.Getenv("ASTRBOX_EMOTE_INTEGRATION") == "" {
		t.Skip("设置 ASTRBOX_EMOTE_INTEGRATION=1 运行真实表情库回归")
	}
	return NewClient(Config{Dir: integrationDir(t)})
}

// TestIntegrationEmoteRegression ports the applicable checks of
// verify_emote_negkw.py. Its R18 case was dropped with the filter (2026-11-16).
func TestIntegrationEmoteRegression(t *testing.T) {
	client := integrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cases := []struct {
		name        string
		query       string
		mustHave    []string
		mustNotHave []string
	}{
		{"抱抱-正向", "抱抱", []string{"抱"}, []string{"抱头", "抱胸", "抱枕", "抱婴儿", "抱着一", "抱猫"}},
		{"抱头-负向", "抱头", nil, []string{"相拥", "拥抱", "贴贴", "抱抱"}},
		{"亲亲-正向", "亲亲", []string{"亲", "吻", "啾", "mua"}, nil},
		{"害羞-正向", "害羞", []string{"害羞", "脸红", "捂脸", "红晕"}, nil},
		// R18 过滤已于 2026-11-16 按维护者要求移除，R18 行参与正常排序，
		// 因此原有「R18 不得出现」的断言不再适用。
		{"摸头-正向", "摸头", []string{"摸头", "摸摸"}, []string{"奶子", "胸部", "乳房", "裸"}},
		{"困-正向", "好困想睡觉", []string{"困", "睡", "哈欠", "疲惫", "休息"}, nil},
		{"难过-正向", "难过想哭", []string{"哭", "泪", "难过", "伤心"}, nil},
	}
	failures := []string{}
	for _, testCase := range cases {
		result, err := client.Find(ctx, testCase.query, 5)
		if err != nil {
			failures = append(failures, testCase.name+": "+err.Error())
			continue
		}
		descriptions := make([]string, 0, len(result.Results))
		for _, item := range result.Results {
			descriptions = append(descriptions, item.Desc)
		}
		pool := strings.Join(descriptions, " ")
		positive := len(testCase.mustHave) == 0 || containsAny(pool, testCase.mustHave)
		negative := !containsAny(pool, testCase.mustNotHave)
		if positive && negative {
			t.Logf("PASS %s %q", testCase.name, testCase.query)
			continue
		}
		failures = append(failures, testCase.name+": 正向="+itoaBool(positive)+" 负向="+itoaBool(negative)+" desc="+truncateRunes(pool, 200))
	}
	if len(failures) > 0 {
		t.Fatalf("回归失败 %d/%d:\n%s", len(failures), len(cases), strings.Join(failures, "\n"))
	}
}

// TestIntegrationEmoteBaseline compares the three frozen queries with the
// Python baseline measured on 2026-11-10, before the R18/暧昧 filters were
// removed. Re-measure the baseline against a real data directory.
func TestIntegrationEmoteBaseline(t *testing.T) {
	client := integrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	baseline := []struct {
		query string
		files []string
		score float64
	}{
		{"主人我好想你呀", []string{"8773171715714f8e.jpg", "0b4fcd65c87436b2.jpg", "355970d48c5b5303.jpg"}, 0.7888},
		{"今天好开心", []string{"423393662c2cc3e3.jpg", "17170d0f5b232e19.jpg", "0d0c1d174fcb4b0b.jpg"}, 0.808},
		{"有点困了想睡觉", []string{"6595981a190faba6.jpg", "3351494b4527068e.jpg", "0911215999972759.jpg"}, 0.8029},
	}
	for _, expected := range baseline {
		result, err := client.Find(ctx, expected.query, 3)
		if err != nil {
			t.Fatalf("%s: %v", expected.query, err)
		}
		files := make([]string, 0, len(result.Results))
		for _, item := range result.Results {
			files = append(files, item.File)
		}
		intersection := 0
		for _, wanted := range expected.files {
			for _, got := range files {
				if got == wanted {
					intersection++
				}
			}
		}
		if intersection < 2 {
			t.Fatalf("%s: got %v, want >=2 of %v", expected.query, files, expected.files)
		}
		if diff := absFloat(result.Results[0].Score - expected.score); diff > 0.002 {
			t.Fatalf("%s: top score %v, want ~%v", expected.query, result.Results[0].Score, expected.score)
		}
		t.Logf("PASS %s: %v (top=%.4f)", expected.query, files, result.Results[0].Score)
	}
}

// itoaBool renders a bool for failure details.
func itoaBool(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

// absFloat returns the absolute value.
func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

// TestIntegrationFindLatency measures the in-process p95 of a warm lookup (A7).
func TestIntegrationFindLatency(t *testing.T) {
	client := integrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if _, err := client.Find(ctx, "今天好开心", 3); err != nil {
		t.Fatalf("warmup: %v", err)
	}
	durations := make([]time.Duration, 0, 10)
	for i := 0; i < 10; i++ {
		start := time.Now()
		if _, err := client.Find(ctx, "今天好开心", 3); err != nil {
			t.Fatalf("find %d: %v", i, err)
		}
		durations = append(durations, time.Since(start))
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p95 := durations[len(durations)*95/100]
	t.Logf("p95 = %v (max %v)", p95, durations[len(durations)-1])
	if p95 > 2*time.Second {
		t.Fatalf("p95 = %v, want < 2s", p95)
	}
}
