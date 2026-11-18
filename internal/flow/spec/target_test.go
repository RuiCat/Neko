// target_test.go pins D6: every effect has a sourcePaths target or a null with
// an explicit reason, forbidden projection namespaces are never targets, and
// the emitted target set stays a subset of settle's sourcePaths whitelist.
package spec

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestEffectTargetCoverage(t *testing.T) {
	corpus := requireCorpus(t)
	artifacts, err := Generate(corpus)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(artifacts.Effects.Effects) != 55 {
		t.Fatalf("effects = %d, want 55", len(artifacts.Effects.Effects))
	}
	targetsByPath := map[string][]*string{}
	mapped, null := 0, 0
	for _, effect := range artifacts.Effects.Effects {
		if strings.TrimSpace(effect.TargetReason) == "" {
			t.Fatalf("%s: empty targetReason", effect.Path)
		}
		if effect.Target == nil {
			null++
		} else {
			mapped++
			target := *effect.Target
			if !isAllowedTarget(target) {
				t.Fatalf("%s: target %q is not in targetWhitelist", effect.Path, target)
			}
			if isForbiddenTarget(target) {
				t.Fatalf("%s: target %q is C3-forbidden", effect.Path, target)
			}
		}
		targetsByPath[effect.Path] = append(targetsByPath[effect.Path], effect.Target)
	}
	if mapped != 30 || null != 25 {
		t.Fatalf("mapped=%d null=%d, want 30/25", mapped, null)
	}
	for path, targets := range targetsByPath {
		first := targetString(targets[0])
		for _, target := range targets[1:] {
			if targetString(target) != first {
				t.Fatalf("%s: inconsistent targets %q and %q", path, first, targetString(target))
			}
		}
	}
}

func TestTargetWhitelistIsSettleSourcePaths(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "settle", "paths.go"))
	if err != nil {
		t.Fatalf("read settle paths.go: %v", err)
	}
	text := string(data)
	const marker = "var sourcePaths = map[string]struct{}{"
	start := strings.Index(text, marker)
	if start < 0 {
		t.Fatalf("sourcePaths map not found in settle/paths.go")
	}
	block := text[start:]
	end := strings.Index(block, "\n}\n")
	if end < 0 {
		t.Fatalf("sourcePaths map end not found")
	}
	block = block[:end]
	sourcePaths := map[string]struct{}{}
	for _, match := range regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(block, -1) {
		sourcePaths[match[1]] = struct{}{}
	}
	if len(sourcePaths) == 0 {
		t.Fatal("parsed no sourcePaths entries")
	}
	for target := range targetWhitelist {
		if _, ok := sourcePaths[target]; !ok {
			t.Fatalf("targetWhitelist %q is not in settle sourcePaths", target)
		}
	}
	for path, decision := range effectTargetDecisions {
		if !decision.mapped {
			continue
		}
		if !isAllowedTarget(decision.target) {
			t.Fatalf("%s: mapped target %q is not in targetWhitelist", path, decision.target)
		}
		if _, ok := sourcePaths[decision.target]; !ok {
			t.Fatalf("%s: mapped target %q is not in settle sourcePaths", path, decision.target)
		}
	}
}

func TestForbiddenTargetsAreAbsent(t *testing.T) {
	corpus := requireCorpus(t)
	artifacts, err := Generate(corpus)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, effect := range artifacts.Effects.Effects {
		if effect.Target == nil {
			continue
		}
		for _, prefix := range forbiddenTargetPrefixes {
			if strings.HasPrefix(*effect.Target, prefix) {
				t.Fatalf("%s: forbidden target %q (prefix %q)", effect.Path, *effect.Target, prefix)
			}
		}
	}
}

func TestTargetNullAndRelationshipMappings(t *testing.T) {
	corpus := requireCorpus(t)
	artifacts, err := Generate(corpus)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	nullPaths := []string{
		namespaceMood + ".当前",
		namespaceMood + ".稳定",
		namespaceParent + ".梦梦状态",
		namespaceExplore + ".最近发现",
		"身体.水分",
		"身体.活力",
		"身体.清洁",
		"身体.舒适",
		"身体.阴道_有效开口",
		namespaceOrganState + ".基础生理.快感程度",
		namespaceOrganState + ".基础生理.收缩状态",
		namespaceOrganState + ".基础生理.阴道紧致度",
	}
	for _, path := range nullPaths {
		if _, ok := effectTargetDecisions[path]; !ok {
			t.Fatalf("missing decision for %q", path)
		}
		for _, effect := range artifacts.Effects.Effects {
			if effect.Path != path {
				continue
			}
			if effect.Target != nil {
				t.Fatalf("%s: target = %q, want null", path, *effect.Target)
			}
			if strings.TrimSpace(effect.TargetReason) == "" {
				t.Fatalf("%s: null target without reason", path)
			}
		}
	}
	relationship := []string{"关系.亲密"}
	for _, effect := range artifacts.Effects.Effects {
		if effect.Path != relationship[0] {
			continue
		}
		if effect.Target == nil || *effect.Target != "关系.数值.亲密" {
			t.Fatalf("关系.亲密 target = %v, want 关系.数值.亲密", targetString(effect.Target))
		}
	}
}

func TestFlowSpecEffectsCarryTargets(t *testing.T) {
	corpus := requireCorpus(t)
	artifacts, err := Generate(corpus)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	count := 0
	for _, flow := range artifacts.Flows.Flows {
		for _, effect := range flow.Effects {
			count++
			if strings.TrimSpace(effect.TargetReason) == "" {
				t.Fatalf("%s effect %s: empty targetReason", flow.ID, effect.Path)
			}
			if effect.Target != nil && !isAllowedTarget(*effect.Target) {
				t.Fatalf("%s effect %s: bad target %q", flow.ID, effect.Path, *effect.Target)
			}
		}
	}
	if count != len(artifacts.Effects.Effects) {
		t.Fatalf("flow effects = %d, effect-map = %d", count, len(artifacts.Effects.Effects))
	}
}

func targetString(target *string) string {
	if target == nil {
		return "<null>"
	}
	return *target
}
