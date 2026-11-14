// derive_test.go pins the live-signal derivation: text → event kind, snapshot →
// affect, constraints → knowledge boundary, and the frozen v1 layout staying
// untouched.
package gate

import (
	"strings"
	"testing"
)

// TestDeriveEventKindFromText pins the user-text vocabulary mapping.
func TestDeriveEventKindFromText(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"对不起，我错了", "apology"},
		{"你不能这样，我生气了", "conflict"},
		{"我想你了，抱抱", "relationship"},
		{"猫猫介绍一下自己吧", "self"},
		{"今天好难过", "emotion"},
		{"猫猫的设定是什么", "knowledge"},
		{"你为什么这么晚才回来？", "question"},
		{"今天吃鱼", "turn"},
		{"", "turn"},
	}
	for _, testCase := range cases {
		if got := deriveEventKind("turn", testCase.text, nil); got != testCase.want {
			t.Errorf("deriveEventKind(%q) = %q, want %q", testCase.text, got, testCase.want)
		}
	}
}

// TestDeriveEventKindExplicitWins proves an explicit non-turn kind is kept.
func TestDeriveEventKindExplicitWins(t *testing.T) {
	if got := deriveEventKind("question", "抱抱", nil); got != "question" {
		t.Fatalf("显式事件类型被改写: %q", got)
	}
	if got := deriveEventKind("", "今天吃鱼", nil); got != "turn" {
		t.Fatalf("空事件类型应回落 turn: %q", got)
	}
}

// TestDeriveEventKindFromStrongMood covers the text-less projection path.
func TestDeriveEventKindFromStrongMood(t *testing.T) {
	if got := deriveEventKind("turn", "", map[string]float64{"valence": 0.8}); got != "emotion" {
		t.Fatalf("强情绪应判 emotion: %q", got)
	}
	if got := deriveEventKind("turn", "", map[string]float64{"valence": 0.4}); got != "turn" {
		t.Fatalf("温和情绪不应改写事件类型: %q", got)
	}
}

// TestDeriveSummary pins summary filling and truncation.
func TestDeriveSummary(t *testing.T) {
	derived := DeriveFeatures(Features{}, DeriveInput{UserInput: "  在吗  "})
	if derived.Summary != "在吗" {
		t.Fatalf("摘要未从用户文本填充: %q", derived.Summary)
	}
	long := strings.Repeat("喵", SUMMARY_MAX_RUNES+20)
	derived = DeriveFeatures(Features{}, DeriveInput{UserInput: long})
	if runes := []rune(derived.Summary); len(runes) != SUMMARY_MAX_RUNES+1 {
		t.Fatalf("摘要未截断到 %d 字: %d", SUMMARY_MAX_RUNES, len(runes))
	}
	kept := DeriveFeatures(Features{Summary: "已有摘要"}, DeriveInput{UserInput: "在吗"})
	if kept.Summary != "已有摘要" {
		t.Fatalf("已有摘要被覆盖: %q", kept.Summary)
	}
}

// TestDeriveAffectSingleEntity pins trust/valence derivation from one entity.
func TestDeriveAffectSingleEntity(t *testing.T) {
	state := map[string]any{
		"猫猫.关系.亲密": 30,
		"猫猫.状态.情绪": "平静",
		"猫猫.心情.当前": "满足",
	}
	derived := DeriveFeatures(Features{}, DeriveInput{State: state})
	if trust := derived.Affect["trust"]; trust != 0.3 {
		t.Fatalf("trust = %v, want 0.3", trust)
	}
	if valence := derived.Affect["valence"]; valence != 0.4 {
		t.Fatalf("valence = %v, want 0.4", valence)
	}
}

// TestDeriveAffectMultiEntitySkips proves mixed snapshots never attribute one
// entity's state to another.
func TestDeriveAffectMultiEntitySkips(t *testing.T) {
	state := map[string]any{
		"猫猫.关系.亲密": 100,
		"梦梦.心情.当前": "开心",
	}
	derived := DeriveFeatures(Features{}, DeriveInput{State: state})
	if len(derived.Affect) != 0 {
		t.Fatalf("多实体快照不应派生 affect: %+v", derived.Affect)
	}
}

// TestDeriveAffectNegation covers negated positive moods.
func TestDeriveAffectNegation(t *testing.T) {
	derived := DeriveFeatures(Features{}, DeriveInput{State: map[string]any{"猫猫.心情.当前": "不开心"}})
	if valence := derived.Affect["valence"]; valence != -0.8 {
		t.Fatalf("否定情绪应为 -0.8，得到 %v", valence)
	}
}

// TestDeriveKnowledgeBoundary covers both knowledge-boundary sources.
func TestDeriveKnowledgeBoundary(t *testing.T) {
	fromConstraint := DeriveFeatures(Features{}, DeriveInput{KnowledgeBoundary: []string{"世界设定"}})
	if _, ok := fromConstraint.HardState[EXPERT_KNOWLEDGE]; !ok {
		t.Fatalf("场景约束未标记知识边界: %+v", fromConstraint.HardState)
	}
	fromState := DeriveFeatures(Features{}, DeriveInput{State: map[string]any{"猫猫.知识边界.范围": "只谈当下"}})
	if _, ok := fromState.HardState[EXPERT_KNOWLEDGE]; !ok {
		t.Fatalf("硬状态未标记知识边界: %+v", fromState.HardState)
	}
	kept := DeriveFeatures(Features{HardState: map[string]any{"其他": nil}}, DeriveInput{})
	if _, ok := kept.HardState[EXPERT_KNOWLEDGE]; ok {
		t.Fatalf("无知识边界信号时不应新增键: %+v", kept.HardState)
	}
}

// TestDeriveMergeAffect keeps caller-provided values.
func TestDeriveMergeAffect(t *testing.T) {
	base := Features{Affect: map[string]float64{"trust": 0.1}}
	state := map[string]any{"猫猫.关系.亲密": 90}
	derived := DeriveFeatures(base, DeriveInput{State: state})
	if derived.Affect["trust"] != 0.1 {
		t.Fatalf("调用方 affect 被覆盖: %v", derived.Affect["trust"])
	}
	if base.Affect["trust"] != 0.1 {
		t.Fatalf("调用方 map 被就地修改: %+v", base.Affect)
	}
}

// TestEmotionValenceVocabulary pins the mood lexicon lookups.
func TestEmotionValenceVocabulary(t *testing.T) {
	cases := []struct {
		text  string
		want  float64
		found bool
	}{
		{"开心", 0.8, true},
		{"有点开心", 0.8, true},
		{"很平静", 0.0, true},
		{"难过", -0.8, true},
		{"半梦半醒被亲懵了", 0, false},
	}
	for _, testCase := range cases {
		got, found := emotionValence(testCase.text)
		if found != testCase.found || (found && got != testCase.want) {
			t.Errorf("emotionValence(%q) = (%v, %v), want (%v, %v)", testCase.text, got, found, testCase.want, testCase.found)
		}
	}
}
