// convert.go applies the D6 unit conversions between a legacy projection value
// and its source-layer target. The conversion table is closed: only the pair
// listed here is converted, and the target whitelist plus kind checks still run
// first, so an unknown row keeps its literal untouched.
package settle

import "github.com/RuiCat/Neko/internal/flow/spec"

// PATH_STOMACH_CONTENT is the 代谢内核 source field the legacy satiety
// projection derives from (metabolismModel.neko: 胃内容 0-5000).
const PATH_STOMACH_CONTENT = "代谢.胃内容"

// PATH_SATIETY is the legacy 0-100 projection that D6 maps onto
// PATH_STOMACH_CONTENT. It stays a corpus key only; settlement never writes it.
const PATH_SATIETY = "身体.饱食"

// SATIETY_PER_STOMACH is the inverse of m1's projection note
// (satiety = stomach content / 1500 * 100): one satiety point is 15 units of
// stomach content.
const SATIETY_PER_STOMACH = 15

// Bounds of 代谢.胃内容 from metabolismModel.neko.
const (
	STOMACH_CONTENT_MIN = 0
	STOMACH_CONTENT_MAX = 5000
)

// convertEffectValue converts one legacy value onto its source target. The only
// D6 pair that changes units is the satiety projection:
//
//	satiety = stomach content / 1500 * 100  (m1 targetReason)
//	=> stomach content = satiety * 15
//
// The K1 kind decides the semantics, so only units change: set stays absolute
// (the post-state satiety equals the literal, i.e. stomach content = value*15,
// clamped to the kernel range) and add stays a delta (stomach content moves by
// value*15). Only the numeric kinds (set/add) reach this conversion.
func convertEffectValue(legacyPath, target string, kind spec.EffectKind, value int) int {
	if legacyPath != PATH_SATIETY || target != PATH_STOMACH_CONTENT {
		return value
	}
	scaled := value * SATIETY_PER_STOMACH
	if kind == spec.EFFECT_KIND_ADD {
		return scaled
	}
	return clampStomachContent(scaled)
}

// clampStomachContent keeps an absolute set inside the kernel's declared range.
// It only guards malformed legacy literals (a valid 0-100 satiety maps to
// 0-1500); an add is left alone because a delta is not re-based.
func clampStomachContent(value int) int {
	if value < STOMACH_CONTENT_MIN {
		return STOMACH_CONTENT_MIN
	}
	if value > STOMACH_CONTENT_MAX {
		return STOMACH_CONTENT_MAX
	}
	return value
}
