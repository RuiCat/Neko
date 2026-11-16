// policy_test.go covers wildcard matching, rule ordering, threshold lookup,
// normalization and the file loader of the observation policy.
package observation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMatchPattern(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"状态.*", "状态.身体", true},
		{"状态.*", "状态", false},
		{"状态.*", "状态.身体.疼痛", true},
		{"*.心率", "循环.心率", true},
		{"*", "任意.路径", true},
		{"代谢.*", "循环.心率", false},
		{"状态.身体", "状态.身体", true},
		{"状态.身", "状态.身体", false},
	}
	for _, testCase := range cases {
		if got := matchPattern(testCase.pattern, testCase.path); got != testCase.want {
			t.Fatalf("matchPattern(%q,%q)=%v，期望 %v", testCase.pattern, testCase.path, got, testCase.want)
		}
	}
}

func TestTierOfUsesFirstMatchingRuleThenDefault(t *testing.T) {
	policy := &Policy{
		DefaultTier: TierNormal,
		Rules: []Rule{
			{Pattern: "状态.*", Tier: TierCritical},
			{Pattern: "状态.身体", Tier: TierNoise},
			{Pattern: "代谢.*", Tier: TierNoise},
		},
	}
	if got := policy.TierOf("状态.身体"); got != TierCritical {
		t.Fatalf("状态.身体 应命中第一条规则（critical），得到 %s", got)
	}
	if got := policy.TierOf("代谢.体温"); got != TierNoise {
		t.Fatalf("代谢.体温 应为 noise，得到 %s", got)
	}
	if got := policy.TierOf("未知.字段"); got != TierNormal {
		t.Fatalf("未登记路径应回落默认 normal，得到 %s", got)
	}
	if got := (*Policy)(nil).TierOf("任意"); got != TierNormal {
		t.Fatalf("nil 策略应返回 normal，得到 %s", got)
	}
}

func TestThresholdOfUsesMatchingRule(t *testing.T) {
	policy := &Policy{Rules: []Rule{
		{Pattern: "循环.心率", Tier: TierNoise, Threshold: 2},
		{Pattern: "循环.*", Tier: TierNoise},
	}}
	if got := policy.ThresholdOf("循环.心率"); got != 2 {
		t.Fatalf("阈值应为 2，得到 %v", got)
	}
	if got := policy.ThresholdOf("循环.血压"); got != 0 {
		t.Fatalf("无阈值规则的阈值应为 0，得到 %v", got)
	}
}

func TestParsePolicyNormalizesInvalidValues(t *testing.T) {
	policy, err := ParsePolicy([]byte(`{"defaultTier":"weird","rules":[{"pattern":"a","tier":"weird"}],"budget":{"maxItems":-1}}`))
	if err != nil {
		t.Fatalf("解析不应失败: %v", err)
	}
	if policy.DefaultTier != TierNormal {
		t.Fatalf("非法默认等级应回落 normal，得到 %s", policy.DefaultTier)
	}
	if policy.Rules[0].Tier != TierNormal {
		t.Fatalf("非法规则等级应回落 normal，得到 %s", policy.Rules[0].Tier)
	}
	if policy.Budget.MaxItems != DEFAULT_MAX_ITEMS {
		t.Fatalf("非法条目上限应回落默认值，得到 %d", policy.Budget.MaxItems)
	}
	if policy.Version != POLICY_VERSION {
		t.Fatalf("缺省版本应补为 %d，得到 %d", POLICY_VERSION, policy.Version)
	}
}

func TestParsePolicyRejectsBrokenJSON(t *testing.T) {
	policy, err := ParsePolicy([]byte(`{`))
	if err == nil {
		t.Fatalf("坏 JSON 必须报错")
	}
	if policy == nil || policy.Budget.MaxItems != DEFAULT_MAX_ITEMS {
		t.Fatalf("解析失败也应返回可用默认策略：%+v", policy)
	}
}

func TestLoadPolicyMissingFileFallsBackToDefault(t *testing.T) {
	policy, err := LoadPolicy(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("缺文件不应报错: %v", err)
	}
	// 内置默认不含业务路径规则：全部走默认等级 normal。
	if policy.TierOf("状态.身体") != TierNormal {
		t.Fatalf("缺文件时内置默认应为中立策略，得到 %s", policy.TierOf("状态.身体"))
	}
}

func TestLoadPolicyReadsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "observation.json")
	payload := `{"version":2,"defaultTier":"noise","rules":[{"pattern":"状态.*","tier":"critical"}],"budget":{"maxItems":5,"maxChars":100,"coalesceMs":1000}}`
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("写策略文件失败: %v", err)
	}
	policy, err := LoadPolicy(path)
	if err != nil {
		t.Fatalf("读取策略失败: %v", err)
	}
	if policy.Budget.MaxItems != 5 || policy.Budget.MaxChars != 100 || policy.Budget.CoalesceMs != 1000 {
		t.Fatalf("预算未读入：%+v", policy.Budget)
	}
	if policy.TierOf("状态.身体") != TierCritical || policy.TierOf("其它") != TierNoise {
		t.Fatalf("规则或默认等级不符：%+v", policy)
	}
}
