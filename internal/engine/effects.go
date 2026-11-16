// Text effect parsing and application ("体力+5", "体力-10~-5", ranges).
//
// This file parses delta effects from free text. It deliberately keeps **no
// effect-word table**: the word is resolved against the target entity's own
// fields at apply time, so kernels (and content data) own the field names.
// Legacy Python parity mapping lives in git history, not in the source.
package engine

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/RuiCat/Neko/internal/config"
	"github.com/RuiCat/Neko/internal/entity"
)

// Effect is one parsed delta effect: the raw word and the signed delta.
type Effect struct {
	Word  string  `json:"word"`
	Delta float64 `json:"delta"`
}

// AppliedEffect is one effect that was written to an entity.
type AppliedEffect struct {
	Path  string  `json:"path"`
	Delta float64 `json:"delta"`
	New   float64 `json:"new"`
}

// fullwidthFold maps fullwidth digits/operators/letters to ASCII.
var fullwidthFold = buildFullwidthFold()

func buildFullwidthFold() map[rune]rune {
	fold := map[rune]rune{}
	digits := []rune("０１２３４５６７８９")
	for i, ch := range digits {
		fold[ch] = rune('0' + i)
	}
	for _, pair := range [][2]rune{{'＋', '+'}, {'－', '-'}, {'＝', '='}, {'：', ':'}} {
		fold[pair[0]] = pair[1]
	}
	upper := []rune("ＡＢＣＤＥＦＧＨＩＪＫＬＭＮＯＰＱＲＳＴＵＶＷＸＹＺ")
	lower := []rune("ａｂｃｄｅｆｇｈｉｊｋｌｍｎｏｐｑｒｓｔｕｖｗｘｙｚ")
	for i, ch := range upper {
		fold[ch] = rune('A' + i)
	}
	for i, ch := range lower {
		fold[ch] = rune('a' + i)
	}
	return fold
}

func normalizeASCII(text string) string {
	return strings.Map(func(ch rune) rune {
		if mapped, ok := fullwidthFold[ch]; ok {
			return mapped
		}
		return ch
	}, text)
}

var (
	effectPattern = regexp.MustCompile(`([\p{Han}]+(?:\.[\p{Han}]+)?)\s*([+\-])\s*(\d+(?:\.\d+)?)`)

	negativeRangePattern = regexp.MustCompile(
		`([\p{Han}]+(?:\.[\p{Han}]+)?)\s*(\d+(?:\.\d+)?)\s*-\s*\(\s*-\s*(\d+(?:\.\d+)?)\s*\)`)
	tildeRangePattern = regexp.MustCompile(
		`([\p{Han}]+(?:\.[\p{Han}]+)?)\s*(-?\d+(?:\.\d+)?)\s*[~～]\s*(-?\d+(?:\.\d+)?)`)
	positiveRangePattern = regexp.MustCompile(
		`([\p{Han}]+(?:\.[\p{Han}]+)?)\s*(\d+(?:\.\d+)?)\s*-\s*(\d+(?:\.\d+)?)`)
)

// expandRanges rewrites range effects to single-value effects:
// "湿润5-15" -> "湿润+5", "恐惧-10-(-5)" -> "恐惧-10",
// "体力-10~-5" -> "体力-10".
func expandRanges(text string) string {
	text = negativeRangePattern.ReplaceAllString(text, "${1}-${2}")
	text = tildeRangePattern.ReplaceAllStringFunc(text, func(match string) string {
		sub := tildeRangePattern.FindStringSubmatch(match)
		name, first := sub[1], sub[2]
		if strings.HasPrefix(first, "-") {
			return name + first
		}
		return name + "+" + first
	})
	text = positiveRangePattern.ReplaceAllString(text, "${1}+${2}")
	return text
}

// ParseEffects extracts delta effects from a text. A word is not mapped to a
// path here: only ApplyEffects can resolve it against a concrete entity.
func ParseEffects(text string) []Effect {
	effects := []Effect{}
	if text == "" {
		return effects
	}
	text = expandRanges(normalizeASCII(text))
	for _, match := range effectPattern.FindAllStringSubmatch(text, -1) {
		number, err := strconv.ParseFloat(match[3], 64)
		if err != nil {
			continue
		}
		delta := number
		if match[2] == "-" {
			delta = -number
		}
		effects = append(effects, Effect{Word: match[1], Delta: delta})
	}
	return effects
}

// ApplyEffects writes parsed delta effects to an entity. Each effect word is
// resolved against that entity's own leaf paths: an exact dotted path wins,
// otherwise the shortest path whose last segment equals the word. Unknown
// words are skipped; deltas are applied as-is (field ranges live in templates).
func ApplyEffects(target *entity.Entity, text string) []AppliedEffect {
	applied := []AppliedEffect{}
	if target == nil {
		return applied
	}
	leaves := leafPathSet(target.Snapshot())
	for _, effect := range ParseEffects(text) {
		path, ok := resolveEffectPath(target, leaves, effect.Word)
		if !ok {
			continue
		}
		current, err := target.Get(path)
		if err != nil {
			continue
		}
		// 不在这里猜 0..100：字段范围由模板注解/数据层负责，旧口径不写死在引擎里。
		newValue := float64(toNumInt(current)) + effect.Delta
		if err := target.Set(path, newValue); err != nil {
			continue
		}
		applied = append(applied, AppliedEffect{Path: path, Delta: effect.Delta, New: newValue})
	}
	return applied
}

// resolveEffectPath maps an effect word onto one of the entity's leaf paths.
// A configured effect_map entry wins (data-driven); otherwise the word is
// matched against the entity's own leaf names.
func resolveEffectPath(target *entity.Entity, leaves map[string]bool, word string) (string, bool) {
	if mapped, ok := config.Current().EffectMap[word]; ok && mapped != "" {
		return config.Current().NormalizePath(mapped), true
	}
	if strings.Contains(word, ".") {
		if leaves[word] {
			return word, true
		}
		if target.Has(word) {
			return word, true
		}
		return "", false
	}
	candidates := make([]string, 0, 4)
	for path := range leaves {
		if leafName(path) == word {
			candidates = append(candidates, path)
		}
	}
	if len(candidates) == 0 {
		return "", false
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if len(candidates[i]) != len(candidates[j]) {
			return len(candidates[i]) < len(candidates[j])
		}
		return candidates[i] < candidates[j]
	})
	return candidates[0], true
}

// leafPathSet flattens a state snapshot into leaf path set.
func leafPathSet(node map[string]any) map[string]bool {
	out := map[string]bool{}
	var walk func(prefix string, current map[string]any)
	walk = func(prefix string, current map[string]any) {
		for key, value := range current {
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			if nested, ok := value.(map[string]any); ok {
				walk(path, nested)
				continue
			}
			out[path] = true
		}
	}
	walk("", node)
	return out
}

func leafName(path string) string {
	if index := strings.LastIndex(path, "."); index >= 0 {
		return path[index+1:]
	}
	return path
}

// toNumInt mirrors Python's int(): numbers truncate, anything unparsable is 0.
func toNumInt(value any) int {
	switch typed := value.(type) {
	case nil:
		return 0
	case int:
		return typed
	case int64:
		return int(typed)
	case int32:
		return int(typed)
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	case string:
		number, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0
		}
		return number
	}
	return 0
}
