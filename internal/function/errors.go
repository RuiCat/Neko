// Typed interpreter errors for try/except type matching.
package function

import (
	"errors"
	"fmt"
)

// scriptError carries a coarse error kind (value/type/name/generic).
type scriptError struct {
	kind    string
	message string
}

func (e *scriptError) Error() string { return e.message }

func newScriptError(kind, format string, args ...any) error {
	return &scriptError{kind: kind, message: fmt.Sprintf(format, args...)}
}

// errorKind returns the kind of an interpreter error.
func errorKind(err error) string {
	var typed *scriptError
	if errors.As(err, &typed) {
		return typed.kind
	}
	return "generic"
}

// exceptMatches reports whether an except type name matches an error.
func exceptMatches(typeName string, err error) bool {
	kind := errorKind(err)
	switch typeName {
	case "值错误", "ValueError", "value":
		return kind == "value"
	case "类型错误", "TypeError", "type":
		return kind == "type"
	case "名称错误", "NameError", "name":
		return kind == "name"
	case "错误", "异常", "Exception":
		return true
	}
	return false
}
