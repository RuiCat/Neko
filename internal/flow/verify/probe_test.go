// probe_test.go evaluates the checked-in probe-cases.json against the same
// in-memory walks the V1/V2/V3 tests use. Every case is {entity, path, expected};
// item quantities use the path 素材.物品.<名称>@<场景> and the V3 replay digest
// uses path r18.replay.sha256 so all three V items stay machine-checkable.
package verify

import (
	"strings"
	"testing"

	"github.com/RuiCat/Neko/internal/flow/settle"
)

// Probe path conventions.
const (
	// ITEM_PATH_PREFIX marks a material quantity probe: <名称>@<场景> follows.
	ITEM_PATH_PREFIX = "素材.物品."
	// REPLAY_DIGEST_PATH is the V3 probe path: the independent replay digest.
	REPLAY_DIGEST_PATH = "r18.replay.sha256"
)

// TestProbeCases rebuilds the V1 chain, the V2 double advance and the V3 replay,
// then checks every probe entry against the resulting entity view. It also
// requires the two V1 fields mandated by scope: 代谢.胃内容 and 标记.饥饿.
func TestProbeCases(t *testing.T) {
	cases := probeCaseFixture(t)
	if len(cases) < 2 {
		t.Fatalf("probe cases = %d, want at least the two V1 fields", len(cases))
	}

	store := newFixtureStore()
	runFlowChain(t, store, settle.NewMemoryLedger())
	document := flowSpecFixture(t)
	advanceTaskTwice(t, store, taskTemplateByID(t, document, "收集任务"))

	flows, err := replayR18(r18Fixture(t))
	if err != nil {
		t.Fatalf("replay r18: %v", err)
	}
	digest := digestReplay(flows)

	required := map[string]bool{
		settle.PATH_STOMACH_CONTENT: false,
		PATH_HUNGER:                 false,
	}
	for index, probe := range cases {
		if probe.Entity == "" || probe.Path == "" || probe.Expected == nil {
			t.Fatalf("case[%d] is incomplete: %+v", index, probe)
		}
		got := resolveProbe(t, store, digest, probe)
		if !expectedEqual(got, probe.Expected) {
			t.Fatalf("case[%d] %s %s = %v, want %v", index, probe.Entity, probe.Path, got, probe.Expected)
		}
		if _, ok := required[probe.Path]; ok {
			required[probe.Path] = true
		}
	}
	for path, seen := range required {
		if !seen {
			t.Fatalf("probe cases do not cover the required V1 field %q", path)
		}
	}
	t.Logf("probe ok: %d cases over one entity view", len(cases))
}

// resolveProbe returns the observed value for one probe path.
func resolveProbe(t *testing.T, store *fakeStore, digest string, probe probeCase) any {
	t.Helper()
	if probe.Path == REPLAY_DIGEST_PATH {
		return digest
	}
	if strings.HasPrefix(probe.Path, ITEM_PATH_PREFIX) {
		name, scene, ok := parseItemPath(probe.Path)
		if !ok {
			t.Fatalf("probe path %q: want %s<名称>@<场景>", probe.Path, ITEM_PATH_PREFIX)
		}
		quantity := store.itemQuantity(name, scene)
		if quantity < 0 {
			t.Fatalf("probe path %q: material view is malformed", probe.Path)
		}
		return quantity
	}
	value, err := store.value(probe.Path)
	if err != nil {
		t.Fatalf("probe path %q: %v", probe.Path, err)
	}
	return value
}

// parseItemPath splits 素材.物品.<名称>@<场景> into its name and scene.
func parseItemPath(path string) (string, string, bool) {
	suffix := strings.TrimPrefix(path, ITEM_PATH_PREFIX)
	at := strings.LastIndex(suffix, "@")
	if at <= 0 || at == len(suffix)-1 {
		return "", "", false
	}
	return suffix[:at], suffix[at+1:], true
}

// expectedEqual compares one observed value with the JSON-decoded expectation.
func expectedEqual(got, expected any) bool {
	if want, ok := asInt(expected); ok {
		actual, ok := asInt(got)
		return ok && actual == want
	}
	switch want := expected.(type) {
	case bool:
		actual, ok := got.(bool)
		return ok && actual == want
	case string:
		actual, ok := got.(string)
		return ok && actual == want
	default:
		return false
	}
}
