// r18Replay_test.go replays the C7 R18 flows from the checked-in artifact and
// asserts the replay is deterministic. D7 marked V3=descoped: effects may be empty
// and a future non-empty list must not change the replay, so the walk reads only
// the flow ID and its ordered steps.
package task

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/RuiCat/Neko/internal/flow/spec"
)

// R18_REPLAY_COUNT is how often TestR18Replay walks the artifact.
const R18_REPLAY_COUNT = 2

// r18Replay is the state one walk of one flow produces: the flow ID, the walked
// step rows and their digest.
type r18Replay struct {
	FlowID    string   `json:"flowID"`
	StepCount int      `json:"stepCount"`
	Steps     []string `json:"steps"`
	Digest    string   `json:"digest"`
}

// replayR18Flows walks every flow's steps in order and returns the per-flow state
// plus one digest over the whole replay. Effects are deliberately not consulted,
// which is what keeps V3=descoped valid now and after a future corpus adds effects.
func replayR18Flows(document R18FlowsFile) ([]r18Replay, string, error) {
	states := make([]r18Replay, 0, len(document.R18Flows))
	for _, flow := range document.R18Flows {
		walked := make([]string, 0, len(flow.Steps))
		for index, step := range flow.Steps {
			walked = append(walked, fmt.Sprintf("%d:%s", index+1, step))
		}
		digest, err := digestOf(walked)
		if err != nil {
			return nil, "", err
		}
		states = append(states, r18Replay{
			FlowID:    flow.ID,
			StepCount: len(flow.Steps),
			Steps:     walked,
			Digest:    digest,
		})
	}
	whole, err := digestOf(states)
	if err != nil {
		return nil, "", err
	}
	return states, whole, nil
}

// digestOf hashes the canonical JSON encoding of value so two replays compare by
// bytes instead of by map order or number formatting.
func digestOf(value any) (string, error) {
	data, err := spec.MarshalCanonical(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// readR18Artifact loads the checked-in C7 artifact.
func readR18Artifact(t *testing.T) R18FlowsFile {
	t.Helper()
	path := filepath.Join(repoRoot(t), "runs", "legacy-extraction-p4", "r18-flows.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var document R18FlowsFile
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&document); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return document
}

// TestR18Replay walks the 12 artifact flows twice and asserts both runs produce
// exactly the same state and digest. Empty effects are fine (V3=descoped) as long
// as every flow has steps, and injecting effects later must not change the walk.
func TestR18Replay(t *testing.T) {
	document := readR18Artifact(t)
	if len(document.R18Flows) != R18_FLOW_COUNT {
		t.Fatalf("r18 flows = %d, want %d", len(document.R18Flows), R18_FLOW_COUNT)
	}
	totalSteps := 0
	for _, flow := range document.R18Flows {
		if len(flow.Steps) == 0 {
			t.Fatalf("r18 flow %q has no steps", flow.ID)
		}
		totalSteps += len(flow.Steps)
	}
	if totalSteps == 0 {
		t.Fatal("artifact replay walked no steps")
	}

	first, firstDigest, err := replayR18Flows(document)
	if err != nil {
		t.Fatalf("first replay: %v", err)
	}
	second, secondDigest, err := replayR18Flows(document)
	if err != nil {
		t.Fatalf("second replay: %v", err)
	}
	if R18_REPLAY_COUNT != 2 {
		t.Fatalf("replay count = %d, want 2", R18_REPLAY_COUNT)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("two replays produced different state")
	}
	if firstDigest != secondDigest {
		t.Fatalf("two replays produced different digests: %s vs %s", firstDigest, secondDigest)
	}

	withEffects := R18FlowsFile{
		SchemaVersion: document.SchemaVersion,
		Corpus:        document.Corpus,
		R18Flows:      append([]R18Flow(nil), document.R18Flows...),
	}
	for index := range withEffects.R18Flows {
		withEffects.R18Flows[index].Effects = []spec.EffectSpec{{
			Path:  "行为.润滑",
			Kind:  spec.EFFECT_KIND_SET,
			Value: 1,
		}}
	}
	_, effectDigest, err := replayR18Flows(withEffects)
	if err != nil {
		t.Fatalf("replay with effects: %v", err)
	}
	if effectDigest != firstDigest {
		t.Fatal("replay must not depend on effects (V3=descoped)")
	}

	t.Logf("replayed %d flows, %d steps, digest=%s", len(first), totalSteps, firstDigest)
}
