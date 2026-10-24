// format() implementation for text values.
package function

import (
	"strconv"
	"strings"
)

// methodFormat implements positional str.format: {} / {0} / {0:.2f} /
// {!r} / {:>10} / {{ }} escaping.
func methodFormat(text string, args []any) any {
	var builder strings.Builder
	runes := []rune(text)
	auto := 0
	for index := 0; index < len(runes); index++ {
		ch := runes[index]
		if ch == '{' {
			if index+1 < len(runes) && runes[index+1] == '{' {
				builder.WriteRune('{')
				index++
				continue
			}
			endIndex := index + 1
			for endIndex < len(runes) && runes[endIndex] != '}' {
				endIndex++
			}
			if endIndex >= len(runes) {
				builder.WriteRune(ch)
				continue
			}
			field := string(runes[index+1 : endIndex])
			index = endIndex
			builder.WriteString(formatField(field, args, &auto))
			continue
		}
		if ch == '}' && index+1 < len(runes) && runes[index+1] == '}' {
			builder.WriteRune('}')
			index++
			continue
		}
		builder.WriteRune(ch)
	}
	return builder.String()
}

// formatField resolves one {field} placeholder.
func formatField(field string, args []any, auto *int) string {
	position := -1
	conversion := ""
	spec := ""
	if colon := strings.IndexByte(field, ':'); colon >= 0 {
		spec = field[colon+1:]
		field = field[:colon]
	}
	if bang := strings.IndexByte(field, '!'); bang >= 0 {
		conversion = field[bang+1:]
		field = field[:bang]
	}
	field = strings.TrimSpace(field)
	if field == "" {
		position = *auto
		*auto++
	} else if parsed, err := strconv.Atoi(field); err == nil {
		position = parsed
	} else {
		return "{" + field + "}"
	}
	value := argumentAt(args, position)
	if conversion == "r" {
		if text, ok := value.(string); ok {
			value = pyReprString(text)
		}
	}
	return formatValue(value, spec)
}

func pyReprString(text string) string {
	if strings.Contains(text, "'") && !strings.Contains(text, "\"") {
		return `"` + text + `"`
	}
	return "'" + strings.ReplaceAll(text, "'", "\\'") + "'"
}

// formatValue applies [[fill]align][0][width][.precision][type] to a value.
func formatValue(value any, spec string) string {
	if spec == "" {
		return pyText(value)
	}
	fill := rune(' ')
	align := rune(0)
	runes := []rune(spec)
	if len(runes) >= 2 && (runes[1] == '<' || runes[1] == '>' || runes[1] == '^') {
		fill = runes[0]
		align = runes[1]
		runes = runes[2:]
	} else if len(runes) >= 1 && (runes[0] == '<' || runes[0] == '>' || runes[0] == '^') {
		align = runes[0]
		runes = runes[1:]
	}
	body := string(runes)
	typeChar := byte(0)
	if len(body) > 0 {
		last := body[len(body)-1]
		if last == 'd' || last == 'f' {
			typeChar = last
			body = body[:len(body)-1]
		}
	}
	precision := -1
	if dot := strings.IndexByte(body, '.'); dot >= 0 {
		if parsed, err := strconv.Atoi(body[dot+1:]); err == nil {
			precision = parsed
		}
		body = body[:dot]
	}
	zero := strings.HasPrefix(body, "0")
	body = strings.TrimPrefix(body, "0")
	width := 0
	if parsed, err := strconv.Atoi(body); err == nil {
		width = parsed
	}
	var text string
	switch typeChar {
	case 'd':
		number, _ := toIntValue(value)
		text = strconv.Itoa(number)
	case 'f':
		number, _ := toNumber(value)
		digits := 6
		if precision >= 0 {
			digits = precision
		}
		text = strconv.FormatFloat(number, 'f', digits, 64)
	default:
		text = pyText(value)
		if precision >= 0 {
			textRunes := []rune(text)
			if precision < len(textRunes) {
				text = string(textRunes[:precision])
			}
		}
	}
	if align == 0 {
		if zero {
			align = '>'
			fill = '0'
		} else if typeChar == 'd' || typeChar == 'f' || isNumericText(text) {
			align = '>'
		} else {
			align = '<'
		}
	}
	padding := width - len([]rune(text))
	if padding <= 0 {
		return text
	}
	switch align {
	case '>':
		return strings.Repeat(string(fill), padding) + text
	case '^':
		left := padding / 2
		return strings.Repeat(string(fill), left) + text + strings.Repeat(string(fill), padding-left)
	}
	return text + strings.Repeat(string(fill), padding)
}

func isNumericText(text string) bool {
	if text == "" {
		return false
	}
	if text[0] == '-' || text[0] == '+' {
		text = text[1:]
	}
	_, err := strconv.ParseFloat(text, 64)
	return err == nil
}
