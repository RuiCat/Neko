// validate.go checks one POST /boards batch before any state changes, so a
// rejected request never leaves a partial board behind (K5 all-or-nothing).
package sessionboard

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	// MAX_OPS bounds one write batch (K5).
	MAX_OPS = 16
	// MAX_VALUE_BYTES bounds one set value's JSON encoding (K5).
	MAX_VALUE_BYTES = 2048
	// MAX_KEY_CHARS bounds one entry key in characters (K5).
	MAX_KEY_CHARS = 128
	// OP_SET writes one key's value (K5).
	OP_SET = "set"
	// OP_DELETE rewrites one key as a tombstone (K5).
	OP_DELETE = "delete"
	// OP_CLEAR rewrites every live key of the caller's region (K5).
	OP_CLEAR = "clear"
)

// Op is one operation of a write batch. Value keeps the raw JSON so the
// encoded-size limit is checked on exactly what the caller sent (K5).
type Op struct {
	Op    string          `json:"op"`
	Key   string          `json:"key,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
}

// writeOp is one validated operation: the value is decoded once, so applying a
// batch cannot fail halfway through (K5).
type writeOp struct {
	kind  string
	key   string
	value any
}

// prepareOps validates the whole batch and decodes its values. A batch that
// passes here is applied all-or-nothing by the store (K5).
func prepareOps(ops []Op) ([]writeOp, error) {
	if len(ops) == 0 {
		return nil, fail(CODE_BAD_REQUEST, "invalid_ops: ops 不能为空")
	}
	if len(ops) > MAX_OPS {
		return nil, fail(CODE_BAD_REQUEST, fmt.Sprintf("too_many_ops: ops 最多 %d 条", MAX_OPS))
	}
	prepared := make([]writeOp, 0, len(ops))
	for index, op := range ops {
		switch op.Op {
		case OP_SET:
			if err := validateKey(op.Key); err != nil {
				return nil, err
			}
			value, err := validateValue(index, op.Value)
			if err != nil {
				return nil, err
			}
			prepared = append(prepared, writeOp{kind: OP_SET, key: op.Key, value: value})
		case OP_DELETE:
			if err := validateKey(op.Key); err != nil {
				return nil, err
			}
			prepared = append(prepared, writeOp{kind: OP_DELETE, key: op.Key})
		case OP_CLEAR:
			if len(ops) != 1 {
				return nil, fail(CODE_BAD_REQUEST, "invalid_ops: clear 不能与其它 op 同批")
			}
			prepared = append(prepared, writeOp{kind: OP_CLEAR})
		default:
			return nil, fail(CODE_BAD_REQUEST, fmt.Sprintf("invalid_ops: op[%d] 未知操作 %q", index, op.Op))
		}
	}
	return prepared, nil
}

// validateKey enforces the key rules of K5: non-blank, at most 128 characters
// and free of newlines and control characters (a key is rendered into text).
func validateKey(key string) error {
	if strings.TrimSpace(key) == "" {
		return fail(CODE_BAD_REQUEST, "invalid_key: key 不能为空")
	}
	if utf8.RuneCountInString(key) > MAX_KEY_CHARS {
		return fail(CODE_BAD_REQUEST, fmt.Sprintf("invalid_key: key 超过 %d 字符", MAX_KEY_CHARS))
	}
	for _, char := range key {
		if char < 0x20 || char == 0x7f {
			return fail(CODE_BAD_REQUEST, "invalid_key: key 不能包含换行或控制字符")
		}
	}
	return nil
}

// validateValue enforces the value rules of K5: present, valid JSON and at most
// 2048 encoded bytes. It returns the decoded value so apply decodes once.
func validateValue(index int, raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, fail(CODE_BAD_REQUEST, fmt.Sprintf("invalid_value: op[%d] set 缺少 value", index))
	}
	if len(raw) > MAX_VALUE_BYTES {
		return nil, fail(CODE_BAD_REQUEST,
			fmt.Sprintf("invalid_value: op[%d] value 编码超过 %d 字节", index, MAX_VALUE_BYTES))
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fail(CODE_BAD_REQUEST, fmt.Sprintf("invalid_value: op[%d] value 不是合法 JSON: %s", index, err))
	}
	return value, nil
}

// validScope reports whether a request names one of the two contract levels.
func validScope(scope string) bool {
	return scope == SCOPE_WORKSPACE || scope == SCOPE_GLOBAL
}
