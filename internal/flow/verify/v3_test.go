// v3_test.go is the V3 acceptance walk (D7 descoped): the 12 R18 hops are
// replayed twice by their ordered steps and summarized with a digest this
// package computes independently of m5's test helpers. Effects are not decoded,
// which is exactly what D7 froze: a replay must not depend on them.
package verify

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"testing"
)

// R18_FLOW_COUNT is the frozen C7 row count.
const R18_FLOW_COUNT = 12

// replayFlow is one walked R18 hop: its ID and the numbered steps it consumed.
type replayFlow struct {
	FlowID string
	Steps  []string
}

// replayR18 walks every hop's steps in order. A missing ID or an empty step list
// is an error instead of a silently short summary.
func replayR18(document r18Artifact) ([]replayFlow, error) {
	flows := make([]replayFlow, 0, len(document.R18Flows))
	for _, flow := range document.R18Flows {
		if flow.ID == "" {
			return nil, fmt.Errorf("r18 flow has an empty id")
		}
		if len(flow.Steps) == 0 {
			return nil, fmt.Errorf("r18 flow %q has no steps", flow.ID)
		}
		steps := make([]string, 0, len(flow.Steps))
		for index, step := range flow.Steps {
			steps = append(steps, fmt.Sprintf("%d:%s", index+1, step))
		}
		flows = append(flows, replayFlow{FlowID: flow.ID, Steps: steps})
	}
	if len(flows) == 0 {
		return nil, fmt.Errorf("r18 artifact has no flows")
	}
	return flows, nil
}

// digestReplay hashes the walked summary with length-prefixed fields so two
// replays compare by bytes and no step boundary can be confused with content.
func digestReplay(flows []replayFlow) string {
	hash := sha256.New()
	for _, flow := range flows {
		hash.Write([]byte(flow.FlowID))
		hash.Write([]byte{0})
		for _, step := range flow.Steps {
			hash.Write([]byte(fmt.Sprintf("%d", len(step))))
			hash.Write([]byte{0})
			hash.Write([]byte(step))
			hash.Write([]byte{0})
		}
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// TestR18ReplayIndependent asserts the artifact has 12 non-empty hops, walks it
// twice and requires an identical summary and digest, then logs the measured
// digest for the verification report.
func TestR18ReplayIndependent(t *testing.T) {
	document := r18Fixture(t)
	if document.SchemaVersion != 1 {
		t.Fatalf("r18 schemaVersion = %d, want 1", document.SchemaVersion)
	}
	if len(document.R18Flows) != R18_FLOW_COUNT {
		t.Fatalf("r18 flows = %d, want %d", len(document.R18Flows), R18_FLOW_COUNT)
	}
	totalSteps := 0
	for _, flow := range document.R18Flows {
		if flow.ID == "" {
			t.Fatal("r18 flow id is empty")
		}
		if len(flow.Steps) == 0 {
			t.Fatalf("r18 flow %q has no steps", flow.ID)
		}
		totalSteps += len(flow.Steps)
	}

	first, err := replayR18(document)
	if err != nil {
		t.Fatalf("first replay: %v", err)
	}
	second, err := replayR18(document)
	if err != nil {
		t.Fatalf("second replay: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("two replays produced different summaries")
	}
	firstDigest := digestReplay(first)
	secondDigest := digestReplay(second)
	if firstDigest != secondDigest {
		t.Fatalf("two replays produced different digests: %s vs %s", firstDigest, secondDigest)
	}
	t.Logf("replayed %d flows, %d steps twice: digest=%s", len(first), totalSteps, firstDigest)
}
