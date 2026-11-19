// entityBlock.go parses the @实体 composition header: a type name followed by
// an optional parent list, e.g. `@实体 <type>:<id> [<parent>, <parent>] { ... }`.
package neko

import (
	"fmt"
	"regexp"
	"strings"
)

var entityTypeNameRe = regexp.MustCompile(`^[\p{L}\p{N}_:]+$`)

// scanEntityBlock collects one @实体 block from lines[start]. It mirrors the
// @定义 scanner: the header may span lines, and the body is one balanced
// { ... } JSON object.
func scanEntityBlock(lines []string, start int) (Block, int) {
	n := len(lines)
	lineNo := start + 1
	line := strings.TrimSpace(stripComment(lines[start]))
	rest := strings.TrimSpace(runeSliceFrom(line, 3)) // runeSliceFrom("@实体", 3)
	collected := rest
	anchor := start
	if findRune(rest, '{') < 0 {
		j := start + 1
		for j < n && !strings.ContainsRune(lines[j], '{') {
			collected += " " + strings.TrimSpace(stripComment(lines[j]))
			j++
		}
		if j >= n {
			return Block{Kind: "at_entity", Name: strings.TrimSpace(collected), Line: lineNo}, n
		}
		collected += " " + strings.TrimSpace(strings.SplitN(strings.TrimSpace(stripComment(lines[j])), "{", 2)[0])
		anchor = j
	}
	header := strings.TrimSpace(strings.SplitN(collected, "{", 2)[0])
	fullText := joinFrom(lines, anchor)
	open := findRune(fullText, '{')
	if open < 0 {
		return Block{Kind: "at_entity", Name: header, Line: lineNo}, anchor + 1
	}
	closeIdx := findBalanced(fullText, open, '{', '}')
	if closeIdx < 0 {
		return Block{Kind: "at_entity", Name: header, Body: runeSliceFrom(fullText, open), Line: lineNo}, n
	}
	body := stripCommentsPerLine(runeSlice(fullText, open, closeIdx+1))
	next := anchor + blockAdvance(fullText, open, closeIdx)
	return Block{Kind: "at_entity", Name: header, Body: body, Line: lineNo}, next
}

// ParseEntityHeader splits `名称 [父1, 父2]` into a type name and its parent
// list. Whitespace separates the name from `[`; both ASCII and full-width
// commas are accepted between parents.
func ParseEntityHeader(header string) (string, []string, error) {
	return splitNameAndParents(header, "@实体", "父模板", true)
}

// ParseInstanceHeader splits `<分类>:<名称> [类型, ...]` for @实例. The name is
// kept whole (the last colon segment is the entity id); the bracketed list
// names the types whose fields the instance draws from.
func ParseInstanceHeader(header string) (string, []string, error) {
	return splitNameAndParents(header, "@实例", "类型", false)
}

// splitNameAndParents parses `<name> [item, item]` for the @实体/@实例 headers.
// The name keeps colons (namespace / classification path); the list entries are
// validated as template references.
func splitNameAndParents(header, keyword, itemLabel string, strictName bool) (string, []string, error) {
	text := strings.TrimSpace(header)
	if text == "" {
		return "", nil, fmt.Errorf("%s 缺少名称", keyword)
	}
	validName := func(name string) bool {
		if strictName {
			return entityTypeNameRe.MatchString(name)
		}
		// 实体 id 允许连字符、括号等：只要非空、不含方括号和空白即可。
		return name != "" && !strings.ContainsAny(name, "[] \t\r\n")
	}
	open := strings.IndexRune(text, '[')
	if open < 0 {
		if !validName(text) {
			return "", nil, fmt.Errorf("%s 名称非法: %q", keyword, text)
		}
		return text, nil, nil
	}
	close := strings.LastIndex(text, "]")
	if close < open {
		return "", nil, fmt.Errorf("%s 列表缺少 ']'", keyword)
	}
	name := strings.TrimSpace(text[:open])
	if !validName(name) {
		return "", nil, fmt.Errorf("%s 名称非法: %q", keyword, name)
	}
	if trailing := strings.TrimSpace(text[close+1:]); trailing != "" {
		return "", nil, fmt.Errorf("%s 列表之后存在多余内容: %q", keyword, trailing)
	}
	inside := strings.TrimSpace(text[open+1 : close])
	if inside == "" {
		return name, nil, nil
	}
	items := []string{}
	seen := map[string]bool{}
	for _, raw := range strings.FieldsFunc(inside, func(r rune) bool { return r == ',' || r == '，' }) {
		item := strings.TrimSpace(raw)
		if item == "" {
			continue
		}
		if !entityTypeNameRe.MatchString(item) {
			return "", nil, fmt.Errorf("%s %s名非法: %q", keyword, itemLabel, item)
		}
		if seen[item] {
			return "", nil, fmt.Errorf("%s %s重复: %q", keyword, itemLabel, item)
		}
		seen[item] = true
		items = append(items, item)
	}
	return name, items, nil
}
