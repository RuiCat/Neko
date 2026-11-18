// extractEffect.go classifies one effect literal into the closed C2 set:
// number -> set, signed +N/-N string -> add, quoted string -> text, bool ->
// bool. Anything else is returned as unclassified instead of being guessed.
package spec

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var signedNumberPattern = regexp.MustCompile(`^[+-][0-9]+(\.[0-9]+)?$`)

func classifyEffect(path string, value Value, source Source) (EffectSpec, *UnclassifiedEffect) {
	effect, rejected := classifyEffectValue(path, value, source)
	if rejected != nil {
		return effect, rejected
	}
	effect.Target, effect.TargetReason = effectTarget(path)
	return effect, nil
}

func classifyEffectValue(path string, value Value, source Source) (EffectSpec, *UnclassifiedEffect) {
	switch value.Kind {
	case VALUE_NUMBER:
		return EffectSpec{
			Path:   path,
			Kind:   EFFECT_KIND_SET,
			Value:  json.Number(value.Text),
			Source: source,
		}, nil
	case VALUE_BOOL:
		return EffectSpec{
			Path:   path,
			Kind:   EFFECT_KIND_BOOL,
			Value:  value.Bool,
			Source: source,
		}, nil
	case VALUE_STRING:
		text := strings.TrimSpace(value.Text)
		if signedNumberPattern.MatchString(text) {
			number := strings.TrimPrefix(text, "+")
			return EffectSpec{
				Path:   path,
				Kind:   EFFECT_KIND_ADD,
				Value:  json.Number(number),
				Source: source,
			}, nil
		}
		return EffectSpec{
			Path:   path,
			Kind:   EFFECT_KIND_TEXT,
			Value:  value.Text,
			Source: source,
		}, nil
	default:
		return EffectSpec{}, &UnclassifiedEffect{
			Path:   path,
			Source: source,
			Raw:    fmt.Sprintf("<%s>", valueKindName(value.Kind)),
			Reason: "literal is not a number, signed string, quoted string or bool",
		}
	}
}

func valueKindName(kind ValueKind) string {
	switch kind {
	case VALUE_STRING:
		return "string"
	case VALUE_NUMBER:
		return "number"
	case VALUE_BOOL:
		return "bool"
	case VALUE_OBJECT:
		return "object"
	case VALUE_ARRAY:
		return "array"
	default:
		return "unknown"
	}
}
