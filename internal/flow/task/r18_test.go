// r18_test.go asserts the C7 artifact structure against the read-only corpus, and
// that the checked-in artifact is exactly what the extractor produces.
package task

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RuiCat/Neko/internal/flow/spec"
)

// R18_FLOW_COUNT is the number of R18 flows C7 requires.
const R18_FLOW_COUNT = 12

// TestR18Structure asserts 12/12 flows carry a source anchor and at least one step.
// D7 marked V3=descoped, so effects are allowed to be empty and a future non-empty
// list is not a failure; what must hold is that an empty list is explained by the
// artifact gaps instead of silently pretending effects exist.
func TestR18Structure(t *testing.T) {
	corpus := DefaultR18Corpus()
	artifact, err := ExtractR18Flows(corpus)
	if err != nil {
		t.Fatalf("extract r18 flows: %v", err)
	}
	if len(artifact.R18Flows) != R18_FLOW_COUNT {
		t.Fatalf("r18 flows = %d, want %d", len(artifact.R18Flows), R18_FLOW_COUNT)
	}
	raw, err := os.ReadFile(corpus)
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}
	lines := strings.Split(string(raw), "\n")
	withSteps := 0
	withoutEffects := 0
	seen := map[string]struct{}{}
	for _, flow := range artifact.R18Flows {
		if strings.TrimSpace(flow.ID) == "" {
			t.Fatal("r18 flow has an empty id")
		}
		if _, duplicate := seen[flow.ID]; duplicate {
			t.Fatalf("duplicate r18 flow id %q", flow.ID)
		}
		seen[flow.ID] = struct{}{}
		if flow.Source.File != corpus {
			t.Fatalf("r18 flow %q source file = %q, want %q", flow.ID, flow.Source.File, corpus)
		}
		if flow.Source.Line < 1 || flow.Source.Line > len(lines) {
			t.Fatalf("r18 flow %q source line %d is outside the corpus", flow.ID, flow.Source.Line)
		}
		if !strings.Contains(lines[flow.Source.Line-1], `"`+flow.ID+`"`) {
			t.Fatalf("r18 flow %q anchor %s:%d does not name the id", flow.ID, flow.Source.File, flow.Source.Line)
		}
		if len(flow.Steps) == 0 {
			t.Fatalf("r18 flow %q has no steps", flow.ID)
		}
		for index, step := range flow.Steps {
			if strings.TrimSpace(step) == "" {
				t.Fatalf("r18 flow %q step %d is empty", flow.ID, index)
			}
		}
		withSteps++
		if len(flow.Effects) == 0 {
			withoutEffects++
		}
	}
	if withSteps != R18_FLOW_COUNT {
		t.Fatalf("flows with steps = %d, want %d", withSteps, R18_FLOW_COUNT)
	}
	if withoutEffects > 0 && len(artifact.Gaps) == 0 {
		t.Fatal("flows without effects must be explained by the artifact gaps (V3=descoped)")
	}
}

// TestR18ArtifactMatchesCorpus asserts runs/legacy-extraction-p4/r18-flows.json is
// exactly the canonical encoding of the current extraction, so a corpus change
// cannot leave the checked-in artifact stale.
func TestR18ArtifactMatchesCorpus(t *testing.T) {
	corpus := DefaultR18Corpus()
	extracted, err := ExtractR18Flows(corpus)
	if err != nil {
		t.Fatalf("extract r18 flows: %v", err)
	}
	want, err := spec.MarshalCanonical(extracted)
	if err != nil {
		t.Fatalf("encode r18 flows: %v", err)
	}
	artifact := filepath.Join(repoRoot(t), "runs", "legacy-extraction-p4", "r18-flows.json")
	have, err := os.ReadFile(artifact)
	if err != nil {
		t.Fatalf("read %s: %v (regenerate it with task.WriteR18Flows)", artifact, err)
	}
	if string(have) != string(want) {
		t.Fatalf("%s drifted from the corpus; regenerate it with task.WriteR18Flows", artifact)
	}
}
