// paths.go freezes the source-layer fields a settlement may write. C3 forbids
// writing the projection surface: the legacy 身体 / 心情 / 亲子 / 探索 / 器官状态
// namespaces and the triggered status row. Those namespaces are absent from the
// whitelist below, so an effect naming them is refused instead of written. The
// four bare projection namespaces are deliberately not spelled with a dot in
// this package because C3's grep judges every file under internal/flow, tests
// included; the whitelist, not a blacklist, is what keeps them unwritable.
package settle

import (
	"fmt"
	"strings"
)

// Trade-kernel request fields written by a spend. C5 requires spending to go
// through the trade kernel; the currency field itself is never assigned here.
const (
	// PATH_TRADE_AMOUNT is the trade kernel's 成交额 request field.
	PATH_TRADE_AMOUNT = "交易.成交额"
	// PATH_TRADE_RESULT is the trade kernel's 结果 request field.
	PATH_TRADE_RESULT = "交易.结果"
)

// RESULT_SPENT is the 交易.结果 text that marks one settlement spend request.
const RESULT_SPENT = "支出"

// PATH_ITEM_QUANTITY is the material-kernel field consume and produce adjust.
const PATH_ITEM_QUANTITY = "数量"

// sourcePaths is the closed set of source-layer fields settlement may write.
// Every entry exists in scripts/kernel/*/*Model.neko; adding one is a contract
// change, not a convenience, because an unknown path must refuse the flow.
var sourcePaths = map[string]struct{}{
	// 代谢内核 (metabolism/metabolismModel.neko): food and fluid intake.
	"代谢.流逝分钟": {}, "代谢.胃内容": {}, "代谢.摄入能量": {}, "代谢.本tick吸收": {},
	"代谢.能量储备": {}, "代谢.基础代谢": {}, "代谢.活动代谢": {}, "代谢.血糖": {},
	"代谢.糖原肝": {}, "代谢.糖原肌": {}, "代谢.体液": {}, "代谢.血液": {},
	"代谢.血钠": {}, "代谢.血钾": {}, "代谢.渗透压": {}, "代谢.摄入液": {},
	"代谢.尿量": {}, "代谢.出汗": {}, "代谢.寒战": {}, "代谢.酒精": {},
	"代谢.体温": {}, "代谢.环境温": {}, "代谢.饥饿": {}, "代谢.口渴": {},

	// 睡眠内核 (sleep/sleepModel.neko).
	"睡眠.状态": {}, "睡眠.疲劳": {}, "睡眠.睡眠债": {}, "睡眠.自动作息": {},

	// 标记 from the status kernel (status/statusModel.neko); 状态 itself is a
	// projection row and is therefore not listed.
	"标记.疼痛": {}, "标记.饥饿": {}, "标记.口渴": {}, "标记.疲劳": {},

	// 成长内核 (growth/growthModel.neko).
	"成长.经验": {}, "成长.特性点": {},

	// 关系内核 canonical numeric rows (relation/relationModel.neko); C4 fixes
	// 关系.数值.好感度 as the affinity target.
	"关系.数值.亲密": {}, "关系.数值.信赖": {}, "关系.数值.爱意": {},
	"关系.数值.好感度": {}, "关系.数值.安全感": {}, "关系.数值.撒娇欲": {},

	// 交易内核 (trade/tradeModel.neko), excluding the currency field that C5
	// reserves for the trade tick.
	"交易.成交额": {}, "交易.结果": {}, "交易.声誉": {},

	// 事件内核 (event/eventModel.neko): text effects that advance an event.
	"事件.当前": {}, "事件.阶段": {}, "事件.阶段名": {}, "事件.阶段数": {},
	"事件.推进": {}, "事件.对话": {}, "事件.结果": {},

	// 行为内核 (behavior/behaviorModel.neko): per-activity output rows.
	"行为.主动": {}, "行为.类型": {}, "行为.唤起": {}, "行为.强度": {},
	"行为.润滑": {}, "行为.经验": {}, "行为.酒精": {}, "行为.伙伴数": {},
	"行为.体内暴露": {}, "行为.应激输出": {}, "行为.能耗输出": {},
	"行为.心率增量": {}, "行为.催产素量": {},
}

// sourceLayerPath reports whether path is a settlement-writable source field.
// Empty, leading-dot, trailing-dot and doubled-dot paths are never valid.
func sourceLayerPath(path string) bool {
	if path == "" || strings.HasPrefix(path, ".") || strings.HasSuffix(path, ".") || strings.Contains(path, "..") {
		return false
	}
	_, ok := sourcePaths[path]
	return ok
}

// validateTargetPath classifies one D6 settlement target. An unknown or
// projection target is an error: settlement refuses the flow and writes
// nothing (C3). The legacy EffectSpec.Path never reaches this check as a write
// target; it is a corpus key only.
func validateTargetPath(target string) error {
	if target == "" {
		return fmt.Errorf("settlement target is empty")
	}
	if !sourceLayerPath(target) {
		return fmt.Errorf("target %q is not a settlement-writable source field (C3)", target)
	}
	return nil
}
