// policy.go turns a path into a push tier and a numeric threshold, and loads
// that policy from observation.json. It is the only place that decides whether
// a change is worth pushing.
package observation

import (
	"encoding/json"
	"os"
)

const (
	// POLICY_VERSION is the schema version this build understands.
	POLICY_VERSION = 2
	// DEFAULT_MAX_ITEMS is how many entries one push carries.
	DEFAULT_MAX_ITEMS = 12
	// DEFAULT_MAX_CHARS is the character budget of one rendered push.
	DEFAULT_MAX_CHARS = 1200
	// DEFAULT_COALESCE_MS merges repeated changes of one path inside a window.
	DEFAULT_COALESCE_MS = 60000
)

// Rule is one path pattern and the tier it deserves. The first matching rule in
// declaration order wins.
type Rule struct {
	Pattern   string  `json:"pattern"`
	Tier      Tier    `json:"tier"`
	Threshold float64 `json:"threshold,omitempty"`
}

// Budget bounds one push: item count, rendered characters and the merge window.
type Budget struct {
	MaxItems   int `json:"maxItems"`
	MaxChars   int `json:"maxChars"`
	CoalesceMs int `json:"coalesceMs"`
}

// Policy is the whole observation policy: default tier, path rules and budget.
type Policy struct {
	Version     int    `json:"version"`
	DefaultTier Tier   `json:"defaultTier"`
	Rules       []Rule `json:"rules"`
	Budget      Budget `json:"budget"`
}

// DefaultPolicy returns a fresh copy of the neutral built-in policy: no path
// rules. Tier patterns are business data and come from observation.json (the
// installer seeds it from observation.example.json); without it every path
// keeps DefaultTier.
func DefaultPolicy() *Policy {
	return &Policy{
		Version:     POLICY_VERSION,
		DefaultTier: TierNormal,
		Rules:       []Rule{},
		Budget:      Budget{MaxItems: DEFAULT_MAX_ITEMS, MaxChars: DEFAULT_MAX_CHARS, CoalesceMs: DEFAULT_COALESCE_MS},
	}
}

// LoadPolicy reads the policy file. A missing file yields the neutral built-in
// policy (no path rules) and no error; a malformed file yields the neutral
// policy and an error so the caller can log it and keep serving.
func LoadPolicy(path string) (*Policy, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultPolicy(), nil
		}
		return DefaultPolicy(), err
	}
	return ParsePolicy(raw)
}

// ParsePolicy decodes and normalizes an observation.json payload.
func ParsePolicy(raw []byte) (*Policy, error) {
	policy := DefaultPolicy()
	if err := json.Unmarshal(raw, policy); err != nil {
		return DefaultPolicy(), err
	}
	policy.normalize()
	return policy, nil
}

// normalize fills defaults and clamps invalid values so downstream code never
// has to guard against zero or negative budgets and unknown tiers.
func (p *Policy) normalize() {
	fallback := DefaultPolicy()
	if p.Version <= 0 {
		p.Version = POLICY_VERSION
	}
	if p.DefaultTier != TierCritical && p.DefaultTier != TierNormal && p.DefaultTier != TierNoise {
		p.DefaultTier = fallback.DefaultTier
	}
	if p.Budget.MaxItems <= 0 {
		p.Budget.MaxItems = fallback.Budget.MaxItems
	}
	if p.Budget.MaxChars <= 0 {
		p.Budget.MaxChars = fallback.Budget.MaxChars
	}
	if p.Budget.CoalesceMs <= 0 {
		p.Budget.CoalesceMs = fallback.Budget.CoalesceMs
	}
	for index := range p.Rules {
		switch p.Rules[index].Tier {
		case TierCritical, TierNormal, TierNoise:
		default:
			p.Rules[index].Tier = TierNormal
		}
	}
}

// TierOf returns the tier of one path: the first matching rule, else the
// policy's default tier.
func (p *Policy) TierOf(path string) Tier {
	if p == nil {
		return TierNormal
	}
	for _, rule := range p.Rules {
		if matchPattern(rule.Pattern, path) {
			return rule.Tier
		}
	}
	if p.DefaultTier == "" {
		return TierNormal
	}
	return p.DefaultTier
}

// ThresholdOf returns the numeric threshold of one path (0 = any difference).
func (p *Policy) ThresholdOf(path string) float64 {
	if p == nil {
		return 0
	}
	for _, rule := range p.Rules {
		if matchPattern(rule.Pattern, path) {
			return rule.Threshold
		}
	}
	return 0
}

// BudgetOf returns the policy budget, falling back to the built-in defaults.
func (p *Policy) BudgetOf() Budget {
	if p == nil {
		return DefaultPolicy().Budget
	}
	return p.Budget
}

// matchPattern matches a path against a pattern where '*' stands for any run of
// characters (dots included). It is a two-pointer wildcard match so patterns
// stay cheap on the push path.
func matchPattern(pattern, path string) bool {
	patternIndex, pathIndex := 0, 0
	starIndex, starPath := -1, 0
	for pathIndex < len(path) {
		if patternIndex < len(pattern) && pattern[patternIndex] == path[pathIndex] {
			patternIndex++
			pathIndex++
			continue
		}
		if patternIndex < len(pattern) && pattern[patternIndex] == '*' {
			starIndex = patternIndex
			patternIndex++
			starPath = pathIndex
			continue
		}
		if starIndex >= 0 {
			patternIndex = starIndex + 1
			starPath++
			pathIndex = starPath
			continue
		}
		return false
	}
	for patternIndex < len(pattern) && pattern[patternIndex] == '*' {
		patternIndex++
	}
	return patternIndex == len(pattern)
}
