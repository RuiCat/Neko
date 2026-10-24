// Tokenizer for .neko function bodies: identifiers, literals, operators and the
// newlines that separate statements.
package function

import (
	"fmt"
	"strconv"
	"unicode"
)

// tokenKind classifies a lexed token.
type tokenKind int

const (
	TOKEN_EOF tokenKind = iota
	TOKEN_NEWLINE
	TOKEN_IDENTIFIER
	TOKEN_NUMBER
	TOKEN_TEXT
	TOKEN_SYMBOL
)

// token is one lexed unit; text holds the source text (or decoded literal), and
// number holds the numeric value for TOKEN_NUMBER.
type token struct {
	kind   tokenKind
	text   string
	number any
	line   int
}

var multiCharacterSymbols = []string{"==", "!=", "<=", ">=", "+=", "-="}

// singleCharacterSymbols is the set of one-rune operators and punctuation.
const singleCharacterSymbols = "+-*/%=<>!()[]{},.:;∧∨"

// lexBody converts .neko function body source into tokens, ending with EOF.
func lexBody(source string) ([]token, error) {
	runes := []rune(source)
	tokens := make([]token, 0, len(runes)/3)
	line := 1
	for i := 0; i < len(runes); {
		ch := runes[i]
		switch {
		case ch == '\n':
			tokens = append(tokens, token{kind: TOKEN_NEWLINE, text: "\n", line: line})
			line++
			i++
		case ch == ' ' || ch == '\t' || ch == '\r':
			i++
		case ch == '/' && i+1 < len(runes) && runes[i+1] == '/':
			for i < len(runes) && runes[i] != '\n' {
				i++
			}
		case ch == '"' || ch == '\'':
			text, next, err := scanStringLiteral(runes, i, line)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token{kind: TOKEN_TEXT, text: text, line: line})
			i = next
		case unicode.IsDigit(ch):
			literal, next, err := scanNumberLiteral(runes, i, line)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, literal)
			i = next
		case isIdentifierStart(ch):
			start := i
			for i < len(runes) && isIdentifierPart(runes[i]) {
				i++
			}
			tokens = append(tokens, token{kind: TOKEN_IDENTIFIER, text: string(runes[start:i]), line: line})
		default:
			symbol, size, ok := matchSymbol(runes[i:])
			if !ok {
				return nil, fmt.Errorf("第 %d 行: 无法识别的字符 %q", line, ch)
			}
			tokens = append(tokens, token{kind: TOKEN_SYMBOL, text: symbol, line: line})
			i += size
		}
	}
	tokens = append(tokens, token{kind: TOKEN_EOF, text: "", line: line})
	return tokens, nil
}

// scanStringLiteral reads a quoted string and returns its decoded content.
func scanStringLiteral(runes []rune, start, line int) (string, int, error) {
	quote := runes[start]
	decoded := make([]rune, 0, 16)
	for i := start + 1; i < len(runes); i++ {
		ch := runes[i]
		if ch == '\n' {
			return "", 0, fmt.Errorf("第 %d 行: 字符串未闭合", line)
		}
		if ch == '\\' && i+1 < len(runes) {
			i++
			switch runes[i] {
			case 'n':
				decoded = append(decoded, '\n')
			case 't':
				decoded = append(decoded, '\t')
			default:
				decoded = append(decoded, runes[i])
			}
			continue
		}
		if ch == quote {
			return string(decoded), i + 1, nil
		}
		decoded = append(decoded, ch)
	}
	return "", 0, fmt.Errorf("第 %d 行: 字符串未闭合", line)
}

// scanNumberLiteral reads an integer or float literal.
func scanNumberLiteral(runes []rune, start, line int) (token, int, error) {
	i := start
	for i < len(runes) && unicode.IsDigit(runes[i]) {
		i++
	}
	if i+1 < len(runes) && runes[i] == '.' && unicode.IsDigit(runes[i+1]) {
		i++
		for i < len(runes) && unicode.IsDigit(runes[i]) {
			i++
		}
		value, err := strconv.ParseFloat(string(runes[start:i]), 64)
		if err != nil {
			return token{}, 0, fmt.Errorf("第 %d 行: 数字格式错误: %v", line, err)
		}
		return token{kind: TOKEN_NUMBER, text: string(runes[start:i]), number: value, line: line}, i, nil
	}
	value, err := strconv.Atoi(string(runes[start:i]))
	if err != nil {
		return token{}, 0, fmt.Errorf("第 %d 行: 数字格式错误: %v", line, err)
	}
	return token{kind: TOKEN_NUMBER, text: string(runes[start:i]), number: value, line: line}, i, nil
}

// matchSymbol picks the longest known operator at the start of runes.
func matchSymbol(runes []rune) (string, int, bool) {
	for _, symbol := range multiCharacterSymbols {
		symbolRunes := []rune(symbol)
		if len(runes) >= len(symbolRunes) && string(runes[:len(symbolRunes)]) == symbol {
			return symbol, len(symbolRunes), true
		}
	}
	if len(runes) > 0 && containsRune(singleCharacterSymbols, runes[0]) {
		return string(runes[0]), 1, true
	}
	return "", 0, false
}

// containsRune reports whether text contains ch.
func containsRune(text string, ch rune) bool {
	for _, candidate := range text {
		if candidate == ch {
			return true
		}
	}
	return false
}

// isIdentifierStart reports whether ch may start an identifier.
func isIdentifierStart(ch rune) bool {
	return unicode.IsLetter(ch) || ch == '_'
}

// isIdentifierPart reports whether ch may continue an identifier.
func isIdentifierPart(ch rune) bool {
	return unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_'
}
