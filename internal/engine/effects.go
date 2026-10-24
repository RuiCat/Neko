// Text effect parsing and application ("体力+5", "体力-10~-5", ranges).
//
// This file is the Go port of astrbox-plugin/state_machine/effect_parser.py,
// limited to the effect (delta) half; assignments live in assign.go.
package engine

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/ruicat/astrbox/internal/config"
	"github.com/ruicat/astrbox/internal/entity"
)

// Effect is one parsed delta effect.
type Effect struct {
	Path  string  `json:"path"`
	Delta float64 `json:"delta"`
}

// AppliedEffect is one effect that was written to an entity.
type AppliedEffect struct {
	Path  string  `json:"path"`
	Delta float64 `json:"delta"`
	New   float64 `json:"new"`
}

// effectMap maps effect words to entity field paths (ported verbatim).
var effectMap = map[string]string{
	"撒娇":      "关系.撒娇欲",
	"撒娇欲":     "关系.撒娇欲",
	"撒娇倾向":    "关系.撒娇欲",
	"傲娇":      "行为.傲娇倾向",
	"傲娇倾向":    "行为.傲娇倾向",
	"黏人":      "行为.黏人",
	"黏人程度":    "行为.黏人",
	"坦率":      "行为.坦率",
	"主动":      "行为.主动",
	"服从":      "行为.服从",
	"抗拒":      "行为.抗拒",
	"好奇":      "行为.好奇心",
	"好奇心":     "行为.好奇心",
	"探索":      "关系.探索欲",
	"安全感":     "关系.安全感",
	"亲密度":     "关系.亲密",
	"亲密":      "关系.亲密",
	"信赖":      "关系.信赖",
	"爱意":      "关系.爱意",
	"数值.亲密":   "数值.亲密",
	"数值.信赖":   "数值.信赖",
	"数值.爱意":   "数值.爱意",
	"数值.好感度":  "数值.好感度",
	"数值.安全感":  "数值.安全感",
	"数值.撒娇欲":  "数值.撒娇欲",
	"害羞":      "关系.害羞",
	"害羞度":     "关系.害羞",
	"兴奋":      "器官状态.扩展心理.兴奋度",
	"兴奋度":     "器官状态.扩展心理.兴奋度",
	"快感":      "器官状态.基础生理.快感程度",
	"满足":      "器官状态.扩展心理.满足度",
	"满足度":     "器官状态.扩展心理.满足度",
	"湿润":      "器官状态.基础生理.湿润度",
	"湿润度":     "器官状态.基础生理.湿润度",
	"收缩":      "器官状态.基础生理.收缩状态",
	"收缩状态":    "器官状态.基础生理.收缩状态",
	"幸福感":     "器官状态.扩展心理.幸福感",
	"幸福":      "器官状态.扩展心理.幸福感",
	"疲劳":      "身体.疲劳",
	"疲劳度":     "身体.疲劳",
	"疼痛":      "身体.疼痛",
	"疼痛程度":    "身体.疼痛",
	"恐惧":      "状态.恐惧",
	"恐惧程度":    "状态.恐惧",
	"体力":      "身体.体力",
	"体力值":     "身体.体力",
	"清洁":      "身体.清洁",
	"清洁度":     "身体.清洁",
	"舒适":      "身体.舒适",
	"舒适度":     "身体.舒适",
	"活力":      "身体.活力",
	"活力值":     "身体.活力",
	"饱食":      "身体.饱食",
	"饱食度":     "身体.饱食",
	"水分":      "身体.水分",
	"口渴":      "身体.水分",
	"口渴度":     "身体.水分",
	"体温":      "身体.体温",
	"伤害":      "器官状态.R18G伤害.伤害程度",
	"伤害程度":    "器官状态.R18G伤害.伤害程度",
	"创伤":      "伤害.创伤",
	"创伤累积":    "伤害.创伤",
	"出血":      "伤害.出血",
	"出血程度":    "伤害.出血",
	"成瘾":      "器官状态.扩展心理.成瘾度",
	"成瘾度":     "器官状态.扩展心理.成瘾度",
	"期待":      "器官状态.扩展心理.期待度",
	"期待度":     "器官状态.扩展心理.期待度",
	"信任":      "器官状态.扩展心理.信任度",
	"信任度":     "器官状态.扩展心理.信任度",
	"羞耻":      "器官状态.扩展心理.羞耻感",
	"羞耻感":     "器官状态.扩展心理.羞耻感",
	"顺从":      "行为.服从",
	"服从倾向":    "行为.服从",
	"主动程度":    "行为.主动",
	"依赖":      "关系.性依赖",
	"性依赖":     "关系.性依赖",
	"阴道.疼痛":   "器官状态.基础生理.疼痛程度",
	"宫颈.疼痛":   "器官状态.基础生理.疼痛程度",
	"子宫.疼痛":   "器官状态.基础生理.疼痛程度",
	"输卵管.疼痛":  "器官状态.基础生理.疼痛程度",
	"阴道.损伤":   "器官状态.R18G伤害.伤害程度",
	"宫颈.损伤":   "器官状态.R18G伤害.伤害程度",
	"卵巢.损伤":   "器官状态.R18G伤害.伤害程度",
	"马眼.损伤":   "器官状态.R18G伤害.伤害程度",
	"子宫.损伤":   "器官状态.R18G伤害.伤害程度",
	"输卵管.损伤":  "器官状态.R18G伤害.伤害程度",
	"直肠.损伤":   "器官状态.R18G伤害.伤害程度",
	"腹膜.损伤":   "器官状态.R18G伤害.伤害程度",
	"肛道.损伤":   "器官状态.R18G伤害.伤害程度",
	"口腔.损伤":   "器官状态.R18G伤害.伤害程度",
	"咽喉.损伤":   "器官状态.R18G伤害.伤害程度",
	"猫耳.损伤":   "器官状态.R18G伤害.伤害程度",
	"尾巴.损伤":   "器官状态.R18G伤害.伤害程度",
	"卵巢.敏感度":  "器官状态.扩展心理.兴奋度",
	"口腔.敏感度":  "器官状态.扩展心理.兴奋度",
	"咽喉.敏感度":  "器官状态.扩展心理.兴奋度",
	"猫耳.敏感度":  "器官状态.扩展心理.兴奋度",
	"肉垫.敏感度":  "器官状态.扩展心理.兴奋度",
	"阴道.扩张度":  "器官状态.基础生理.湿润度",
	"口腔.扩张度":  "器官状态.基础生理.湿润度",
	"外翻卵巢.精液": "体液追踪.子宫内精液",
	"敏感":      "器官状态.扩展心理.兴奋度",
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

// ParseEffects extracts delta effects from a text.
func ParseEffects(text string) []Effect {
	effects := []Effect{}
	if text == "" {
		return effects
	}
	text = expandRanges(normalizeASCII(text))
	for _, match := range effectPattern.FindAllStringSubmatch(text, -1) {
		path, ok := effectMap[match[1]]
		if !ok {
			continue
		}
		path = config.Current().NormalizePath(path)
		number, err := strconv.ParseFloat(match[3], 64)
		if err != nil {
			continue
		}
		delta := number
		if match[2] == "-" {
			delta = -number
		}
		effects = append(effects, Effect{Path: path, Delta: delta})
	}
	return effects
}

// ApplyEffects writes parsed delta effects to an entity, clamping 0..100 and
// preserving legacy {value,unit,desc} wrappers.
func ApplyEffects(target *entity.Entity, text string) []AppliedEffect {
	applied := []AppliedEffect{}
	if target == nil {
		return applied
	}
	for _, effect := range ParseEffects(text) {
		current := target.GetOrNone(effect.Path)
		wrapper, isWrapper := current.(map[string]any)
		if isWrapper {
			current = wrapper["value"]
		}
		newValue := float64(toNumInt(current)) + effect.Delta
		if newValue < 0 {
			newValue = 0
		}
		if newValue > 100 {
			newValue = 100
		}
		if isWrapper {
			updated := make(map[string]any, len(wrapper))
			for key, value := range wrapper {
				updated[key] = value
			}
			updated["value"] = newValue
			if err := target.Set(effect.Path, updated); err != nil {
				continue
			}
		} else if err := target.Set(effect.Path, newValue); err != nil {
			continue
		}
		applied = append(applied, AppliedEffect{Path: effect.Path, Delta: effect.Delta, New: newValue})
	}
	return applied
}

// toNumInt mirrors Python's int(): numbers truncate, dicts unwrap "value",
// anything unparsable becomes 0.
func toNumInt(value any) int {
	if wrapper, ok := value.(map[string]any); ok {
		value = wrapper["value"]
	}
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
