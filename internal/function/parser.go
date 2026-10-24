// Recursive-descent parser for the .neko function body subset: blocks, control
// flow, assignment and the expression grammar used by the corpus functions.
package function

import (
	"fmt"
)

// parser walks the token stream produced by lexBody.
type parser struct {
	tokens []token
	pos    int
}

// parseBody parses a complete function body into executable statements.
func parseBody(source string) ([]statement, error) {
	tokens, err := lexBody(source)
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: tokens}
	body, err := p.parseStatements(false)
	if err != nil {
		return nil, err
	}
	if p.current().kind != TOKEN_EOF {
		return nil, p.errorf("多余的内容: %q", p.current().text)
	}
	return body, nil
}

// current returns the token under the cursor.
func (p *parser) current() token { return p.tokens[p.pos] }

// advance consumes and returns the current token.
func (p *parser) advance() token {
	current := p.current()
	if p.pos < len(p.tokens)-1 {
		p.pos++
	}
	return current
}

// skipNewlines consumes statement separators.
func (p *parser) skipNewlines() {
	for p.current().kind == TOKEN_NEWLINE || p.current().text == ";" {
		p.advance()
	}
}

// matches reports whether the current token text is one of texts.
func (p *parser) matches(texts ...string) bool {
	current := p.current().text
	for _, text := range texts {
		if current == text {
			return true
		}
	}
	return false
}

// expectSymbol consumes a required punctuation token.
func (p *parser) expectSymbol(symbol string) error {
	if p.current().kind != TOKEN_SYMBOL || p.current().text != symbol {
		return p.errorf("期望 %q, 实际 %q", symbol, p.current().text)
	}
	p.advance()
	return nil
}

// expectIdentifier consumes a required identifier token and returns its name.
func (p *parser) expectIdentifier() (string, error) {
	if p.current().kind != TOKEN_IDENTIFIER {
		return "", p.errorf("期望标识符, 实际 %q", p.current().text)
	}
	return p.advance().text, nil
}

// errorf builds a positioned parse error.
func (p *parser) errorf(format string, args ...any) error {
	return fmt.Errorf("第 %d 行: %s", p.current().line, fmt.Sprintf(format, args...))
}

// parseStatements parses statements until EOF or, when untilBrace is set, the
// closing brace of a block (which it consumes).
func (p *parser) parseStatements(untilBrace bool) ([]statement, error) {
	statements := []statement{}
	for {
		p.skipNewlines()
		if p.current().kind == TOKEN_EOF {
			if untilBrace {
				return nil, p.errorf("代码块缺少闭合的 }")
			}
			return statements, nil
		}
		if p.current().kind == TOKEN_SYMBOL && p.current().text == "}" {
			if !untilBrace {
				return nil, p.errorf("多余的 }")
			}
			p.advance()
			return statements, nil
		}
		statement, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		statements = append(statements, statement)
	}
}

// parseStatement parses one statement, including assignment and control flow.
func (p *parser) parseStatement() (statement, error) {
	if p.current().kind == TOKEN_IDENTIFIER {
		switch p.current().text {
		case "if", "如果":
			return p.parseIf()
		case "return", "返回":
			return p.parseReturn()
		case "for", "循环":
			return p.parseFor()
		case "while", "当":
			return p.parseWhile()
		case "try", "尝试":
			return p.parseTry()
		case "break", "跳出":
			return p.parseBreak()
		case "continue":
			return p.parseContinue()
		case "else", "否则", "否则如果":
			return nil, p.errorf("意外的 %q", p.current().text)
		}
	}
	value, err := p.parseExpression()
	if err != nil {
		return nil, err
	}
	if p.matches(",") {
		targets := []expression{value}
		for p.matches(",") {
			p.advance()
			next, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			targets = append(targets, next)
		}
		if !p.matches("=") {
			return nil, p.errorf("多目标赋值缺少 =")
		}
		p.advance()
		right, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		values := []expression{right}
		for p.matches(",") {
			p.advance()
			next, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			values = append(values, next)
		}
		if len(values) > 1 {
			right = &listExpression{items: values}
		}
		return &multiAssignStatement{targets: targets, value: right}, nil
	}
	if p.current().kind == TOKEN_SYMBOL && p.matches("=", "+=", "-=") {
		operator := p.advance().text
		right, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		return &assignStatement{target: value, value: right, operator: operator}, nil
	}
	return &exprStatement{value: value}, nil
}

