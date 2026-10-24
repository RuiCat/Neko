// Arithmetic evaluation for condition operands such as "变化量 + 1".
package engine

import (
	"strconv"
	"strings"
)

// looksArithmetic reports whether an operand may contain arithmetic operators.
func looksArithmetic(expr string) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return false
	}
	if strings.ContainsAny(expr, "+*/%()") {
		return true
	}
	// A '-' inside the expression (not a leading sign of a number) is arithmetic.
	if index := strings.IndexByte(expr, '-'); index > 0 && expr[index-1] != 'e' && expr[index-1] != 'E' {
		return true
	}
	return strings.ContainsAny(expr, " \t")
}

// evalArithmetic parses a numeric expression, resolving identifiers through the
// value resolver. ok is false when the expression is not valid arithmetic.
func evalArithmetic(expr string, resolver ValueResolver) (any, bool) {
	parser := &arithParser{runes: []rune(expr), resolver: resolver}
	value, ok := parser.parseExpression()
	if !ok {
		return nil, false
	}
	parser.skipSpaces()
	if parser.pos != len(parser.runes) {
		return nil, false
	}
	return value, true
}

type arithParser struct {
	runes    []rune
	pos      int
	resolver ValueResolver
}

func (p *arithParser) skipSpaces() {
	for p.pos < len(p.runes) && (p.runes[p.pos] == ' ' || p.runes[p.pos] == '\t') {
		p.pos++
	}
}

func (p *arithParser) peek() rune {
	p.skipSpaces()
	if p.pos >= len(p.runes) {
		return 0
	}
	return p.runes[p.pos]
}

func (p *arithParser) parseExpression() (any, bool) {
	left, ok := p.parseTerm()
	if !ok {
		return nil, false
	}
	for {
		switch p.peek() {
		case '+', '-':
			operator := p.runes[p.pos]
			p.pos++
			right, ok := p.parseTerm()
			if !ok {
				return nil, false
			}
			value, ok := arithApply(string(operator), left, right)
			if !ok {
				return nil, false
			}
			left = value
		default:
			return left, true
		}
	}
}

func (p *arithParser) parseTerm() (any, bool) {
	left, ok := p.parseUnary()
	if !ok {
		return nil, false
	}
	for {
		switch p.peek() {
		case '*', '/', '%':
			operator := p.runes[p.pos]
			p.pos++
			right, ok := p.parseUnary()
			if !ok {
				return nil, false
			}
			value, ok := arithApply(string(operator), left, right)
			if !ok {
				return nil, false
			}
			left = value
		default:
			return left, true
		}
	}
}

func (p *arithParser) parseUnary() (any, bool) {
	if p.peek() == '-' {
		p.pos++
		value, ok := p.parseUnary()
		if !ok {
			return nil, false
		}
		number, ok := arithNumber(value)
		if !ok {
			return nil, false
		}
		return -number, true
	}
	return p.parsePrimary()
}

func (p *arithParser) parsePrimary() (any, bool) {
	ch := p.peek()
	if ch == '(' {
		p.pos++
		value, ok := p.parseExpression()
		if !ok {
			return nil, false
		}
		if p.peek() != ')' {
			return nil, false
		}
		p.pos++
		return value, true
	}
	if ch == '"' || ch == '\'' {
		return p.parseString(ch)
	}
	if ch == 0 {
		return nil, false
	}
	start := p.pos
	for p.pos < len(p.runes) {
		ch := p.runes[p.pos]
		if ch == ' ' || ch == '\t' || ch == '+' || ch == '-' || ch == '*' || ch == '/' || ch == '%' || ch == '(' || ch == ')' {
			break
		}
		p.pos++
	}
	atom := string(p.runes[start:p.pos])
	if atom == "" {
		return nil, false
	}
	if number, err := strconv.ParseFloat(atom, 64); err == nil {
		return number, true
	}
	if p.resolver == nil {
		return nil, false
	}
	value, err := p.resolver(atom)
	if err != nil {
		return nil, false
	}
	return value, true
}

func (p *arithParser) parseString(quote rune) (any, bool) {
	p.pos++
	var builder strings.Builder
	for p.pos < len(p.runes) {
		ch := p.runes[p.pos]
		p.pos++
		if ch == quote {
			return builder.String(), true
		}
		builder.WriteRune(ch)
	}
	return nil, false
}

func arithNumber(value any) (float64, bool) {
	number, ok := toFloatValue(value)
	return number, ok
}

func arithApply(operator string, left, right any) (any, bool) {
	leftNumber, leftOK := arithNumber(left)
	rightNumber, rightOK := arithNumber(right)
	if !leftOK || !rightOK {
		return nil, false
	}
	switch operator {
	case "+":
		return leftNumber + rightNumber, true
	case "-":
		return leftNumber - rightNumber, true
	case "*":
		return leftNumber * rightNumber, true
	case "/":
		if rightNumber == 0 {
			return nil, false
		}
		return leftNumber / rightNumber, true
	case "%":
		if rightNumber == 0 {
			return nil, false
		}
		return float64(int64(leftNumber) % int64(rightNumber)), true
	}
	return nil, false
}
