// Package compiler turns a parsed .neko AST into templates and compiled rules.
package compiler

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/RuiCat/Neko/internal/config"
	"github.com/RuiCat/Neko/internal/neko"
)

// bareFieldPath resolves an unqualified field through the configured map
// (config.bare_field_map). The table is data, not source: with an empty map a
// bare field keeps its literal meaning.
func bareFieldPath(field string) (string, bool) {
	mapped, ok := config.Current().BareFieldMap[field]
	return mapped, ok
}

var (
	typeSubjectPattern = regexp.MustCompile(`^(\$\d*)\[([^\]]+)\]\.(.*)$`)
	typeOnlyPattern    = regexp.MustCompile(`^(\$\d*)\[([^\]]+)\]$`)
	multiRefPattern    = regexp.MustCompile(`^\$\d+\.`)
	typeDotPattern     = regexp.MustCompile(`\$\[[^\]]+\]\.`)
	bareFieldPattern   = regexp.MustCompile(`^([\p{L}\p{N}_]+(?:\.[\p{L}\p{N}_]+)*)\s*(>=|<=|!=|==|=|>|<|in|contains)\s*(.*)$`)
	rightFieldPattern  = regexp.MustCompile(`^[\p{Han}][\p{Han}\p{L}\p{N}_]*$`)
	pathRefPattern     = regexp.MustCompile(`^[\p{L}\p{N}_]+(?:\.[\p{L}\p{N}_]+)+$`)
	typedRefPattern    = regexp.MustCompile(`\$\[[^\]]+\]\.`)
	typedBarePattern   = regexp.MustCompile(`\$\[[^\]]+\]$`)

	statePrefixPattern = regexp.MustCompile(`^(?:\$\d*\.|\$\[[^\]]+\]\.)`)
)

// SplitLogical splits on top-level ∨ / ∧ and returns parts with their leading
// separator (empty string for the first).
func SplitLogical(text string) []struct {
	Part string
	Sep  string
} {
	parts := []struct {
		Part string
		Sep  string
	}{}
	depth, quote := 0, rune(0)
	var current []rune
	lastSep := ""
	for _, ch := range text {
		switch {
		case quote != 0:
			current = append(current, ch)
			if ch == quote {
				quote = 0
			}
		case ch == '"' || ch == '\'':
			quote = ch
			current = append(current, ch)
		case ch == '(':
			depth++
			current = append(current, ch)
		case ch == ')':
			if depth > 0 {
				depth--
			}
			current = append(current, ch)
		case depth == 0 && (ch == '∨' || ch == '∧'):
			parts = append(parts, struct {
				Part string
				Sep  string
			}{strings.TrimSpace(string(current)), lastSep})
			current = nil
			lastSep = string(ch)
		default:
			current = append(current, ch)
		}
	}
	parts = append(parts, struct {
		Part string
		Sep  string
	}{strings.TrimSpace(string(current)), lastSep})
	return parts
}

