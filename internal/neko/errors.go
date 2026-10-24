package neko

import "fmt"

// ParseError is a recoverable parse failure bound to a source position.
type ParseError struct {
	Message string
	Pos     Pos
	Hint    string
}

func (e *ParseError) Error() string {
	if e.Pos.Line > 0 {
		return fmt.Sprintf("第 %d 行: %s", e.Pos.Line, e.Message)
	}
	return e.Message
}

// CompileError is a compilation failure bound to a source position.
type CompileError struct {
	Message string
	Pos     Pos
	Hint    string
	Block   string
}

func (e *CompileError) Error() string {
	if e.Pos.Line > 0 {
		return fmt.Sprintf("第 %d 行: %s", e.Pos.Line, e.Message)
	}
	return e.Message
}
