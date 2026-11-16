// publish.go lets engine scripts publish messages onto the shared session
// board: a fixed global region owned by the system, one key per entity.
package sessionboard

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	// PUBLISH_BY is the entry author of script-published messages.
	PUBLISH_BY = "脚本"
	// PUBLISH_REGION is the fixed global-board region for script messages.
	PUBLISH_REGION = "脚本"
	// OWNER_SYSTEM marks a system-owned region (not a session or workspace).
	OWNER_SYSTEM = "system"
)

// PublishRequest is one script message: Key scopes the message (the entity id),
// Value is the message text and By optionally overrides the author.
type PublishRequest struct {
	Key   string
	Value any
	By    string
}

// Publish writes one script message to the shared global region. It carries no
// session identity: the region is system-owned, and every session subscribed to
// the global board receives the entry through the normal delta channel.
func (s *Store) Publish(request PublishRequest) (WriteResult, error) {
	if strings.TrimSpace(request.Key) == "" {
		return WriteResult{}, fail(CODE_BAD_REQUEST, "invalid_key: key 不能为空")
	}
	raw, err := json.Marshal(request.Value)
	if err != nil {
		return WriteResult{}, fail(CODE_BAD_REQUEST, "invalid_value: 消息无法序列化")
	}
	ops, err := prepareOps([]Op{{Op: OP_SET, Key: request.Key, Value: raw}})
	if err != nil {
		return WriteResult{}, err
	}
	by := strings.TrimSpace(request.By)
	if by == "" {
		by = PUBLISH_BY
	}
	target := Target{RegionID: PUBLISH_REGION, Owner: Owner{Kind: OWNER_SYSTEM, ID: PUBLISH_REGION}}
	s.mu.Lock()
	defer s.mu.Unlock()
	board, _ := s.boardCopy(SCOPE_GLOBAL, "")
	now := s.now()
	changed, trimmed := board.applyOps(target, by, ops, now)
	if !changed {
		return WriteResult{Changed: false, Version: board.Version}, nil
	}
	evicted := board.enforceRegionCap()
	size, err := board.encodeSize()
	if err != nil {
		return WriteResult{}, fail(CODE_PERSIST_FAILED, "序列化候选板失败: "+err.Error())
	}
	if size > MAX_BOARD_BYTES {
		return WriteResult{}, fail(CODE_BOARD_TOO_LARGE,
			fmt.Sprintf("单板序列化超过 %d 字节上限", MAX_BOARD_BYTES))
	}
	payload, err := s.encodeCandidate(SCOPE_GLOBAL, "", board, now)
	if err != nil {
		return WriteResult{}, fail(CODE_PERSIST_FAILED, err.Error())
	}
	if len(payload) > MAX_STORE_BYTES {
		return WriteResult{}, fail(CODE_STORE_TOO_LARGE,
			fmt.Sprintf("会话黑板存储超过 %d 字节上限", MAX_STORE_BYTES))
	}
	if err := writeFileAtomic(s.path, payload); err != nil {
		return WriteResult{}, fail(CODE_PERSIST_FAILED, "保存会话黑板失败: "+err.Error())
	}
	s.commit(SCOPE_GLOBAL, "", board)
	return WriteResult{Changed: true, Version: board.Version, UpdatedAt: board.UpdatedAt,
		Trimmed: trimmed, Evicted: evicted}, nil
}
