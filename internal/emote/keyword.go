// keyword.go is the third tier: the pure keyword scorer of emote_matcher.py.
// It runs when neither vector artifact exists, so the tool still answers (with
// lower quality) on a data directory that only has the index.
package emote

import (
	"regexp"
	"sort"
	"strings"
)

// keywordIntentMap mirrors emote_matcher.INTENT_MAP (14 categories).
var keywordIntentMap = []struct {
	name     string
	keywords []string
}{
	{"开心", []string{"开心", "高兴", "哈哈", "好耶", "嘻嘻", "快乐", "笑", "得意", "棒"}},
	{"难过", []string{"难过", "伤心", "哭", "呜呜", "委屈", "失落", "emo", "想哭"}},
	{"生气", []string{"生气", "愤怒", "气", "烦", "讨厌", "哼", "怒"}},
	{"惊讶", []string{"惊讶", "震惊", "天哪", "哇", "吃惊", "不会吧", "吓"}},
	{"害羞", []string{"害羞", "脸红", "不好意思", "羞涩", "嘤"}},
	{"撒娇", []string{"撒娇", "蹭", "抱抱", "贴贴", "要亲亲", "求", "喵"}},
	{"爱", []string{"爱", "喜欢", "想你", "亲亲", "心", "么么", "喜欢你"}},
	{"困", []string{"困", "睡觉", "晚安", "打哈欠", "疲惫", "累了"}},
	{"疑问", []string{"?", "？", "疑问", "疑惑", "什么", "为什么", "嗯?"}},
	{"嫌弃", []string{"嫌弃", "无语", "翻白眼", "呵呵", "切", "嫌弃"}},
	{"加油", []string{"加油", "努力", "冲", "奋斗", "坚持"}},
	{"害怕", []string{"害怕", "怕", "恐怖", "吓死", "救命"}},
	{"吃货", []string{"吃", "饿", "蛋糕", "好吃", "零食", "甜品", "干饭"}},
	{"日常", []string{"早安", "晚安", "你好", "再见", "谢谢", "拜拜"}},
}

// hanWordPattern is Python's r'[\u4e00-\u9fff]{2,4}'.
var hanWordPattern = regexp.MustCompile(`[\x{4e00}-\x{9fff}]{2,4}`)

// searchKeyword scores every index row by category, keyword and description
// matches, mirroring emote_matcher.find_emotes.
func searchKeyword(data *DataSet, text string, top int) []Result {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	intentCategories := map[string]bool{}
	for _, intent := range keywordIntentMap {
		if containsAny(text, intent.keywords) {
			intentCategories[intent.name] = true
		}
	}
	words := uniqueHanWords(text)
	type keywordHit struct {
		score int
		name  string
	}
	hits := make([]keywordHit, 0, 64)
	for _, name := range data.IndexOrder {
		entry := data.Index[name]
		score := 0
		for category := range intentCategories {
			if hasCategory(entry.Categories, category) {
				score += 10
			}
		}
		for _, word := range words {
			if containsString(entry.Keywords, word) {
				score += 3
			}
		}
		for _, word := range words {
			if strings.Contains(entry.Desc, word) {
				score += 2
			}
		}
		if hasCategory(entry.Categories, "开心") && hasCategory(entry.Categories, "爱") {
			score--
		}
		if hasCategory(entry.Categories, "其他") {
			score -= 2
		}
		if score > 0 {
			hits = append(hits, keywordHit{score: score, name: name})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	if len(hits) > top {
		hits = hits[:top]
	}
	results := make([]Result, 0, len(hits))
	for _, hit := range hits {
		entry := data.Index[hit.name]
		path, missing := data.pathFor(hit.name)
		results = append(results, Result{
			File:        hit.name,
			Desc:        truncateRunes(entry.Desc, 100),
			Score:       float64(hit.score),
			Categories:  entry.Categories,
			IntentBonus: 0,
			Cluster:     -1,
			Path:        path,
			Missing:     missing,
		})
	}
	return results
}

// uniqueHanWords returns the distinct 2-4 Han-character runs of text.
func uniqueHanWords(text string) []string {
	matches := hanWordPattern.FindAllString(text, -1)
	seen := map[string]bool{}
	words := make([]string, 0, len(matches))
	for _, match := range matches {
		if seen[match] {
			continue
		}
		seen[match] = true
		words = append(words, match)
	}
	return words
}

// containsString reports exact membership of a string slice.
func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
