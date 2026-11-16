// parser.go parses the `.neko` data subset into an ordered block/entry tree
// with 1-based line anchors. Only @定义 bodies are parsed; @规则 and other
// directive blocks are skipped by balanced-brace scanning and surfaced as
// skipped blocks, never silently dropped.
package spec

import (
	"fmt"
	"strconv"
	"strings"
)

// ValueKind is the literal shape of a parsed value.
type ValueKind int

const (
	VALUE_STRING ValueKind = iota
	VALUE_NUMBER
	VALUE_BOOL
	VALUE_OBJECT
	VALUE_ARRAY
)

// Value is one parsed literal. Text holds a string or the raw number literal;
// Entries holds object members; Items holds array elements. Line is 1-based.
type Value struct {
	Kind    ValueKind
	Text    string
	Bool    bool
	Entries []Entry
	Items   []Value
	Line    int
}

// Entry is one object member with the line of its key.
type Entry struct {
	Key     string
	KeyLine int
	Value   Value
}

// Block is one top-level @定义 or skipped directive.
type Block struct {
	Directive string
	Name      string
	Line      int
	Entries   []Entry
	Skipped   bool
}

// Document is the parse result for one corpus file.
type Document struct {
	Blocks []Block
	Issues []ParseIssue
}

// ParseDocument parses one corpus file. Parse errors are collected in Issues
// and do not abort the remaining blocks.
func ParseDocument(src []byte) Document {
	s := newScanner(src)
	doc := Document{Blocks: []Block{}, Issues: []ParseIssue{}}
	for {
		s.skipSpaceAndComments()
		if s.eof() {
			break
		}
		if s.peek() != '@' {
			line := s.line
			for !s.eof() && s.peek() != '\n' {
				s.advance()
			}
			doc.Issues = append(doc.Issues, ParseIssue{Line: line, Message: "unexpected top-level text"})
			continue
		}
		line := s.line
		s.advance() // '@'
		directive := s.readLetters()
		if directive == "" {
			doc.Issues = append(doc.Issues, ParseIssue{Line: line, Message: "missing directive name after @"})
			for !s.eof() && s.peek() != '\n' {
				s.advance()
			}
			continue
		}
		if directive != "定义" {
			if !s.skipToBrace() {
				doc.Issues = append(doc.Issues, ParseIssue{Line: line, Message: "directive @" + directive + " has no block"})
				continue
			}
			s.advance() // '{'
			if _, err := s.skipBraceBody(); err != nil {
				doc.Issues = append(doc.Issues, ParseIssue{Line: errLine(err, line), Message: err.Error()})
				break
			}
			doc.Blocks = append(doc.Blocks, Block{Directive: directive, Line: line, Skipped: true})
			continue
		}
		s.skipSpaceAndComments()
		name := s.readName()
		if name == "" {
			doc.Issues = append(doc.Issues, ParseIssue{Line: line, Message: "definition has no name"})
			for !s.eof() && s.peek() != '\n' {
				s.advance()
			}
			continue
		}
		s.skipSpaceAndComments()
		if s.peek() != '{' {
			doc.Issues = append(doc.Issues, ParseIssue{Line: line, Message: "definition " + name + " has no body"})
			for !s.eof() && s.peek() != '\n' {
				s.advance()
			}
			continue
		}
		bodyLine := s.line
		s.advance() // '{'
		bodyStart := s.pos
		bodyEnd, err := s.skipBraceBody()
		if err != nil {
			doc.Issues = append(doc.Issues, ParseIssue{Line: errLine(err, line), Message: err.Error()})
			break
		}
		body := src[bodyStart:bodyEnd]
		entries, err := parseDefinitionBody(body, bodyLine)
		if err != nil {
			doc.Issues = append(doc.Issues, ParseIssue{Line: errLine(err, bodyLine), Message: err.Error()})
		}
		doc.Blocks = append(doc.Blocks, Block{
			Directive: "定义",
			Name:      name,
			Line:      line,
			Entries:   entries,
		})
	}
	return doc
}

func errLine(err error, fallback int) int {
	if err == nil {
		return fallback
	}
	msg := err.Error()
	const prefix = "line "
	if !strings.HasPrefix(msg, prefix) {
		return fallback
	}
	rest := msg[len(prefix):]
	colon := strings.Index(rest, ":")
	if colon <= 0 {
		return fallback
	}
	line, convErr := strconv.Atoi(rest[:colon])
	if convErr != nil || line <= 0 {
		return fallback
	}
	return line
}

