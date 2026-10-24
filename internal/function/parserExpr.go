// Expression grammar for .neko function bodies: OR/AND/NOT, comparisons,
// arithmetic, postfix calls/indexes/slices and literals.
package function

import (
	"strings"
)

func (p *parser) parseExpression() (expression, error) { return p.parseOr() }

// parseOr parses "∨"/"or".
func (p *parser) parseOr() (expression, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.matches("∨", "or") {
		p.advance()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &binaryExpression{operator: "∨", left: left, right: right}
	}
	return left, nil
}

// parseAnd parses "∧"/"and".
func (p *parser) parseAnd() (expression, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for p.matches("∧", "and") {
		p.advance()
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		left = &binaryExpression{operator: "∧", left: left, right: right}
	}
	return left, nil
}

// parseNot parses prefix "!"/"not".
func (p *parser) parseNot() (expression, error) {
	if p.matches("!", "not") {
		p.advance()
		operand, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return &unaryExpression{operator: "!", operand: operand}, nil
	}
	return p.parseComparison()
}

// parseComparison parses comparison/membership operators, keeping Python's
// chained-comparison semantics (a < b < c means a < b and b < c).
func (p *parser) parseComparison() (expression, error) {
	left, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}
	operands := []expression{left}
	var operators []string
	for p.matches("==", "!=", "<=", ">=", "<", ">", "in", "contains") {
		operators = append(operators, p.advance().text)
		right, err := p.parseAdditive()
		if err != nil {
			return nil, err
		}
		operands = append(operands, right)
	}
	if len(operators) == 0 {
		return left, nil
	}
	if len(operators) == 1 {
		return &binaryExpression{operator: operators[0], left: operands[0], right: operands[1]}, nil
	}
	return &chainExpression{operands: operands, operators: operators}, nil
}

// parseAdditive parses "+" and "-".
func (p *parser) parseAdditive() (expression, error) {
	left, err := p.parseMultiplicative()
	if err != nil {
		return nil, err
	}
	for p.matches("+", "-") {
		operator := p.advance().text
		right, err := p.parseMultiplicative()
		if err != nil {
			return nil, err
		}
		left = &binaryExpression{operator: operator, left: left, right: right}
	}
	return left, nil
}

// parseMultiplicative parses "*", "/" and "%".
func (p *parser) parseMultiplicative() (expression, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.matches("*", "/", "%") {
		operator := p.advance().text
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &binaryExpression{operator: operator, left: left, right: right}
	}
	return left, nil
}

// parseUnary parses a prefix minus sign.
func (p *parser) parseUnary() (expression, error) {
	if p.matches("-") {
		p.advance()
		operand, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &unaryExpression{operator: "-", operand: operand}, nil
	}
	return p.parsePostfix()
}

// parsePostfix parses member access, indexing/slicing and method calls.
func (p *parser) parsePostfix() (expression, error) {
	target, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for {
		current := p.current()
		if current.kind != TOKEN_SYMBOL {
			return target, nil
		}
		switch current.text {
		case "[":
			indexed, err := p.parseIndex(target)
			if err != nil {
				return nil, err
			}
			target = indexed
		case ".":
			p.advance()
			field, err := p.expectIdentifier()
			if err != nil {
				return nil, err
			}
			if p.current().kind == TOKEN_SYMBOL && p.current().text == "(" {
				args, err := p.parseArguments()
				if err != nil {
					return nil, err
				}
				target = &methodCallExpression{target: target, method: field, args: args}
				continue
			}
			path, ok := target.(*pathExpression)
			if !ok {
				return nil, p.errorf("成员访问只支持路径, 实际 %q", field)
			}
			path.fields = append(path.fields, field)
		default:
			return target, nil
		}
	}
}

// parseIndex parses "[index]", "[low:high]" and "[low:high:step]".
func (p *parser) parseIndex(target expression) (expression, error) {
	p.advance()
	var low, high, step expression
	var err error
	if !p.matches(":") {
		low, err = p.parseExpression()
		if err != nil {
			return nil, err
		}
	}
	if p.matches(":") {
		p.advance()
		if !p.matches("]", ":") {
			high, err = p.parseExpression()
			if err != nil {
				return nil, err
			}
		}
		if p.matches(":") {
			p.advance()
			if !p.matches("]") {
				step, err = p.parseExpression()
				if err != nil {
					return nil, err
				}
			}
		}
		if err := p.expectSymbol("]"); err != nil {
			return nil, err
		}
		return &indexExpression{target: target, low: low, high: high, step: step, slice: true}, nil
	}
	if err := p.expectSymbol("]"); err != nil {
		return nil, err
	}
	return &indexExpression{target: target, low: low}, nil
}

