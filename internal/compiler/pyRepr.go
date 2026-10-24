package compiler

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ruicat/astrbox/internal/engine"
)

// pyRepr renders a value the way Python's repr() does for the subset used by
// rule definitions. Rule ids are derived from this text, so byte parity with the
// Python compiler matters.
func pyRepr(value any) string {
	switch typed := value.(type) {
	case nil:
		return "None"
	case bool:
		if typed {
			return "True"
		}
		return "False"
	case string:
		return pyStr(typed)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case int32:
		return strconv.FormatInt(int64(typed), 10)
	case float64:
		return pyFloat(typed)
	case float32:
		return pyFloat(float64(typed))
	case []string:
		items := make([]string, len(typed))
		for index, item := range typed {
			items[index] = pyStr(item)
		}
		return "[" + strings.Join(items, ", ") + "]"
	case []any:
		items := make([]string, len(typed))
		for index, item := range typed {
			items[index] = pyRepr(item)
		}
		return "[" + strings.Join(items, ", ") + "]"
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, pyStr(key)+": "+pyRepr(typed[key]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	default:
		return fmt.Sprint(value)
	}
}

func pyStr(text string) string {
	quote := byte('\'')
	if strings.Contains(text, "'") && !strings.Contains(text, "\"") {
		quote = '"'
	}
	var builder strings.Builder
	builder.WriteByte(quote)
	for _, ch := range text {
		switch {
		case ch == '\\':
			builder.WriteString(`\\`)
		case ch == '\n':
			builder.WriteString(`\n`)
		case ch == '\r':
			builder.WriteString(`\r`)
		case ch == '\t':
			builder.WriteString(`\t`)
		case ch == rune(quote):
			builder.WriteByte('\\')
			builder.WriteRune(ch)
		case ch < 0x20 || ch == 0x7f:
			builder.WriteString(fmt.Sprintf(`\x%02x`, ch))
		default:
			builder.WriteRune(ch)
		}
	}
	builder.WriteByte(quote)
	return builder.String()
}

func pyFloat(value float64) string {
	text := strconv.FormatFloat(value, 'g', -1, 64)
	if !strings.ContainsAny(text, ".eE") {
		text += ".0"
	}
	return text
}

// canonicalRule mirrors the Python compiler's f-string:
// f"{match}|{subjects_list}|{when}|{branches}".
func canonicalRule(match string, subjects []string, when *string, branches []engine.Branch) string {
	var builder strings.Builder
	builder.WriteString(match)
	builder.WriteString("|")
	builder.WriteString(pyRepr(subjects))
	builder.WriteString("|")
	if when == nil {
		builder.WriteString("None")
	} else {
		builder.WriteString(*when)
	}
	builder.WriteString("|")
	builder.WriteString(pyBranches(branches))
	return builder.String()
}

func pyBranches(branches []engine.Branch) string {
	items := make([]string, len(branches))
	for index, branch := range branches {
		items[index] = pyBranch(branch)
	}
	return "[" + strings.Join(items, ", ") + "]"
}

func pyBranch(branch engine.Branch) string {
	parts := make([]string, 0, 4)
	if branch.If != nil {
		parts = append(parts, "'if': "+pyStr(*branch.If))
	}
	if branch.Else {
		parts = append(parts, "'else': True")
	}
	if len(branch.Then) > 0 || branch.ThenPresent {
		parts = append(parts, "'then': "+pyOps(branch.Then))
	}
	if branch.State != nil {
		parts = append(parts, "'state': "+pyRepr(branch.State))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func pyOps(ops []engine.Op) string {
	items := make([]string, len(ops))
	for index, op := range ops {
		items[index] = pyOp(op)
	}
	return "[" + strings.Join(items, ", ") + "]"
}

func pyOp(op engine.Op) string {
	parts := []string{"'op': " + pyStr(op.Op)}
	switch {
	case op.Op == "emit":
		parts = append(parts, "'scene': "+pyStr(op.Entity))
	case op.Entity != "":
		parts = append(parts, "'entity': "+pyStr(op.Entity), "'path': "+pyStr(op.Path))
	case op.Subject != 0:
		parts = append(parts, "'subject': "+strconv.Itoa(op.Subject), "'path': "+pyStr(op.Path))
	default:
		parts = append(parts, "'path': "+pyStr(op.Path))
	}
	parts = append(parts, "'value': "+pyRepr(op.Value))
	return "{" + strings.Join(parts, ", ") + "}"
}
