// effects.go converts K1 EffectSpec rows into validated source-layer changes.
// D6 moves the write target onto EffectSpec.Target: the legacy Path is only a
// corpus key and is never written. Every row is validated before any write; a
// nil target or a bad literal refuses the whole settlement.
package settle

import (
	"fmt"
	"strings"

	"github.com/RuiCat/Neko/internal/flow/spec"
)

// planEffects validates every mapped effect and converts it to one OP_EFFECT
// change. An effect with a nil target is intentionally unmapped: its field is
// computed by a .neko tick, so settlement skips it instead of writing or
// refusing. It collects all errors so the caller sees every offending row.
func planEffects(entity string, effects []spec.EffectSpec) ([]Change, []error) {
	changes := make([]Change, 0, len(effects))
	var errs []error
	for index, effect := range effects {
		if effect.Target == nil {
			continue
		}
		target, err := resolveTarget(effect)
		if err != nil {
			errs = append(errs, fmt.Errorf("effect[%d] %w", index, err))
			continue
		}
		value, err := effectValue(effect, target)
		if err != nil {
			errs = append(errs, fmt.Errorf("effect[%d] path %q target %q: %w", index, effect.Path, target, err))
			continue
		}
		changes = append(changes, Change{
			Op:     OP_EFFECT,
			Entity: entity,
			Path:   target,
			Kind:   effect.Kind,
			Value:  value,
		})
	}
	return changes, errs
}

// resolveTarget returns the settlement target of one mapped effect. A non-nil
// target must pass the closed write-domain check, which also rejects a legacy
// projection or computed-layer path used as the target.
func resolveTarget(effect spec.EffectSpec) (string, error) {
	if effect.Target == nil {
		return "", fmt.Errorf("path %q has no settlement target", effect.Path)
	}
	target := *effect.Target
	if strings.TrimSpace(target) == "" {
		return "", fmt.Errorf("path %q has a blank settlement target", effect.Path)
	}
	if err := validateTargetPath(target); err != nil {
		return "", fmt.Errorf("path %q: %w", effect.Path, err)
	}
	return target, nil
}

// effectValue validates the literal against the K1 kind, converts legacy units
// where D6 requires it, and returns the value written to the source field.
func effectValue(effect spec.EffectSpec, target string) (any, error) {
	switch effect.Kind {
	case spec.EFFECT_KIND_SET, spec.EFFECT_KIND_ADD:
		value, err := toInteger(effect.Value)
		if err != nil {
			return nil, err
		}
		return value, nil
	case spec.EFFECT_KIND_TEXT:
		text, ok := effect.Value.(string)
		if !ok {
			return nil, fmt.Errorf("kind %s wants string, got %T", effect.Kind, effect.Value)
		}
		return text, nil
	case spec.EFFECT_KIND_BOOL:
		flag, ok := effect.Value.(bool)
		if !ok {
			return nil, fmt.Errorf("kind %s wants bool, got %T", effect.Kind, effect.Value)
		}
		return flag, nil
	default:
		return nil, fmt.Errorf("unknown effect kind %q", effect.Kind)
	}
}
