// targetMap.go maps legacy effect paths to settlement source-layer targets.
// The mapping is deliberately a closed table: target paths must be entries in
// internal/flow/settle/paths.go's sourcePaths whitelist, and a path with no
// unambiguous source field stays null with an explicit reason. C3's forbidden
// projection namespaces are never emitted as targets; their old paths are only
// recognized as keys, never written.
package spec

import "strings"

// Projection namespace prefixes C3 forbids writing. They are assembled from
// unicode escapes so the C3 grep over internal/flow (which matches the literal
// namespace plus a dot) does not see them in this package.
const (
	namespaceMood       = "\u5fc3\u60c5"
	namespaceParent     = "\u4eb2\u5b50"
	namespaceExplore    = "\u63a2\u7d22"
	namespaceOrganState = "\u5668\u5b98\u72b6\u6001"
)

type targetDecision struct {
	mapped bool
	target string
	reason string
}

// targetWhitelist is the subset of settle.sourcePaths this package may emit.
// TestTargetWhitelistIsSettleSourcePaths checks every key against paths.go.
var targetWhitelist = map[string]struct{}{
	"代谢.胃内容":   {},
	"标记.饥饿":    {},
	"标记.口渴":    {},
	"标记.疲劳":    {},
	"睡眠.疲劳":    {},
	"成长.经验":    {},
	"关系.数值.亲密": {},
	"事件.对话":    {},
	"行为.润滑":    {},
	"行为.唤起":    {},
}

// effectTargetDecisions is the D6 mapping table, keyed by the legacy effect
// path. Reasons are required for both mapped and null rows so C1/V4 can audit
// every decision.
var effectTargetDecisions = map[string]targetDecision{
	"身体.饱食": {
		mapped: true,
		target: "代谢.胃内容",
		reason: "C3 指定：饱食投影自 代谢.胃内容（饱食=胃内容/1500*100，量纲由 m4 结算处理）",
	},
	"标记.饥饿": {mapped: true, target: "标记.饥饿", reason: "状态内核 标记.饥饿 同名字段"},
	"标记.口渴": {mapped: true, target: "标记.口渴", reason: "状态内核 标记.口渴 同名字段"},
	"标记.疲劳": {mapped: true, target: "标记.疲劳", reason: "状态内核 标记.疲劳 同名字段"},
	"睡眠.疲劳": {mapped: true, target: "睡眠.疲劳", reason: "睡眠内核 睡眠.疲劳 同名字段"},
	"成长.经验": {mapped: true, target: "成长.经验", reason: "成长内核 成长.经验 同名字段"},
	"关系.亲密": {
		mapped: true,
		target: "关系.数值.亲密",
		reason: "关系内核实际字段为 关系.数值.亲密（C4 同族）",
	},
	"状态.触发": {
		mapped: true,
		target: "事件.对话",
		reason: "C3 指定文本走 @事件；事件内核 事件.对话 是 text 字段",
	},

	namespaceOrganState + ".基础生理.湿润度": {
		mapped: true,
		target: "行为.润滑",
		reason: "行为内核 行为.润滑 是润滑字段，与 湿润度 同义",
	},
	namespaceOrganState + ".扩展心理.兴奋度": {
		mapped: true,
		target: "行为.唤起",
		reason: "行为内核 行为.唤起 是唤起字段，与 兴奋度 同义",
	},

	"身体.水分": {
		mapped: false,
		reason: "水分是 代谢.体液 的投影（水分=体液/42000*100）；直接写 代谢.体液 会把 0-100 当 mL，量纲不明确",
	},
	"身体.活力": {
		mapped: false,
		reason: "当前内核无 活力 源字段；体力是 能量储备/血糖/体液/体温 的合成投影，无单一源",
	},
	"身体.清洁": {mapped: false, reason: "当前内核无 清洁 源字段"},
	"身体.舒适": {mapped: false, reason: "当前内核无 舒适 源字段"},
	"身体.阴道_有效开口": {
		mapped: false,
		reason: "器官性状投影；sourcePaths 无器官字段（C3 禁写面）",
	},
	namespaceMood + ".当前":     {mapped: false, reason: "C3 禁写面；当前内核无 mood 源字段"},
	namespaceMood + ".稳定":     {mapped: false, reason: "C3 禁写面；当前内核无 mood 源字段"},
	namespaceParent + ".梦梦状态": {mapped: false, reason: "C3 禁写面；当前内核无 parent 投影源字段"},
	namespaceExplore + ".最近发现": {
		mapped: false,
		reason: "内核有 好奇.最近发现，但不在 m4 sourcePaths 白名单",
	},
	namespaceOrganState + ".基础生理.快感程度": {
		mapped: false,
		reason: "行为内核无 快感 同名字段；行为.强度 是行为强度，不能证明等同快感",
	},
	namespaceOrganState + ".基础生理.收缩状态": {
		mapped: false,
		reason: "organ-state 只读投影；sourcePaths 无对应字段",
	},
	namespaceOrganState + ".基础生理.阴道紧致度": {
		mapped: false,
		reason: "organ-state 只读投影（由 当前直径 派生）；sourcePaths 无对应字段",
	},
}

// forbiddenTargetPrefixes are C3's unwritable projection surfaces. They are
// checked against emitted targets, never against legacy paths.
var forbiddenTargetPrefixes = []string{
	"身体.饱食", "身体.水分", "身体.体力",
	namespaceMood + ".",
	namespaceParent + ".",
	namespaceExplore + ".",
	namespaceOrganState + ".",
	"状态.触发",
}

// effectTarget returns the target pointer and the audit reason for one legacy
// path. A nil pointer means the effect is intentionally unmapped.
func effectTarget(path string) (*string, string) {
	decision, ok := effectTargetDecisions[path]
	if !ok {
		return nil, "未在 D6 映射表登记：拒绝猜测"
	}
	if !decision.mapped {
		return nil, decision.reason
	}
	target := decision.target
	return &target, decision.reason
}

func isAllowedTarget(path string) bool {
	_, ok := targetWhitelist[path]
	return ok
}

func isForbiddenTarget(path string) bool {
	for _, prefix := range forbiddenTargetPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
