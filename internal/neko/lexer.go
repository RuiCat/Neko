// lexer.go scans .neko source text into the top-level blocks consumed by the parser.
package neko

import "strings"

// ScanBlocks splits .neko source text into top-level blocks: data definitions,
// rules (block and line form), events, functions and metadata lines. The
// scanning rules mirror astrbox-plugin/script/lexer.py block for block.
func ScanBlocks(text string) []Block {
	blocks := []Block{}
	lines := strings.Split(text, "\n")
	n := len(lines)
	i := 0
	for i < n {
		line := strings.TrimSpace(stripComment(lines[i]))
		lineNo := i + 1

		if line == "" {
			i++
			continue
		}
		if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "# ") {
			i++
			continue
		}

		if strings.HasPrefix(line, "@多条件") || strings.HasPrefix(line, "@多规则") {
			rest := strings.TrimSpace(runeSliceFrom(line, 4))
			fullText := joinFrom(lines, i)
			open := findRune(fullText, '{')
			if open < 0 {
				blocks = append(blocks, Block{Kind: "at_multicond", Name: rest, Line: lineNo})
				i++
				continue
			}
			closeIdx := findBalanced(fullText, open, '{', '}')
			if closeIdx < 0 {
				blocks = append(blocks, Block{Kind: "at_multicond", Name: rest, Body: runeSliceFrom(fullText, open), Line: lineNo})
				i = n
				break
			}
			blocks = append(blocks, Block{Kind: "at_multicond", Name: rest, Body: runeSlice(fullText, open, closeIdx+1), Line: lineNo})
			i += blockAdvance(fullText, open, closeIdx)
			continue
		}

		if strings.HasPrefix(line, "@定义") || strings.HasPrefix(line, "@描述") {
			kind := "at_desc"
			if strings.HasPrefix(line, "@定义") {
				kind = "at_data"
			}
			rest := strings.TrimSpace(runeSliceFrom(line, 3))
			collected := rest
			if findRune(rest, '{') < 0 {
				j := i + 1
				for j < n && !strings.ContainsRune(lines[j], '{') {
					collected += " " + strings.TrimSpace(stripComment(lines[j]))
					j++
				}
				if j >= n {
					blocks = append(blocks, Block{Kind: kind, Name: strings.TrimSpace(collected), Line: lineNo})
					i = n
					break
				}
				i = j
			}
			name := strings.TrimSpace(strings.SplitN(strings.TrimSpace(runeSliceFrom(line, 3)), "{", 2)[0])
			fullText := joinFrom(lines, i)
			open := findRune(fullText, '{')
			if open < 0 {
				blocks = append(blocks, Block{Kind: kind, Name: name, Line: lineNo})
				i++
				continue
			}
			closeIdx := findBalanced(fullText, open, '{', '}')
			if closeIdx < 0 {
				blocks = append(blocks, Block{Kind: kind, Name: name, Body: runeSliceFrom(fullText, open), Line: lineNo})
				i = n
				break
			}
			body := stripCommentsPerLine(runeSlice(fullText, open, closeIdx+1))
			blocks = append(blocks, Block{Kind: kind, Name: name, Body: body, Line: lineNo})
			i += blockAdvance(fullText, open, closeIdx)
			continue
		}

		if strings.HasPrefix(line, "@Data") || strings.HasPrefix(line, "@data") {
			kind := "at_data_legacy"
			if strings.HasPrefix(line, "@Data") {
				kind = "at_data"
			}
			rest := strings.TrimSpace(runeSliceFrom(line, 5))
			collected := rest
			if findRune(rest, '{') < 0 {
				j := i + 1
				for j < n && !strings.ContainsRune(lines[j], '{') {
					collected += " " + strings.TrimSpace(stripComment(lines[j]))
					j++
				}
				if j >= n {
					blocks = append(blocks, Block{Kind: kind, Name: strings.TrimSpace(collected), Line: lineNo})
					i = n
					break
				}
				i = j
			}
			name := strings.TrimSpace(strings.SplitN(strings.TrimSpace(runeSliceFrom(line, 5)), "{", 2)[0])
			fullText := joinFrom(lines, i)
			open := findRune(fullText, '{')
			if open < 0 {
				blocks = append(blocks, Block{Kind: kind, Name: name, Line: lineNo})
				i++
				continue
			}
			closeIdx := findBalanced(fullText, open, '{', '}')
			if closeIdx < 0 {
				blocks = append(blocks, Block{Kind: kind, Name: name, Body: runeSliceFrom(fullText, open), Line: lineNo})
				i = n
				break
			}
			body := stripCommentsPerLine(runeSlice(fullText, open, closeIdx+1))
			blocks = append(blocks, Block{Kind: kind, Name: name, Body: body, Line: lineNo})
			i += blockAdvance(fullText, open, closeIdx)
			continue
		}

		if strings.HasPrefix(line, "@事件") {
			name := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(strings.SplitN(runeSliceFrom(line, 3), "{", 2)[0]), ":"))
			fullText := joinFrom(lines, i)
			open := findRune(fullText, '{')
			if open < 0 {
				blocks = append(blocks, Block{Kind: "at_event", Name: name, Line: lineNo})
				i++
				continue
			}
			closeIdx := findBalanced(fullText, open, '{', '}')
			if closeIdx < 0 {
				blocks = append(blocks, Block{Kind: "at_event", Name: name, Body: runeSliceFrom(fullText, open), Line: lineNo})
				i = n
				break
			}
			blocks = append(blocks, Block{Kind: "at_event", Name: name, Body: runeSlice(fullText, open, closeIdx+1), Line: lineNo})
			i += blockAdvance(fullText, open, closeIdx)
			continue
		}

		if strings.HasPrefix(line, "@func") || strings.HasPrefix(line, "@函数") {
			prefixLen := 5
			if strings.HasPrefix(line, "@函数") {
				prefixLen = 3
			}
			rawName := strings.TrimSpace(runeSliceFrom(line, prefixLen))
			fullText := joinFrom(lines, i)
			open := findRune(fullText, '{')
			if open < 0 {
				blocks = append(blocks, Block{Kind: "at_func", Name: rawName, Line: lineNo})
				i++
				continue
			}
			closeIdx := findBalanced(fullText, open, '{', '}')
			if closeIdx < 0 {
				blocks = append(blocks, Block{Kind: "at_func", Name: rawName, Body: runeSliceFrom(fullText, open), Line: lineNo})
				i = n
				break
			}
			header := strings.TrimSpace(strings.SplitN(rawName, "{", 2)[0])
			blocks = append(blocks, Block{Kind: "at_func", Name: header, Body: runeSlice(fullText, open, closeIdx+1), Line: lineNo})
			i += blockAdvance(fullText, open, closeIdx)
			continue
		}

		if strings.HasPrefix(line, "@规则") {
			closeBFirst := -1
			if openB := findRune(line, '['); openB >= 0 {
				closeBFirst = findBalanced(line, openB, '[', ']')
			}
			if closeBFirst >= 0 && closeBFirst+1 < runeCount(line) && runeAt(line, closeBFirst+1) == ':' {
				fullText := joinFrom(lines, i)
				open := findRune(fullText, '{')
				if open < 0 {
					blocks = append(blocks, Block{Kind: "at_rule_range", Name: line, Line: lineNo})
					i++
					continue
				}
				closeIdx := findBalanced(fullText, open, '{', '}')
				if closeIdx < 0 {
					blocks = append(blocks, Block{Kind: "at_rule_range", Name: line, Body: runeSliceFrom(fullText, open), Line: lineNo})
					i = n
					break
				}
				blocks = append(blocks, Block{Kind: "at_rule_range", Name: line, Body: runeSlice(fullText, open, closeIdx+1), Line: lineNo})
				i += blockAdvance(fullText, open, closeIdx)
				continue
			}

			openB := findRune(line, '[')
			if openB < 0 {
				blocks = append(blocks, Block{Kind: "at_rule", Name: line, Line: lineNo})
				i++
				continue
			}
			closeB := findBalanced(line, openB, '[', ']')
			var header, rest string
			if closeB < 0 {
				collected := line
				j := i + 1
				for j < n && !strings.ContainsRune(lines[j], ']') {
					collected += "\n" + lines[j]
					j++
				}
				if j >= n {
					blocks = append(blocks, Block{Kind: "at_rule", Name: collected, Line: lineNo})
					i = n
					break
				}
				collected += "\n" + lines[j]
				lb := findRune(collected, '[')
				rb := rfindRune(collected, ']')
				header = runeSlice(collected, lb, rb+1)
				rest = strings.TrimSpace(runeSliceFrom(collected, rb+1))
				i = j
			} else {
				header = runeSlice(line, openB, closeB+1)
				rest = strings.TrimSpace(runeSliceFrom(line, closeB+1))
				i++
			}

			fullText := joinFrom(lines, i)
			bodyText := fullText
			if rest != "" {
				bodyText = rest + "\n" + fullText
			}
			open := findRune(bodyText, '{')
			if open < 0 {
				blocks = append(blocks, Block{Kind: "at_rule", Name: header, Line: lineNo})
				i++
				continue
			}
			closeIdx := findBalanced(bodyText, open, '{', '}')
			if closeIdx < 0 {
				blocks = append(blocks, Block{Kind: "at_rule", Name: header, Body: runeSliceFrom(bodyText, open), Line: lineNo})
				i = n
				break
			}
			blocks = append(blocks, Block{Kind: "at_rule", Name: header, Body: runeSlice(bodyText, open, closeIdx+1), Line: lineNo})
			advance := blockAdvance(bodyText, open, closeIdx)
			if rest != "" {
				advance--
			}
			i += advance
			continue
		}

		if strings.HasPrefix(line, "#规则") {
			blocks = append(blocks, Block{Kind: "hash_rule", Name: line, Line: lineNo})
			i++
			continue
		}

		if strings.HasPrefix(line, "#") {
			blocks = append(blocks, Block{Kind: "meta", Name: line, Line: lineNo})
			i++
			continue
		}

		i++
	}
	return blocks
}

// blockAdvance returns how many source lines a balanced { ... } block spans,
// starting from the line that holds the opening brace.
func blockAdvance(bodyText string, open, closeIdx int) int {
	return countNewlines(runeSlice(bodyText, 0, closeIdx+1)) - countNewlines(runeSlice(bodyText, 0, open)) + 1
}
