// runeText.go provides code-point-indexed string helpers shared by the lexer and
// parser; index arithmetic mirrors the Python reference, which counts code points.
package neko

import (
	"strings"
	"unicode/utf8"
)

// runeCount reports the number of code points in s.
func runeCount(s string) int { return utf8.RuneCountInString(s) }

// runeAt returns the code point at rune index i, or 0 when out of range.
func runeAt(s string, i int) rune {
	if i < 0 {
		return 0
	}
	idx := 0
	for _, r := range s {
		if idx == i {
			return r
		}
		idx++
	}
	return 0
}

// runeSlice mimics Python s[start:end] over code points.
func runeSlice(s string, start, end int) string {
	r := []rune(s)
	n := len(r)
	if start < 0 {
		start += n
	}
	if end < 0 {
		end += n
	}
	if start < 0 {
		start = 0
	}
	if start > n {
		start = n
	}
	if end < 0 {
		end = 0
	}
	if end > n {
		end = n
	}
	if start >= end {
		return ""
	}
	return string(r[start:end])
}

// runeSliceFrom mimics Python s[start:] over code points.
func runeSliceFrom(s string, start int) string {
	r := []rune(s)
	n := len(r)
	if start < 0 {
		start += n
	}
	if start < 0 {
		start = 0
	}
	if start > n {
		start = n
	}
	return string(r[start:])
}

// findRune returns the code-point index of ch in s, or -1.
func findRune(s string, ch rune) int {
	idx := 0
	for _, r := range s {
		if r == ch {
			return idx
		}
		idx++
	}
	return -1
}

// rfindRune returns the last code-point index of ch in s, or -1.
func rfindRune(s string, ch rune) int {
	idx := 0
	last := -1
	for _, r := range s {
		if r == ch {
			last = idx
		}
		idx++
	}
	return last
}

// countNewlines reports how many newline characters s contains.
func countNewlines(s string) int { return strings.Count(s, "\n") }

// joinFrom joins lines[i:] with newlines; out-of-range i yields "".
func joinFrom(lines []string, i int) string {
	if i >= len(lines) {
		return ""
	}
	return strings.Join(lines[i:], "\n")
}

// stripComment removes an inline // comment, keeping // inside double-quoted
// strings, mirroring the Python lexer helper.
func stripComment(s string) string {
	inStr := false
	rs := []rune(s)
	for i, ch := range rs {
		if ch == '"' && (i == 0 || rs[i-1] != '\\') {
			inStr = !inStr
		} else if ch == '/' && !inStr && i+1 < len(rs) && rs[i+1] == '/' {
			return string(rs[:i])
		}
	}
	return s
}

// stripCommentsPerLine applies stripComment to every line of s.
func stripCommentsPerLine(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = stripComment(lines[i])
	}
	return strings.Join(lines, "\n")
}

// findBalanced finds the index of the close rune matching open at start,
// skipping string literals exactly like the Python reference.
func findBalanced(s string, start int, open, close rune) int {
	rs := []rune(s)
	depth := 0
	i := start
	inStr := false
	strCh := '"'
	for i < len(rs) {
		ch := rs[i]
		if inStr {
			if ch == '\\' {
				i += 2
				continue
			}
			if ch == strCh {
				inStr = false
			}
			i++
			continue
		}
		if ch == '"' || ch == '\'' {
			inStr = true
			strCh = ch
			i++
			continue
		}
		if ch == open {
			depth++
		} else if ch == close {
			depth--
			if depth == 0 {
				return i
			}
		}
		i++
	}
	return -1
}

// firstLineTrim returns the first line of s with surrounding whitespace removed.
func firstLineTrim(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return strings.TrimSpace(s[:idx])
	}
	return strings.TrimSpace(s)
}

// dropFirstLine removes the first line and its newline from s.
func dropFirstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[idx+1:]
	}
	return ""
}

// byteToRuneIndex converts a byte offset in s to a code-point offset.
func byteToRuneIndex(s string, byteIdx int) int {
	if byteIdx < 0 {
		return -1
	}
	if byteIdx > len(s) {
		byteIdx = len(s)
	}
	return utf8.RuneCountInString(s[:byteIdx])
}
