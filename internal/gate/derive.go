// derive.go fills the frozen v1 gate features from signals a live turn already
// has: the user text, the hard-state snapshot and the scene constraints. It
// never changes the feature layout — only the values that flow into it — so
// the rule gate and any trained model see real event kinds and affect values
// instead of the constant "turn".
package gate

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// SUMMARY_MAX_RUNES bounds the summary copied from user text; it matches the
// chat path's summarize so samples and packages keep the same size.
const SUMMARY_MAX_RUNES = 120

// STRONG_VALENCE is the |valence| that counts as a strongly felt emotion; it
// mirrors the rule gate's own threshold.
const STRONG_VALENCE = 0.6

// DeriveInput carries the per-turn signals available at assembly time.
type DeriveInput struct {
	// UserInput is the user text of this turn (empty for projection-only turns).
	UserInput string
	// State is the hard-state snapshot, keyed "<entityID>.<dotted path>".
	State map[string]any
	// KnowledgeBoundary is the scene's knowledge-boundary list; a non-empty
	// list marks a knowledge-boundary turn.
	KnowledgeBoundary []string
}

// DeriveFeatures returns base with event kind, summary, affect and hard-state
// hints filled from the live turn. An explicit non-"turn" event kind always
// wins; otherwise user text, then a strongly felt mood decide the kind. The
// feature layout itself is untouched (v1 stays 17 dimensions).
func DeriveFeatures(base Features, in DeriveInput) Features {
	out := base
	affect := deriveAffect(in.State)
	out.EventKind = deriveEventKind(out.EventKind, in.UserInput, affect)
	if strings.TrimSpace(out.Summary) == "" {
		out.Summary = summarizeUserInput(in.UserInput)
	}
	out.Affect = mergeAffect(out.Affect, affect)
	if hasKnowledgeBoundary(in.State, in.KnowledgeBoundary) {
		out.HardState = withKnowledgeKey(out.HardState)
	}
	return out
}

// textKindRule maps one event kind onto the user-text markers that select it.
type textKindRule struct {
	kind  string
	words []string
}

// textKindRules is ordered: more specific social signals win over questions,
// so "对不起，你别生气吗" stays an apology instead of becoming a question.
var textKindRules = []textKindRule{
	{"apology", []string{"对不起", "抱歉", "原谅我", "我错了", "别生气", "原谅我嘛"}},
	{"conflict", []string{"生气", "讨厌", "烦死", "吵架", "滚开", "闭嘴", "不许", "气死", "过分", "恨你"}},
	{"relationship", []string{"想你", "喜欢你", "爱你", "抱抱", "抱我", "亲亲", "贴贴", "蹭蹭", "撒娇", "陪我", "摸摸"}},
	{"self", []string{"自我介绍", "介绍自己", "介绍一下自己", "你是谁", "我是谁", "你的性格", "你的身份", "你自己", "自我认知"}},
	{"emotion", []string{"开心", "高兴", "难过", "伤心", "害怕", "恐惧", "紧张", "委屈", "哭了", "心烦"}},
	{"knowledge", []string{"设定", "世界观", "规则", "资料", "背景", "文档", "知识"}},
	{"question", []string{"？", "?", "吗", "为什么", "怎么", "什么", "哪", "是不是", "能不能", "可以吗", "如何", "多少"}},
}

// deriveEventKind picks the event kind: explicit hint, user text, strong mood,
// then the neutral "turn" fallback.
func deriveEventKind(explicit, userInput string, affect map[string]float64) string {
	if kind := strings.TrimSpace(explicit); kind != "" && !strings.EqualFold(kind, "turn") {
		return kind
	}
	if kind := kindFromText(userInput); kind != "" {
		return kind
	}
	if valence, ok := affect["valence"]; ok && (valence >= STRONG_VALENCE || valence <= -STRONG_VALENCE) {
		return "emotion"
	}
	return "turn"
}

// kindFromText matches the first rule whose marker appears in the user text.
func kindFromText(userInput string) string {
	text := strings.TrimSpace(userInput)
	if text == "" {
		return ""
	}
	for _, rule := range textKindRules {
		for _, word := range rule.words {
			if strings.Contains(text, word) {
				return rule.kind
			}
		}
	}
	return ""
}

// deriveAffect derives trust and valence from a single-entity snapshot. A
// multi-entity snapshot returns nothing: mixing two characters' moods would
// attribute one entity's state to another.
func deriveAffect(state map[string]any) map[string]float64 {
	entity, ok := singleEntityPrefix(state)
	if !ok {
		return nil
	}
	affect := make(map[string]float64, 2)
	if value, found := numericBySuffix(state, entity, ".关系.信赖"); found {
		affect["trust"] = clamp01(value)
	} else if value, found := numericBySuffix(state, entity, ".关系.亲密"); found {
		affect["trust"] = clamp01(value)
	}
	if value, found := maxValenceBySuffix(state, entity, ".状态.情绪", ".心情.当前"); found {
		affect["valence"] = value
	}
	if len(affect) == 0 {
		return nil
	}
	return affect
}

// singleEntityPrefix returns the entity id when every key belongs to one
// entity, so affect is only derived from an unambiguous snapshot.
func singleEntityPrefix(state map[string]any) (string, bool) {
	prefix := ""
	for key := range state {
		entity, _, found := strings.Cut(key, ".")
		if !found || entity == "" {
			continue
		}
		if prefix == "" {
			prefix = entity
			continue
		}
		if prefix != entity {
			return "", false
		}
	}
	return prefix, prefix != ""
}