// parseReturn parses 返回/return with an optional value.
func (p *parser) parseReturn() (statement, error) {
	p.advance()
	if p.current().kind == TOKEN_EOF || p.current().kind == TOKEN_NEWLINE || p.matches("}") {
		return &returnStatement{}, nil
	}
	value, err := p.parseExpression()
	if err != nil {
		return nil, err
	}
	return &returnStatement{value: value}, nil
}

// parseIf parses an if / else if / else chain.
func (p *parser) parseIf() (statement, error) {
	p.advance()
	condition, err := p.parseExpression()
	if err != nil {
		return nil, err
	}
	then, err := p.parseBraceBlock()
	if err != nil {
		return nil, err
	}
	elseBlock, err := p.parseElseChain()
	if err != nil {
		return nil, err
	}
	return &ifStatement{condition: condition, then: then, elseBlock: elseBlock}, nil
}

// parseElseChain parses an optional else / else if continuation.
func (p *parser) parseElseChain() ([]statement, error) {
	p.skipNewlines()
	if p.current().kind != TOKEN_IDENTIFIER {
		return nil, nil
	}
	switch p.current().text {
	case "else", "否则":
		p.advance()
		p.skipNewlines()
		if p.current().kind == TOKEN_IDENTIFIER && p.matches("if", "如果") {
			nested, err := p.parseIf()
			if err != nil {
				return nil, err
			}
			return []statement{nested}, nil
		}
		return p.parseBraceBlock()
	case "否则如果":
		p.advance()
		condition, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		then, err := p.parseBraceBlock()
		if err != nil {
			return nil, err
		}
		rest, err := p.parseElseChain()
		if err != nil {
			return nil, err
		}
		return []statement{&ifStatement{condition: condition, then: then, elseBlock: rest}}, nil
	}
	return nil, nil
}

// parseBraceBlock parses "{ ... }" as a statement list.
func (p *parser) parseBraceBlock() ([]statement, error) {
	p.skipNewlines()
	if err := p.expectSymbol("{"); err != nil {
		return nil, err
	}
	return p.parseStatements(true)
}

// parseFor parses "for 项 in 列表 { ... }".
func (p *parser) parseFor() (statement, error) {
	p.advance()
	variable, err := p.expectIdentifier()
	if err != nil {
		return nil, err
	}
	variables := []string{variable}
	for p.matches(",") {
		p.advance()
		next, err := p.expectIdentifier()
		if err != nil {
			return nil, err
		}
		variables = append(variables, next)
	}
	if !p.matches("in") {
		return nil, p.errorf("for 语句缺少 in")
	}
	p.advance()
	iterable, err := p.parseExpression()
	if err != nil {
		return nil, err
	}
	body, err := p.parseBraceBlock()
	if err != nil {
		return nil, err
	}
	return &forStatement{variables: variables, iterable: iterable, body: body}, nil
}

// parseExpression parses the lowest-precedence expression.
// parseTry parses "try { ... } [except [类型] [as 变量] { ... }] [finally { ... }]".
func (p *parser) parseTry() (statement, error) {
	p.advance()
	body, err := p.parseBraceBlock()
	if err != nil {
		return nil, err
	}
	var handler, finalizer []statement
	errorName := "错误"
	exceptType := ""
	if p.matches("except", "catch", "捕获") {
		p.advance()
		if p.current().kind == TOKEN_IDENTIFIER && !p.matches("{") {
			first := p.current().text
			p.advance()
			if p.matches("as") {
				p.advance()
				exceptType = first
				name, nameErr := p.expectIdentifier()
				if nameErr != nil {
					return nil, nameErr
				}
				errorName = name
			} else {
				errorName = first
			}
		}
		handler, err = p.parseBraceBlock()
		if err != nil {
			return nil, err
		}
	}
	if p.matches("finally", "最终") {
		p.advance()
		finalizer, err = p.parseBraceBlock()
		if err != nil {
			return nil, err
		}
	}
	if handler == nil && finalizer == nil {
		return nil, p.errorf("try 需要 except/catch 或 finally")
	}
	return &tryStatement{body: body, handler: handler, finalizer: finalizer, errorName: errorName, exceptType: exceptType}, nil
}

// parseBreak parses "break".
func (p *parser) parseBreak() (statement, error) {
	p.advance()
	return &breakStatement{}, nil
}

// parseContinue parses "continue".
func (p *parser) parseContinue() (statement, error) {
	p.advance()
	return &continueStatement{}, nil
}

// parseWhile parses "while 条件 { ... }".
func (p *parser) parseWhile() (statement, error) {
	p.advance()
	condition, err := p.parseExpression()
	if err != nil {
		return nil, err
	}
	body, err := p.parseBraceBlock()
	if err != nil {
		return nil, err
	}
	return &whileStatement{condition: condition, body: body}, nil
}
