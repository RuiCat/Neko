// Text methods for .neko function bodies (Python str API subset).
package function

import (
	"strings"
	"unicode"
)

// methodTextCall dispatches string methods; ok=false means "not a text method".
func methodTextCall(name string, target any, args []any) (any, bool, error) {
	text, ok := target.(string)
	if !ok {
		return nil, false, nil
	}
	switch name {
	case "join":
		return methodJoin(text, argumentAt(args, 0)), true, nil
	case "split":
		return methodSplit(text, argumentAt(args, 0)), true, nil
	case "rsplit":
		return methodRSplit(text, argumentAt(args, 0), argumentAt(args, 1)), true, nil
	case "splitlines":
		return methodSplitLines(text), true, nil
	case "strip":
		return strings.TrimSpace(text), true, nil
	case "lstrip":
		return strings.TrimLeft(text, " \t\r\n"), true, nil
	case "rstrip":
		return strings.TrimRight(text, " \t\r\n"), true, nil
	case "replace":
		return strings.ReplaceAll(text, pyText(argumentAt(args, 0)), pyText(argumentAt(args, 1))), true, nil
	case "lower":
		return strings.ToLower(text), true, nil
	case "upper":
		return strings.ToUpper(text), true, nil
	case "swapcase":
		return methodSwapCase(text), true, nil
	case "title":
		return methodTitle(text), true, nil
	case "capitalize":
		return methodCapitalize(text), true, nil
	case "startswith":
		return strings.HasPrefix(text, pyText(argumentAt(args, 0))), true, nil
	case "endswith":
		return strings.HasSuffix(text, pyText(argumentAt(args, 0))), true, nil
	case "find":
		return methodFind(text, argumentAt(args, 0), false), true, nil
	case "rfind":
		return methodFind(text, argumentAt(args, 0), true), true, nil
	case "index":
		index := methodFind(text, argumentAt(args, 0), false)
		if index == -1 {
			return nil, true, newScriptError("value", "子串不存在")
		}
		return index, true, nil
	case "rindex":
		index := methodFind(text, argumentAt(args, 0), true)
		if index == -1 {
			return nil, true, newScriptError("value", "子串不存在")
		}
		return index, true, nil
	case "count":
		needle := pyText(argumentAt(args, 0))
		if needle == "" {
			return len([]rune(text)) + 1, true, nil
		}
		return strings.Count(text, needle), true, nil
	case "ljust":
		return methodJustify(text, argumentAt(args, 0), argumentAt(args, 1), 0)
	case "rjust":
		return methodJustify(text, argumentAt(args, 0), argumentAt(args, 1), 1)
	case "center":
		return methodJustify(text, argumentAt(args, 0), argumentAt(args, 1), 2)
	case "zfill":
		return methodZFill(text, argumentAt(args, 0)), true, nil
	case "partition":
		return methodPartition(text, argumentAt(args, 0), false)
	case "rpartition":
		return methodPartition(text, argumentAt(args, 0), true)
	case "removeprefix":
		return strings.TrimPrefix(text, pyText(argumentAt(args, 0))), true, nil
	case "removesuffix":
		return strings.TrimSuffix(text, pyText(argumentAt(args, 0))), true, nil
	case "isdigit":
		return methodIsKind(text, "digit"), true, nil
	case "isdecimal":
		return methodIsKind(text, "decimal"), true, nil
	case "isalpha":
		return methodIsKind(text, "alpha"), true, nil
	case "isnumeric":
		return methodIsKind(text, "numeric"), true, nil
	case "isalnum":
		return methodIsKind(text, "alnum"), true, nil
	case "isspace":
		return methodIsKind(text, "space"), true, nil
	case "islower":
		return methodCasedCase(text, false), true, nil
	case "isupper":
		return methodCasedCase(text, true), true, nil
	case "casefold":
		return strings.ToLower(text), true, nil
	case "isascii":
		return methodIsASCII(text), true, nil
	case "isprintable":
		return methodIsPrintable(text), true, nil
	case "istitle":
		return methodIsTitle(text), true, nil
	case "expandtabs":
		return methodExpandTabs(text, argumentAt(args, 0)), true, nil
	case "format":
		return methodFormat(text, args), true, nil
	}
	return nil, false, nil
}

func methodJoin(separator string, target any) any {
	items, ok := target.([]any)
	if !ok {
		return pyText(target)
	}
	parts := make([]string, len(items))
	for index, item := range items {
		parts[index] = pyText(item)
	}
	return strings.Join(parts, separator)
}

func methodSplit(text string, separator any) any {
	sep := pyText(separator)
	var parts []string
	if sep == "" {
		parts = strings.Fields(text)
	} else {
		parts = strings.Split(text, sep)
	}
	return stringsToAny(parts)
}

func methodRSplit(text string, separator, maxSplit any) any {
	sep := pyText(separator)
	limit := -1
	if maxSplit != nil {
		limit, _ = toIntValue(maxSplit)
	}
	if sep == "" || limit <= 0 {
		return methodSplit(text, separator)
	}
	parts := strings.Split(text, sep)
	if len(parts) <= limit+1 {
		return stringsToAny(parts)
	}
	head := strings.Join(parts[:len(parts)-limit], sep)
	result := []string{head}
	result = append(result, parts[len(parts)-limit:]...)
	return stringsToAny(result)
}

func methodSplitLines(text string) any {
	lines := strings.FieldsFunc(text, func(ch rune) bool { return ch == '\n' || ch == '\r' })
	return stringsToAny(lines)
}