// RewriteCondition rewrites $-prefixed subjects to {e} and qualifies bare fields.
func RewriteCondition(cond string, subjects []neko.Subject) (string, []string) {
	extraWhen := []string{}
	if strings.TrimSpace(cond) == "" {
		return cond, extraWhen
	}
	text := strings.TrimSpace(cond)

	var primary *neko.Subject
	multi := false
	for i := range subjects {
		switch subjects[i].Kind {
		case "primary":
			primary = &subjects[i]
		case "multi":
			multi = true
		}
	}

	if parts := SplitLogical(text); len(parts) > 1 {
		var rewritten []string
		var separators []string
		for _, part := range parts {
			if strings.TrimSpace(part.Part) == "" {
				continue
			}
			value, extra := RewriteCondition(part.Part, subjects)
			rewritten = append(rewritten, value)
			separators = append(separators, part.Sep)
			extraWhen = append(extraWhen, extra...)
		}
		if len(rewritten) > 0 {
			result := rewritten[0]
			for i := 1; i < len(rewritten) && i < len(separators); i++ {
				result += " " + separators[i] + " " + rewritten[i]
			}
			return result, extraWhen
		}
	}

	if match := typeSubjectPattern.FindStringSubmatch(text); match != nil && (match[1] == "$" || match[1] == "") {
		typeCond, rest := match[2], match[3]
		if strings.Contains(typeCond, ".") && strings.Contains(typeCond, "=") {
			attr := typeCond[strings.Index(typeCond, ".")+1:]
			extraWhen = append(extraWhen, "{e}."+attr)
		}
		text = "{e}." + rest
		if !multi {
			text = typeDotPattern.ReplaceAllString(text, "{e}.")
			text = strings.ReplaceAll(text, "$.", "{e}.")
		}
		return text, extraWhen
	}

	if typeOnlyPattern.MatchString(text) {
		return "{e}", extraWhen
	}
	if strings.HasPrefix(text, "$.") {
		return strings.Replace(text, "$.", "{e}.", 1), extraWhen
	}
	if text == "$" {
		return "{e}", extraWhen
	}
	if multiRefPattern.MatchString(text) {
		return text, extraWhen
	}
	if strings.HasPrefix(text, "{e}.") {
		if !multi {
			text = typeDotPattern.ReplaceAllString(text, "{e}.")
			text = strings.ReplaceAll(text, "$.", "{e}.")
		}
		return text, extraWhen
	}

	first := strings.TrimSpace(strings.SplitN(text, ".", 2)[0])
	if config.Current().IsGlobalEntity(first) || (primary == nil && !multi) {
		if primary != nil && !multi {
			text = typeDotPattern.ReplaceAllString(text, "{e}.")
			text = strings.ReplaceAll(text, "$.", "{e}.")
		}
		return text, extraWhen
	}

	if match := bareFieldPattern.FindStringSubmatch(text); match != nil {
		field, op, value := match[1], match[2], match[3]
		if op == ">" || op == ">=" || op == "<" || op == "<=" {
			raw := strings.TrimSpace(value)
			if rightFieldPattern.MatchString(raw) && !strings.HasPrefix(raw, ".") && !isDigitStart(raw) {
				if mapped, ok := bareFieldPath(raw); ok {
					value = "{e}." + config.Current().NormalizePath(mapped)
				} else {
					value = "{e}.身体." + raw
				}
			}
		}
		if strings.Contains(field, ".") {
			return fmt.Sprintf("{e}.%s %s %s", field, op, value), extraWhen
		}
		if mapped, ok := bareFieldPath(field); ok {
			return fmt.Sprintf("{e}.%s %s %s", config.Current().NormalizePath(mapped), op, value), extraWhen
		}
		return fmt.Sprintf("{e}.%s.%s %s %s", config.Current().DefaultStateSubgroup, field, op, value), extraWhen
	}

	if strings.Contains(text, "(") {
		return RewritePrimaryRefs(text), extraWhen
	}
	return text, extraWhen
}

func isDigitStart(value string) bool {
	if value == "" {
		return false
	}
	ch := value[0]
	return ch >= '0' && ch <= '9'
}

// RewritePrimaryRefs replaces single-subject references with {e}.
func RewritePrimaryRefs(text string) string {
	text = typeDotPattern.ReplaceAllString(text, "{e}.")
	text = strings.ReplaceAll(text, "$.", "{e}.")
	return replaceBareDollar(text)
}

// RewriteStateWhen the 状态: line into a concrete condition.
func RewriteStateWhen(stateWhen *string) *string {
	if stateWhen == nil || strings.TrimSpace(*stateWhen) == "" {
		return nil
	}
	text := strings.TrimSpace(*stateWhen)
	if match := stateLineRegex().FindStringSubmatch(text); match != nil {
		group := match[1]
		if group == "" {
			group = config.Current().DefaultStateSubgroup
		}
		result := fmt.Sprintf("{e}.%s.%s = %s", config.Current().StateGroup, group, strings.TrimSpace(match[2]))
		return &result
	}
	text = statePrefixPattern.ReplaceAllString(text, "{e}.")
	if strings.HasPrefix(text, "$.") {
		text = "{e}." + text[2:]
	}
	return &text
}

// replaceBareDollar replaces a standalone "$" that is not part of "$1"/"$2".
func replaceBareDollar(text string) string {
	runes := []rune(text)
	var out []rune
	for i, ch := range runes {
		if ch != '$' {
			out = append(out, ch)
			continue
		}
		prevDigit := i > 0 && runes[i-1] >= '0' && runes[i-1] <= '9'
		nextDigit := i+1 < len(runes) && runes[i+1] >= '0' && runes[i+1] <= '9'
		if prevDigit || nextDigit {
			out = append(out, ch)
			continue
		}
		out = append(out, []rune("{e}")...)
	}
	return string(out)
}

// stateLineRegex builds the `状态: ...` condition matcher from the active config.
func stateLineRegex() *regexp.Regexp {
	group := regexp.QuoteMeta(config.Current().StateGroup)
	return regexp.MustCompile(`^(?:\$\d*\.|\$\[[^\]]+\]\.)?` + group + `\s*(?:\.([\p{L}\p{N}_]+))?\s*=\s*(.+)$`)
}