// numericBySuffix reads the numeric value stored under "<entity><suffix>".
// Values above 1 are treated as 0–100 scale, matching how entity data stores
// fields such as 关系.亲密.
func numericBySuffix(state map[string]any, entity, suffix string) (float64, bool) {
	value, ok := state[entity+suffix]
	if !ok {
		return 0, false
	}
	number, ok := toFloat(value)
	if !ok {
		return 0, false
	}
	if number > 1 {
		number /= 100
	}
	return number, true
}

// maxValenceBySuffix keeps the strongest felt valence of the given suffixes.
func maxValenceBySuffix(state map[string]any, entity string, suffixes ...string) (float64, bool) {
	best := 0.0
	found := false
	for _, suffix := range suffixes {
		text, ok := state[entity+suffix].(string)
		if !ok {
			continue
		}
		value, ok := emotionValence(text)
		if !ok {
			continue
		}
		if !found || absFloat(value) > absFloat(best) {
			best = value
			found = true
		}
	}
	return best, found
}

// emotionValenceTable maps felt-mood vocabulary onto [-1, 1] valence.
var emotionValenceTable = map[string]float64{
	"开心": 0.8, "高兴": 0.8, "兴奋": 0.8, "幸福": 0.8, "激动": 0.8, "快乐": 0.8, "喜悦": 0.8, "感动": 0.8,
	"满足": 0.4, "舒服": 0.4, "放松": 0.4, "安心": 0.4, "温暖": 0.4, "期待": 0.4, "好奇": 0.4, "愉快": 0.4,
	"平静": 0.0, "一般": 0.0, "正常": 0.0, "无感": 0.0,
	"疲惫": -0.4, "犯困": -0.4, "困倦": -0.4, "累了": -0.4, "不安": -0.4, "担心": -0.4, "紧张": -0.4, "心烦": -0.4,
	"难过": -0.8, "伤心": -0.8, "悲伤": -0.8, "痛苦": -0.8, "害怕": -0.8, "恐惧": -0.8,
	"生气": -0.8, "愤怒": -0.8, "焦虑": -0.8, "绝望": -0.8, "委屈": -0.8,
}

// emotionValence scores one mood label. Unknown labels score nothing; a
// negated positive ("不开心") flips to negative.
func emotionValence(text string) (float64, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0, false
	}
	if value, ok := emotionValenceTable[trimmed]; ok {
		return negateIfNegated(trimmed, value), true
	}
	keys := make([]string, 0, len(emotionValenceTable))
	for key := range emotionValenceTable {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len([]rune(keys[i])) != len([]rune(keys[j])) {
			return len([]rune(keys[i])) > len([]rune(keys[j]))
		}
		return keys[i] < keys[j]
	})
	for _, key := range keys {
		if len([]rune(key)) < 2 || !strings.Contains(trimmed, key) {
			continue
		}
		return negateIfNegated(trimmed, emotionValenceTable[key]), true
	}
	return 0, false
}

// negateIfNegated flips a positive score when the text negates it.
func negateIfNegated(text string, value float64) float64 {
	if value <= 0 {
		return value
	}
	if strings.Contains(text, "不") || strings.Contains(text, "没") {
		return -value
	}
	return value
}

// hasKnowledgeBoundary reports whether this turn carries knowledge-boundary
// information, either as a scene constraint or as a hard-state field.
func hasKnowledgeBoundary(state map[string]any, boundary []string) bool {
	if len(boundary) > 0 {
		return true
	}
	for key := range state {
		if strings.Contains(key, EXPERT_KNOWLEDGE) {
			return true
		}
	}
	return false
}

// withKnowledgeKey copies a hard-state key set and marks the knowledge
// boundary, without mutating the caller's map.
func withKnowledgeKey(hardState map[string]any) map[string]any {
	out := make(map[string]any, len(hardState)+1)
	for key, value := range hardState {
		out[key] = value
	}
	out[EXPERT_KNOWLEDGE] = nil
	return out
}

// mergeAffect keeps caller-provided affect values and fills only the missing
// ones from the snapshot.
func mergeAffect(base, derived map[string]float64) map[string]float64 {
	if len(derived) == 0 {
		return base
	}
	out := make(map[string]float64, len(base)+len(derived))
	for key, value := range base {
		out[key] = value
	}
	for key, value := range derived {
		if _, ok := out[key]; !ok {
			out[key] = value
		}
	}
	return out
}

// summarizeUserInput copies user text into the summary slot, bounded by
// SUMMARY_MAX_RUNES.
func summarizeUserInput(userInput string) string {
	trimmed := strings.TrimSpace(userInput)
	if trimmed == "" {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) <= SUMMARY_MAX_RUNES {
		return trimmed
	}
	return string(runes[:SUMMARY_MAX_RUNES]) + "…"
}

// toFloat converts the numeric shapes entity data can hold.
func toFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		number, err := typed.Float64()
		return number, err == nil
	case string:
		number, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return number, err == nil
	}
	return 0, false
}

// clamp01 keeps a normalized value inside [0, 1].
func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

// absFloat returns the absolute value.
func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}
