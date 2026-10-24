// parseScalar.go turns one raw .neko scalar into the value types the Python
// reference produces (string, bool, int, float, dict or list).
package neko

import (
	"encoding/json"
	"io"
	"regexp"
	"strconv"
	"strings"
)

var (
	intLiteralRe   = regexp.MustCompile(`^[+-]?[0-9]+(?:_[0-9]+)*$`)
	floatLiteralRe = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)
)

// parseScalar mirrors the Python parse_scalar helper: quoted text becomes a
// string, true/false become booleans, JSON containers are decoded, numeric
// text becomes a number and anything else stays a raw string.
func parseScalar(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if (strings.HasPrefix(raw, `"`) && strings.HasSuffix(raw, `"`)) ||
		(strings.HasPrefix(raw, `'`) && strings.HasSuffix(raw, `'`)) {
		if len(raw) < 2 {
			return ""
		}
		return raw[1 : len(raw)-1]
	}
	if raw == "true" {
		return true
	}
	if raw == "false" {
		return false
	}
	if strings.HasPrefix(raw, "{") && strings.HasSuffix(raw, "}") {
		if v, err := decodeJSONAny(raw); err == nil {
			return v
		}
		return raw
	}
	if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
		if v, err := decodeJSONAny(raw); err == nil {
			return v
		}
		return raw
	}
	if v, ok := parsePythonInt(raw); ok {
		return v
	}
	if v, ok := parsePythonFloat(raw); ok {
		return v
	}
	return raw
}

// parsePythonInt parses an integer literal the way Python's int() does,
// including underscore digit separators; out-of-range values stay unparsed.
func parsePythonInt(raw string) (int64, bool) {
	if !intLiteralRe.MatchString(raw) {
		return 0, false
	}
	cleaned := strings.ReplaceAll(raw, "_", "")
	v, err := strconv.ParseInt(cleaned, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// parsePythonFloat parses a float literal the way Python's float() does,
// including underscore separators and inf/nan spellings.
func parsePythonFloat(raw string) (float64, bool) {
	cleaned := raw
	if strings.ContainsRune(cleaned, '_') {
		if !validDigitSeparators(cleaned) {
			return 0, false
		}
		cleaned = strings.ReplaceAll(cleaned, "_", "")
	}
	if !floatLiteralRe.MatchString(cleaned) {
		switch strings.ToLower(cleaned) {
		case "inf", "infinity", "+inf", "+infinity", "-inf", "-infinity",
			"nan", "+nan", "-nan":
		default:
			return 0, false
		}
	}
	v, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// validDigitSeparators reports whether every underscore in s sits between two
// digits, which is the only placement Python numeric literals accept.
func validDigitSeparators(s string) bool {
	rs := []rune(s)
	for i, ch := range rs {
		if ch != '_' {
			continue
		}
		if i == 0 || i+1 >= len(rs) {
			return false
		}
		if !isASCIIDigit(rs[i-1]) || !isASCIIDigit(rs[i+1]) {
			return false
		}
	}
	return true
}

// isASCIIDigit reports whether ch is a decimal digit in 0-9.
func isASCIIDigit(ch rune) bool { return ch >= '0' && ch <= '9' }

// decodeJSONAny decodes JSON with Python-like number typing (integer literals
// become int64, everything else float64) and rejects trailing data.
func decodeJSONAny(raw string) (any, error) {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, errTrailingJSON
		}
		return nil, err
	}
	return convertJSONNumbers(v), nil
}

// errTrailingJSON reports JSON data after the first complete value.
var errTrailingJSON = jsonError("trailing data after JSON value")

// jsonError is a plain error type used for JSON decoding diagnostics.
type jsonError string

// Error implements the error interface.
func (e jsonError) Error() string { return string(e) }

// convertJSONNumbers rewrites json.Number values into int64 or float64 so the
// decoded tree matches Python's json module typing.
func convertJSONNumbers(v any) any {
	switch val := v.(type) {
	case json.Number:
		text := val.String()
		if !strings.ContainsAny(text, ".eE") {
			if n, err := strconv.ParseInt(text, 10, 64); err == nil {
				return n
			}
		}
		if f, err := strconv.ParseFloat(text, 64); err == nil {
			return f
		}
		return text
	case map[string]any:
		for k, item := range val {
			val[k] = convertJSONNumbers(item)
		}
		return val
	case []any:
		for i, item := range val {
			val[i] = convertJSONNumbers(item)
		}
		return val
	default:
		return v
	}
}

// isContainerValue reports whether v is a JSON object or array value.
func isContainerValue(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return true
	default:
		return false
	}
}
