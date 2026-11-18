// scanner.go provides the byte/rune scanner used by the corpus parser. It
// understands // comments, quoted strings, numbers, booleans, punctuation and
// CJK identifiers, and can skip a balanced brace block without parsing it so
// legacy @规则 bodies never confuse the data parser.
package extract

import (
	"fmt"
	"strconv"
	"unicode"
	"unicode/utf8"
)

type tokenKind int

const (
	tokenEOF tokenKind = iota
	tokenLBrace
	tokenRBrace
	tokenLBracket
	tokenRBracket
	tokenColon
	tokenComma
	tokenString
	tokenNumber
	tokenWord
	tokenOther
)

type token struct {
	kind tokenKind
	text string
	line int
}

type scanner struct {
	src  []byte
	pos  int
	line int
}

func newScanner(src []byte) *scanner {
	return &scanner{src: src, line: 1}
}

func newScannerAt(src []byte, line int) *scanner {
	return &scanner{src: src, line: line}
}

func (s *scanner) eof() bool {
	return s.pos >= len(s.src)
}

func (s *scanner) peek() byte {
	if s.eof() {
		return 0
	}
	return s.src[s.pos]
}

func (s *scanner) peekByte(offset int) byte {
	if s.pos+offset >= len(s.src) {
		return 0
	}
	return s.src[s.pos+offset]
}

func (s *scanner) advance() byte {
	b := s.src[s.pos]
	s.pos++
	if b == '\n' {
		s.line++
	}
	return b
}

func (s *scanner) advanceRune(size int) {
	for i := 0; i < size; i++ {
		s.advance()
	}
}

// skipSpaceAndComments consumes whitespace and // line comments.
func (s *scanner) skipSpaceAndComments() {
	for !s.eof() {
		b := s.peek()
		if b == ' ' || b == '\t' || b == '\r' || b == '\n' {
			s.advance()
			continue
		}
		if b == '/' && s.peekByte(1) == '/' {
			for !s.eof() && s.peek() != '\n' {
				s.advance()
			}
			continue
		}
		return
	}
}

func (s *scanner) next() (token, error) {
	s.skipSpaceAndComments()
	if s.eof() {
		return token{kind: tokenEOF, line: s.line}, nil
	}
	line := s.line
	b := s.peek()
	switch b {
	case '{':
		s.advance()
		return token{kind: tokenLBrace, text: "{", line: line}, nil
	case '}':
		s.advance()
		return token{kind: tokenRBrace, text: "}", line: line}, nil
	case '[':
		s.advance()
		return token{kind: tokenLBracket, text: "[", line: line}, nil
	case ']':
		s.advance()
		return token{kind: tokenRBracket, text: "]", line: line}, nil
	case ':':
		s.advance()
		return token{kind: tokenColon, text: ":", line: line}, nil
	case ',':
		s.advance()
		return token{kind: tokenComma, text: ",", line: line}, nil
	case '"':
		return s.readString()
	}
	if b == '-' || (b >= '0' && b <= '9') {
		return s.readNumber()
	}
	if isWordRune(b) {
		return s.readWord()
	}
	s.advance()
	return token{kind: tokenOther, text: string(b), line: line}, nil
}

func (s *scanner) readString() (token, error) {
	line := s.line
	s.advance() // opening quote
	var out []rune
	for !s.eof() {
		r, size := utf8.DecodeRune(s.src[s.pos:])
		switch r {
		case '"':
			s.advanceRune(size)
			return token{kind: tokenString, text: string(out), line: line}, nil
		case '\\':
			s.advanceRune(size)
			if s.eof() {
				return token{}, fmt.Errorf("line %d: unfinished escape", s.line)
			}
			esc, escSize := utf8.DecodeRune(s.src[s.pos:])
			s.advanceRune(escSize)
			switch esc {
			case 'n':
				out = append(out, '\n')
			case 't':
				out = append(out, '\t')
			case 'r':
				out = append(out, '\r')
			default:
				out = append(out, esc)
			}
		case '\n':
			return token{}, fmt.Errorf("line %d: unterminated string", line)
		default:
			s.advanceRune(size)
			out = append(out, r)
		}
	}
	return token{}, fmt.Errorf("line %d: unterminated string", line)
}

func (s *scanner) readNumber() (token, error) {
	line := s.line
	start := s.pos
	if s.peek() == '-' {
		s.advance()
	}
	for !s.eof() {
		b := s.peek()
		if (b >= '0' && b <= '9') || b == '.' || b == 'e' || b == 'E' || b == '+' || b == '-' {
			s.advance()
			continue
		}
		break
	}
	text := string(s.src[start:s.pos])
	if _, err := strconv.ParseFloat(text, 64); err != nil {
		return token{}, fmt.Errorf("line %d: invalid number %q", line, text)
	}
	return token{kind: tokenNumber, text: text, line: line}, nil
}

func (s *scanner) readWord() (token, error) {
	line := s.line
	start := s.pos
	for !s.eof() {
		r, size := utf8.DecodeRune(s.src[s.pos:])
		if !isWordRuneRune(r) {
			break
		}
		s.advanceRune(size)
	}
	return token{kind: tokenWord, text: string(s.src[start:s.pos]), line: line}, nil
}

// readLetters reads a CJK/Latin directive word right after '@'.
func (s *scanner) readLetters() string {
	start := s.pos
	for !s.eof() {
		r, size := utf8.DecodeRune(s.src[s.pos:])
		if !unicode.IsLetter(r) {
			break
		}
		s.advanceRune(size)
	}
	return string(s.src[start:s.pos])
}

// readName reads a definition name up to whitespace or the opening brace.
func (s *scanner) readName() string {
	start := s.pos
	for !s.eof() {
		r, size := utf8.DecodeRune(s.src[s.pos:])
		if unicode.IsSpace(r) || r == '{' {
			break
		}
		s.advanceRune(size)
	}
	return string(s.src[start:s.pos])
}

// skipToBrace advances over a directive header until the opening brace.
func (s *scanner) skipToBrace() bool {
	for !s.eof() {
		b := s.peek()
		if b == '"' {
			s.advance()
			if err := s.skipString(); err != nil {
				return false
			}
			continue
		}
		if b == '/' && s.peekByte(1) == '/' {
			for !s.eof() && s.peek() != '\n' {
				s.advance()
			}
			continue
		}
		if b == '{' {
			return true
		}
		s.advance()
	}
	return false
}

// skipString consumes the rest of a string whose opening quote was consumed.
func (s *scanner) skipString() error {
	for !s.eof() {
		b := s.advance()
		if b == '\\' {
			if !s.eof() {
				s.advance()
			}
			continue
		}
		if b == '"' {
			return nil
		}
		if b == '\n' {
			return fmt.Errorf("line %d: unterminated string", s.line)
		}
	}
	return fmt.Errorf("line %d: unterminated string", s.line)
}

// skipBraceBody consumes a balanced { ... } block and returns the byte offset
// of the closing brace. The caller must have consumed the opening brace.
func (s *scanner) skipBraceBody() (int, error) {
	depth := 1
	for !s.eof() {
		b := s.advance()
		switch b {
		case '"':
			if err := s.skipString(); err != nil {
				return 0, err
			}
		case '/':
			if s.peek() == '/' {
				for !s.eof() && s.peek() != '\n' {
					s.advance()
				}
			}
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s.pos - 1, nil
			}
		}
	}
	return 0, fmt.Errorf("line %d: unterminated block", s.line)
}

func isWordRune(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b >= 0x80
}

func isWordRuneRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
