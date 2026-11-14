// features.go defines the gate's model input vector: a fixed, versioned layout
// shared by training-sample export and model inference. The layout is frozen in
// v1 — appending a feature means a new model version, never a silent resize.
package gate

import (
	"strings"
)

// eventKinds is the one-hot order for the structured event kind. Unknown or
// empty kinds light the trailing "unknown" slot instead.
var eventKinds = [9]string{
	"relationship", "conflict", "apology", "question", "knowledge", "emotion", "self", "event", "turn",
}

// feature layout: 9 one-hot kinds + unknown + summary(2) + affect(4) + hardstate(1).
const (
	featureKindOffset   = 0
	featureUnknownIndex = featureKindOffset + len(eventKinds)
	// SUMMARY_PRESENT_INDEX 是「摘要非空」的 0/1 位。
	SUMMARY_PRESENT_INDEX = featureUnknownIndex + 1
	// SUMMARY_LENGTH_INDEX 是摘要长度归一化到 [0,1] 的连续位（200 字符封顶）。
	SUMMARY_LENGTH_INDEX = SUMMARY_PRESENT_INDEX + 1
	// TRUST_PRESENT_INDEX 标记 Affect 是否带 trust。
	TRUST_PRESENT_INDEX = SUMMARY_LENGTH_INDEX + 1
	// TRUST_INDEX 是 trust 数值（缺失记 0；是否可信看 TRUST_PRESENT_INDEX）。
	TRUST_INDEX = TRUST_PRESENT_INDEX + 1
	// VALENCE_PRESENT_INDEX 标记 Affect 是否带 valence。
	VALENCE_PRESENT_INDEX = TRUST_INDEX + 1
	// VALENCE_INDEX 是 valence 数值（缺失记 0）。
	VALENCE_INDEX = VALENCE_PRESENT_INDEX + 1
	// HARDSTATE_KNOWLEDGE_INDEX 标记 HardState 是否带「知识边界」类字段。
	HARDSTATE_KNOWLEDGE_INDEX = VALENCE_INDEX + 1
	// FeatureSizeV1 是 v1 特征向量维数；模型输入层必须等于它。
	FeatureSizeV1 = HARDSTATE_KNOWLEDGE_INDEX + 1
)

// IsKnownEventKind reports whether kind is one of the v1 one-hot event kinds;
// teaching tools use it to reject typos before they light the unknown slot.
func IsKnownEventKind(kind string) bool {
	normalized := strings.ToLower(strings.TrimSpace(kind))
	for _, known := range eventKinds {
		if normalized == known {
			return true
		}
	}
	return false
}

// SemanticExpertIndex returns the index of name in SEMANTIC_EXPERTS, or -1.
func SemanticExpertIndex(name string) int {
	for index, expert := range SEMANTIC_EXPERTS {
		if name == expert {
			return index
		}
	}
	return -1
}

// FeatureNamesV1 returns the v1 feature names in vector order (for docs/tests).
func FeatureNamesV1() []string {
	names := make([]string, 0, FeatureSizeV1)
	for _, kind := range eventKinds {
		names = append(names, "kind:"+kind)
	}
	names = append(names,
		"kind:unknown",
		"summary:present",
		"summary:length",
		"affect:trust:present",
		"affect:trust",
		"affect:valence:present",
		"affect:valence",
		"hardstate:knowledge",
	)
	return names
}

// FeatureVectorV1 maps gate.Features to the frozen v1 vector. The same function
// is used to export training samples and to run a model gate, so both sides can
// never drift: any feature change must land here and bump the model version.
func FeatureVectorV1(features Features) []float64 {
	vector := make([]float64, FeatureSizeV1)
	kind := strings.ToLower(strings.TrimSpace(features.EventKind))
	matched := false
	for index, candidate := range eventKinds {
		if kind == candidate {
			vector[featureKindOffset+index] = 1
			matched = true
			break
		}
	}
	if !matched {
		vector[featureUnknownIndex] = 1
	}
	summary := strings.TrimSpace(features.Summary)
	if summary != "" {
		vector[SUMMARY_PRESENT_INDEX] = 1
		length := float64(len([]rune(summary))) / 200
		if length > 1 {
			length = 1
		}
		vector[SUMMARY_LENGTH_INDEX] = length
	}
	if trust, ok := features.Affect["trust"]; ok {
		vector[TRUST_PRESENT_INDEX] = 1
		vector[TRUST_INDEX] = trust
	}
	if valence, ok := features.Affect["valence"]; ok {
		vector[VALENCE_PRESENT_INDEX] = 1
		vector[VALENCE_INDEX] = valence
	}
	for key := range features.HardState {
		if strings.Contains(key, EXPERT_KNOWLEDGE) {
			vector[HARDSTATE_KNOWLEDGE_INDEX] = 1
			break
		}
	}
	return vector
}
