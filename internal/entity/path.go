package entity

import (
	"fmt"
	"regexp"
	"strings"
)

// SelectorKind enumerates the supported entity reference forms.
type SelectorKind string

const (
	SelectorAll          SelectorKind = "all"
	SelectorByID         SelectorKind = "by_id"
	SelectorByType       SelectorKind = "by_type"
	SelectorTypeWildcard SelectorKind = "type_wildcard"
)

// Selector references entities by id, type, type wildcard or globally.
type Selector struct {
	Kind     SelectorKind
	EntityID string
	TypeName string
}

var selectorPattern = regexp.MustCompile(`^` +
	`(?:\[(?P<all>\*)\])` +
	`|^(?P<byType>(?P<t1>[\p{L}\p{N}_:]+)\[(?P<n1>[\p{L}\p{N}_]+)\])` +
	`|^(?P<wild>(?P<t2>[\p{L}\p{N}_:]+)\[\*\])` +
	`|^(?P<byID>[\p{L}\p{N}_]+)$`)

// ParseEntityRef parses NAME, TYPE[NAME], TYPE[*] and [*].
func ParseEntityRef(ref string) (Selector, error) {
	ref = strings.TrimSpace(ref)
	if ref == "[*]" {
		return Selector{Kind: SelectorAll}, nil
	}
	match := selectorPattern.FindStringSubmatch(ref)
	if match == nil {
		return Selector{}, &PathNotFoundError{Message: fmt.Sprintf("无效的实体引用: '%s'", ref)}
	}
	names := selectorPattern.SubexpNames()
	groups := map[string]string{}
	for i, name := range names {
		if name != "" && i < len(match) {
			groups[name] = match[i]
		}
	}
	if groups["byType"] != "" {
		return Selector{Kind: SelectorByType, EntityID: groups["n1"], TypeName: groups["t1"]}, nil
	}
	if groups["wild"] != "" {
		return Selector{Kind: SelectorTypeWildcard, TypeName: groups["t2"]}, nil
	}
	if groups["byID"] != "" {
		return Selector{Kind: SelectorByID, EntityID: groups["byID"]}, nil
	}
	return Selector{}, &PathNotFoundError{Message: fmt.Sprintf("无法解析实体引用: '%s'", ref)}
}

// ParseEntityPath splits "NPC[*].身体.体力" into selector and field parts.
func ParseEntityPath(path string) (Selector, []string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return Selector{}, nil, &PathNotFoundError{Message: "空路径"}
	}
	ref := path
	var fields []string
	if index := strings.Index(path, "."); index >= 0 {
		ref = path[:index]
		fields = strings.Split(path[index+1:], ".")
	}
	sel, err := ParseEntityRef(ref)
	return sel, fields, err
}

// Matches reports whether a concrete entity matches the selector. Classifications
// provide the extra multi-inheritance labels from the "分类" field.
func (s Selector) Matches(entityType, entityID string, classifications []string) bool {
	switch s.Kind {
	case SelectorAll:
		return true
	case SelectorByID:
		return entityID == s.EntityID
	case SelectorByType:
		return entityID == s.EntityID && s.TypeMatches(entityType, classifications)
	case SelectorTypeWildcard:
		return s.TypeMatches(entityType, classifications)
	}
	return false
}

// TypeMatches accepts subtype prefixes and classification labels.
func (s Selector) TypeMatches(entityType string, classifications []string) bool {
	if s.TypeName == "" {
		return true
	}
	if entityType == s.TypeName || strings.HasPrefix(entityType, s.TypeName+":") {
		return true
	}
	for _, label := range classifications {
		if label == s.TypeName {
			return true
		}
	}
	return false
}

// BindSelectorToID rewrites wildcard selectors to a concrete entity id.
func BindSelectorToID(expr, entityID string) string {
	re := regexp.MustCompile(`\[[*]\]\.`)
	return re.ReplaceAllString(expr, entityID+".")
}

// ExtractSelector returns the first selector found in an expression.
func ExtractSelector(expr string) (Selector, bool) {
	re := regexp.MustCompile(`(?:[\p{L}\p{N}_:]+\[[*]?[\p{L}\p{N}_]*\]|\[\*\]|[\p{L}\p{N}_]+)(?:\.[\p{L}\p{N}_]+)*`)
	for _, raw := range re.FindAllString(expr, -1) {
		ref := raw
		if index := strings.IndexByte(ref, '.'); index >= 0 {
			ref = ref[:index]
		}
		if ref == "" {
			continue
		}
		if sel, err := ParseEntityRef(ref); err == nil {
			return sel, true
		}
	}
	return Selector{}, false
}

// ExtractFieldAfter returns the field path after a concrete entity id.
func ExtractFieldAfter(expr, entityID string) string {
	re := regexp.MustCompile(regexp.QuoteMeta(entityID) + `\.([\p{L}\p{N}_.]+)`)
	match := re.FindStringSubmatch(expr)
	if match == nil {
		return ""
	}
	return match[1]
}
