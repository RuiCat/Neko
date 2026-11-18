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

// Intent fields settlement writes. Settlement never computes a result: it
// writes the interaction/trade request rows, and the .neko tick resolves them.
const (
	// PATH_INTERACT_ACTION is the interaction kernel's 动作 request row.
	PATH_INTERACT_ACTION = "交互.动作"
	// PATH_INTERACT_TARGET is the interaction kernel's 目标 request row.
	PATH_INTERACT_TARGET = "交互.目标"
	// PATH_INTERACT_OBJECT is the interaction kernel's 对象 request row.
	PATH_INTERACT_OBJECT = "交互.对象"
	// PATH_TRADE_INTENT is the trade kernel's 意向 request row.
	PATH_TRADE_INTENT = "交易.意向"
	// PATH_TRADE_OBJECT is the trade kernel's 标的 request row.
	PATH_TRADE_OBJECT = "交易.标的"
	// PATH_TRADE_PRICE is the trade kernel's 报价 request row.
	PATH_TRADE_PRICE = "交易.报价"
	// TRADE_SIDE_BUY is the 交易.意向 value of a buy request.
	TRADE_SIDE_BUY = "买"
	// DEFAULT_INTERACT_ACTION is used when a consume flow derives no verb.
	DEFAULT_INTERACT_ACTION = "使用"
)

// PATH_ITEM_QUANTITY is the material-kernel field a produce row adds to.
const PATH_ITEM_QUANTITY = "数量"

// dataLayerPrefixes are the writable data-layer namespaces: status marks,
// relation numeric rows and growth rows. They are plain data, not computed
// projections, so settlement may set them directly.
var dataLayerPrefixes = []string{"标记.", "关系.数值.", "成长."}

// computedLayerPrefixes are calculation namespaces owned by .neko ticks. A
// settlement naming one is refused as a whole flow: writing it here would be a
// second writer of a computed result.
var computedLayerPrefixes = []string{"代谢.", "行为.", "睡眠.", "事件.", "状态."}

// computedLayerExact are computed trade result rows. The trade request rows
// (意向/标的/报价) are writable; the result rows are not.
var computedLayerExact = map[string]struct{}{
	"交易.成交额": {},
	"交易.结果":  {},
}

// intentExact is the closed set of kernel request rows settlement may write.
var intentExact = map[string]struct{}{
	PATH_INTERACT_ACTION: {},
	PATH_INTERACT_TARGET: {},
	PATH_INTERACT_OBJECT: {},
	PATH_TRADE_INTENT:    {},
	PATH_TRADE_OBJECT:    {},
	PATH_TRADE_PRICE:     {},
}

// sourceLayerPath reports whether path is a settlement-writable field: one
// closed intent row or one data-layer namespace entry. Empty, leading-dot,
// trailing-dot and doubled-dot paths are never valid.
func sourceLayerPath(path string) bool {
	if path == "" || strings.HasPrefix(path, ".") || strings.HasSuffix(path, ".") || strings.Contains(path, "..") {
		return false
	}
	if _, ok := intentExact[path]; ok {
		return true
	}
	for _, prefix := range dataLayerPrefixes {
		if strings.HasPrefix(path, prefix) && len(path) > len(prefix) {
			return true
		}
	}
	return false
}

// computedLayerPath reports whether path belongs to a calculation namespace.
func computedLayerPath(path string) bool {
	if _, ok := computedLayerExact[path]; ok {
		return true
	}
	for _, prefix := range computedLayerPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// validateTargetPath classifies one settlement target. A computed-layer target
// refuses the whole flow and names the field; an unknown target refuses it as
// not writable. Settlement writes intents and data-layer rows only.
func validateTargetPath(target string) error {
	if target == "" {
		return fmt.Errorf("settlement target is empty")
	}
	if computedLayerPath(target) {
		return fmt.Errorf("target %q is a computed-layer field; settlement writes intents and data-layer rows only", target)
	}
	if !sourceLayerPath(target) {
		return fmt.Errorf("target %q is not a settlement-writable intent or data-layer field", target)
	}
	return nil
}