// parseArguments parses "(arg, arg, ...)".
func (p *parser) parseArguments() ([]expression, error) {
	if err := p.expectSymbol("("); err != nil {
		return nil, err
	}
	args := []expression{}
	if p.matches(")") {
		p.advance()
		return args, nil
	}
	for {
		arg, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
		if p.matches(",") {
			p.advance()
			continue
		}
		if err := p.expectSymbol(")"); err != nil {
			return nil, err
		}
		return args, nil
	}
}

// parsePrimary parses literals, names, calls, entity refs, lists and groups.
func (p *parser) parsePrimary() (expression, error) {
	current := p.current()
	switch current.kind {
	case TOKEN_NUMBER:
		p.advance()
		return &literalExpression{value: current.number}, nil
	case TOKEN_TEXT:
		p.advance()
		return &literalExpression{value: current.text}, nil
	case TOKEN_IDENTIFIER:
		p.advance()
		switch current.text {
		case "true", "真":
			return &literalExpression{value: true}, nil
		case "false", "假":
			return &literalExpression{value: false}, nil
		}
		if p.current().kind == TOKEN_SYMBOL && p.current().text == "(" {
			args, err := p.parseArguments()
			if err != nil {
				return nil, err
			}
			return &callExpression{name: current.text, args: args}, nil
		}
		return &pathExpression{root: current.text}, nil
	case TOKEN_SYMBOL:
		switch current.text {
		case "(":
			p.advance()
			inner, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			if err := p.expectSymbol(")"); err != nil {
				return nil, err
			}
			return inner, nil
		case "[":
			return p.parseListLiteral()
		case "{":
			if p.looksLikePlaceholder() {
				p.advance()
				name, err := p.expectIdentifier()
				if err != nil {
					return nil, err
				}
				if name != "e" {
					return nil, p.errorf("只支持 {e} 占位, 实际 {%s}", name)
				}
				if err := p.expectSymbol("}"); err != nil {
					return nil, err
				}
				return &pathExpression{current: true}, nil
			}
			return p.parseMapLiteral()
		}
	}
	return nil, p.errorf("意外的标记 %q", current.text)
}

// parseListLiteral parses "[]", "[a, b]" and "[expr for x in list if cond]".
func (p *parser) parseListLiteral() (expression, error) {
	p.advance()
	items := []expression{}
	if p.matches("]") {
		p.advance()
		return &listExpression{items: items}, nil
	}
	for {
		item, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if p.matches("for", "循环") {
			p.advance()
			variable, err := p.expectIdentifier()
			if err != nil {
				return nil, err
			}
			if !p.matches("in") {
				return nil, p.errorf("列表推导缺少 in")
			}
			p.advance()
			iterable, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			var condition expression
			if p.matches("if") {
				p.advance()
				condition, err = p.parseExpression()
				if err != nil {
					return nil, err
				}
			}
			if err := p.expectSymbol("]"); err != nil {
				return nil, err
			}
			return &listCompExpression{expression: item, variable: variable, iterable: iterable, condition: condition}, nil
		}
		items = append(items, item)
		if p.matches(",") {
			p.advance()
			continue
		}
		if err := p.expectSymbol("]"); err != nil {
			return nil, err
		}
		return &listExpression{items: items}, nil
	}
}

// fullPath joins the root and field segments of a path.
func (path *pathExpression) fullPath() string {
	if path.current {
		return strings.Join(path.fields, ".")
	}
	if len(path.fields) == 0 {
		return path.root
	}
	return path.root + "." + strings.Join(path.fields, ".")
}

// looksLikePlaceholder reports whether the current "{" starts the {e} form.
func (p *parser) looksLikePlaceholder() bool {
	if p.pos+1 >= len(p.tokens) {
		return false
	}
	if p.current().text != "{" || p.tokens[p.pos+1].kind != TOKEN_IDENTIFIER {
		return false
	}
	return p.pos+2 < len(p.tokens) && p.tokens[p.pos+2].text == "}"
}

// parseMapLiteral parses {"键": 值, ...}; keys are text or bare identifiers.
func (p *parser) parseMapLiteral() (expression, error) {
	p.advance()
	keys := []string{}
	values := []expression{}
	for !p.matches("}") {
		keyToken := p.current()
		var key string
		switch keyToken.kind {
		case TOKEN_TEXT, TOKEN_IDENTIFIER:
			key = keyToken.text
			p.advance()
		default:
			return nil, p.errorf("映射键必须是文本或标识符, 实际 %q", keyToken.text)
		}
		if err := p.expectSymbol(":"); err != nil {
			return nil, err
		}
		value, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
		values = append(values, value)
		if p.matches(",") {
			p.advance()
			continue
		}
		break
	}
	if err := p.expectSymbol("}"); err != nil {
		return nil, err
	}
	return &mapExpression{keys: keys, values: values}, nil
}