func methodSwapCase(text string) string {
	runes := []rune(text)
	for index, ch := range runes {
		if unicode.IsUpper(ch) {
			runes[index] = unicode.ToLower(ch)
		} else if unicode.IsLower(ch) {
			runes[index] = unicode.ToUpper(ch)
		}
	}
	return string(runes)
}

func methodTitle(text string) string {
	runes := []rune(strings.ToLower(text))
	previousLetter := false
	for index, ch := range runes {
		if unicode.IsLetter(ch) {
			if !previousLetter {
				runes[index] = unicode.ToUpper(ch)
			}
			previousLetter = true
			continue
		}
		previousLetter = false
	}
	return string(runes)
}

func methodCapitalize(text string) string {
	runes := []rune(strings.ToLower(text))
	if len(runes) > 0 {
		runes[0] = unicode.ToUpper(runes[0])
	}
	return string(runes)
}

func methodFind(text string, needle any, last bool) any {
	search := pyText(needle)
	index := strings.Index(text, search)
	if last {
		index = strings.LastIndex(text, search)
	}
	if index < 0 {
		return -1
	}
	return len([]rune(text[:index]))
}

func methodJustify(text string, width, fill any, mode int) (any, bool, error) {
	runes := []rune(text)
	widthValue, _ := toIntValue(width)
	if widthValue <= len(runes) {
		return text, true, nil
	}
	fillRunes := []rune(pyText(fill))
	if len(fillRunes) != 1 {
		return nil, true, newScriptError("value", "填充字符必须是单个字符")
	}
	fillRune := fillRunes[0]
	padding := widthValue - len(runes)
	switch mode {
	case 0:
		return text + strings.Repeat(string(fillRune), padding), true, nil
	case 1:
		return strings.Repeat(string(fillRune), padding) + text, true, nil
	}
	left := padding / 2
	right := padding - left
	return strings.Repeat(string(fillRune), left) + text + strings.Repeat(string(fillRune), right), true, nil
}

func methodZFill(text string, width any) any {
	widthValue, _ := toIntValue(width)
	runes := []rune(text)
	if widthValue <= len(runes) {
		return text
	}
	padding := strings.Repeat("0", widthValue-len(runes))
	if len(runes) > 0 && (runes[0] == '-' || runes[0] == '+') {
		return string(runes[0]) + padding + string(runes[1:])
	}
	return padding + text
}

func methodPartition(text string, separator any, last bool) (any, bool, error) {
	sep := pyText(separator)
	if sep == "" {
		return nil, true, newScriptError("value", "分隔符不能为空")
	}
	index := strings.Index(text, sep)
	if last {
		index = strings.LastIndex(text, sep)
	}
	if index < 0 {
		if last {
			return []any{"", "", text}, true, nil
		}
		return []any{text, "", ""}, true, nil
	}
	return []any{text[:index], sep, text[index+len(sep):]}, true, nil
}

func methodIsKind(text, kind string) bool {
	if text == "" {
		return false
	}
	for _, ch := range text {
		switch kind {
		case "digit":
			if ch < '0' || ch > '9' {
				return false
			}
		case "decimal":
			if !unicode.IsDigit(ch) {
				return false
			}
		case "alpha":
			if !unicode.IsLetter(ch) {
				return false
			}
		case "numeric":
			if !unicode.IsNumber(ch) {
				return false
			}
		case "alnum":
			if !unicode.IsLetter(ch) && !unicode.IsNumber(ch) {
				return false
			}
		case "space":
			if !unicode.IsSpace(ch) {
				return false
			}
		}
	}
	return true
}

func methodCasedCase(text string, upper bool) bool {
	hasCased := false
	for _, ch := range text {
		if unicode.IsUpper(ch) || unicode.IsLower(ch) {
			hasCased = true
			if upper && unicode.IsLower(ch) {
				return false
			}
			if !upper && unicode.IsUpper(ch) {
				return false
			}
		}
	}
	return hasCased
}

func stringsToAny(values []string) []any {
	items := make([]any, len(values))
	for index, value := range values {
		items[index] = value
	}
	return items
}

func methodIsASCII(text string) bool {
	for _, ch := range text {
		if ch > 127 {
			return false
		}
	}
	return true
}

func methodIsPrintable(text string) bool {
	for _, ch := range text {
		if !unicode.IsPrint(ch) && !unicode.IsSpace(ch) {
			return false
		}
	}
	return true
}

func methodIsTitle(text string) bool {
	if text == "" {
		return false
	}
	hasCased := false
	previousLetter := false
	for _, ch := range text {
		if unicode.IsLetter(ch) {
			if previousLetter && unicode.IsUpper(ch) {
				return false
			}
			if !previousLetter && unicode.IsLower(ch) {
				return false
			}
			hasCased = true
			previousLetter = true
			continue
		}
		previousLetter = false
	}
	return hasCased
}

func methodExpandTabs(text string, tabSize any) string {
	size, ok := toIntValue(tabSize)
	if !ok || size <= 0 {
		size = 8
	}
	var builder strings.Builder
	column := 0
	for _, ch := range text {
		if ch == '\t' {
			padding := size - (column % size)
			builder.WriteString(strings.Repeat(" ", padding))
			column += padding
			continue
		}
		builder.WriteRune(ch)
		if ch == '\n' || ch == '\r' {
			column = 0
		} else {
			column++
		}
	}
	return builder.String()
}