func parseDefinitionBody(body []byte, startLine int) ([]Entry, error) {
	p := &bodyParser{s: newScannerAt(body, startLine)}
	entries := []Entry{}
	for {
		p.s.skipSpaceAndComments()
		if p.s.eof() {
			return entries, nil
		}
		tok, err := p.s.next()
		if err != nil {
			return entries, err
		}
		if tok.kind == tokenComma {
			continue
		}
		if tok.kind != tokenString {
			return entries, fmt.Errorf("line %d: expected quoted key", tok.line)
		}
		key := tok.text
		keyLine := tok.line
		sep, err := p.s.next()
		if err != nil {
			return entries, err
		}
		if sep.kind != tokenColon {
			return entries, fmt.Errorf("line %d: expected ':' after key %q", sep.line, key)
		}
		val, err := p.parseValue()
		if err != nil {
			return entries, err
		}
		entries = append(entries, Entry{Key: key, KeyLine: keyLine, Value: val})
	}
}

type bodyParser struct {
	s *scanner
}

func (p *bodyParser) parseValue() (Value, error) {
	tok, err := p.s.next()
	if err != nil {
		return Value{}, err
	}
	switch tok.kind {
	case tokenString:
		return Value{Kind: VALUE_STRING, Text: tok.text, Line: tok.line}, nil
	case tokenNumber:
		return Value{Kind: VALUE_NUMBER, Text: tok.text, Line: tok.line}, nil
	case tokenWord:
		switch tok.text {
		case "true":
			return Value{Kind: VALUE_BOOL, Bool: true, Line: tok.line}, nil
		case "false":
			return Value{Kind: VALUE_BOOL, Bool: false, Line: tok.line}, nil
		}
		return Value{}, fmt.Errorf("line %d: unexpected word %q", tok.line, tok.text)
	case tokenLBrace:
		entries, err := p.parseObject()
		if err != nil {
			return Value{}, err
		}
		return Value{Kind: VALUE_OBJECT, Entries: entries, Line: tok.line}, nil
	case tokenLBracket:
		items, err := p.parseArray()
		if err != nil {
			return Value{}, err
		}
		return Value{Kind: VALUE_ARRAY, Items: items, Line: tok.line}, nil
	}
	return Value{}, fmt.Errorf("line %d: unexpected token %q", tok.line, tok.text)
}

func (p *bodyParser) parseObject() ([]Entry, error) {
	entries := []Entry{}
	for {
		tok, err := p.s.next()
		if err != nil {
			return entries, err
		}
		switch tok.kind {
		case tokenRBrace:
			return entries, nil
		case tokenComma:
			continue
		case tokenString:
			key := tok.text
			keyLine := tok.line
			sep, err := p.s.next()
			if err != nil {
				return entries, err
			}
			if sep.kind != tokenColon {
				return entries, fmt.Errorf("line %d: expected ':' after key %q", sep.line, key)
			}
			val, err := p.parseValue()
			if err != nil {
				return entries, err
			}
			entries = append(entries, Entry{Key: key, KeyLine: keyLine, Value: val})
		default:
			return entries, fmt.Errorf("line %d: expected object key or '}'", tok.line)
		}
	}
}

func (p *bodyParser) parseArray() ([]Value, error) {
	items := []Value{}
	for {
		tok, err := p.s.next()
		if err != nil {
			return items, err
		}
		switch tok.kind {
		case tokenRBracket:
			return items, nil
		case tokenComma:
			continue
		case tokenString:
			items = append(items, Value{Kind: VALUE_STRING, Text: tok.text, Line: tok.line})
		case tokenNumber:
			items = append(items, Value{Kind: VALUE_NUMBER, Text: tok.text, Line: tok.line})
		case tokenWord:
			switch tok.text {
			case "true":
				items = append(items, Value{Kind: VALUE_BOOL, Bool: true, Line: tok.line})
			case "false":
				items = append(items, Value{Kind: VALUE_BOOL, Bool: false, Line: tok.line})
			default:
				return items, fmt.Errorf("line %d: unexpected word %q", tok.line, tok.text)
			}
		case tokenLBrace:
			entries, err := p.parseObject()
			if err != nil {
				return items, err
			}
			items = append(items, Value{Kind: VALUE_OBJECT, Entries: entries, Line: tok.line})
		case tokenLBracket:
			nested, err := p.parseArray()
			if err != nil {
				return items, err
			}
			items = append(items, Value{Kind: VALUE_ARRAY, Items: nested, Line: tok.line})
		default:
			return items, fmt.Errorf("line %d: unexpected token in array", tok.line)
		}
	}
}
