// v4_test.go is the V4 acceptance walk: it rehashes the five corpus files from
// the migration report, reopens every record anchor, and recounts the frozen
// flow-spec settleable statistic. The checks are implemented here instead of
// calling m1's validators so the report is verified by an independent reader.
package verify

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

// Frozen counts of the V4 snapshot.
const (
	// SNAPSHOT_FILE_COUNT is the number of corpus files C1 binds.
	SNAPSHOT_FILE_COUNT = 5
	// MIGRATION_RECORD_COUNT is the measured record count (C1 replaced 197/69/26).
	MIGRATION_RECORD_COUNT = 55
	// FLOW_COUNT is the number of rows in flow-spec.json.
	FLOW_COUNT = 44
	// SETTLEABLE_FLOW_COUNT is the number of flows whose effects all have a
	// settlement target (a flow without effects is settleable).
	SETTLEABLE_FLOW_COUNT = 26
	// BLOCKED_FLOW_COUNT is the number of flows carrying at least one null target.
	BLOCKED_FLOW_COUNT = 18
)

// TestMigrationReportArtifact recomputes the per-file and combined sha256,
// rebuilds the sha256sum manifest, checks count == len(records) and reopens every
// file:line anchor to confirm it still contains its recorded key.
func TestMigrationReportArtifact(t *testing.T) {
	report := migrationReportFixture(t)
	if report.SnapshotSha256 == "" || report.SnapshotSha256Compat == "" {
		t.Fatal("migration report is missing a snapshot hash")
	}
	if len(report.SnapshotFiles) != SNAPSHOT_FILE_COUNT {
		t.Fatalf("snapshotFiles = %d, want %d", len(report.SnapshotFiles), SNAPSHOT_FILE_COUNT)
	}
	if report.Count != len(report.Records) {
		t.Fatalf("count = %d, records = %d", report.Count, len(report.Records))
	}
	if report.Count != MIGRATION_RECORD_COUNT {
		t.Fatalf("count = %d, want %d", report.Count, MIGRATION_RECORD_COUNT)
	}

	combined := sha256.New()
	var manifest strings.Builder
	lines := make(map[string][]string, len(report.SnapshotFiles))
	for _, fileHash := range report.SnapshotFiles {
		data, err := os.ReadFile(fileHash.File)
		if err != nil {
			t.Fatalf("read corpus %s: %v", fileHash.File, err)
		}
		sum := sha256.Sum256(data)
		sumHex := hex.EncodeToString(sum[:])
		if sumHex != fileHash.Sha256 {
			t.Fatalf("%s sha256 = %s, report says %s", fileHash.File, sumHex, fileHash.Sha256)
		}
		combined.Write(data)
		manifest.WriteString(sumHex)
		manifest.WriteString("  ")
		manifest.WriteString(fileHash.File)
		manifest.WriteByte('\n')
		lines[fileHash.File] = strings.Split(string(data), "\n")
	}
	if got := hex.EncodeToString(combined.Sum(nil)); got != report.SnapshotSha256 {
		t.Fatalf("snapshotSha256 = %s, recomputed %s", report.SnapshotSha256, got)
	}
	if manifest.String() != report.SnapshotSha256Compat {
		t.Fatalf("snapshot_sha256 manifest mismatch:\nwant %q\ngot  %q", report.SnapshotSha256Compat, manifest.String())
	}

	for index, record := range report.Records {
		if record.File == "" || record.Line <= 0 || strings.TrimSpace(record.Key) == "" {
			t.Fatalf("records[%d] has an empty anchor: %+v", index, record)
		}
		fileLines, ok := lines[record.File]
		if !ok {
			t.Fatalf("records[%d] %s is not in snapshotFiles", index, record.File)
		}
		if record.Line > len(fileLines) {
			t.Fatalf("records[%d] %s:%d is past the file end", index, record.File, record.Line)
		}
		if !strings.Contains(fileLines[record.Line-1], record.Key) {
			t.Fatalf("records[%d] %s:%d does not contain key %q", index, record.File, record.Line, record.Key)
		}
	}

	document := flowSpecFixture(t)
	kinds := map[string]int{}
	for _, record := range report.Records {
		kinds[record.Kind]++
	}
	for kind := range kinds {
		if kind != "flow" && kind != "r18Flow" && kind != "taskTemplate" {
			t.Fatalf("unknown record kind %q", kind)
		}
	}
	if kinds["flow"]+kinds["r18Flow"] != len(document.Flows) {
		t.Fatalf("flow+r18Flow records = %d, flow-spec flows = %d", kinds["flow"]+kinds["r18Flow"], len(document.Flows))
	}
	if kinds["taskTemplate"] != len(document.TaskTemplates) {
		t.Fatalf("taskTemplate records = %d, flow-spec templates = %d", kinds["taskTemplate"], len(document.TaskTemplates))
	}
	if kinds["r18Flow"] != R18_FLOW_COUNT {
		t.Fatalf("r18Flow records = %d, want %d", kinds["r18Flow"], R18_FLOW_COUNT)
	}
	t.Logf("report ok: %d records (%d flow, %d r18Flow, %d taskTemplate), snapshotSha256=%s",
		report.Count, kinds["flow"], kinds["r18Flow"], kinds["taskTemplate"], report.SnapshotSha256)
}

// TestSettleableStats recounts the 26/44 statistic: a flow counts as settleable
// when none of its effects has a null D6 target. The 18 blocked flows are the
// ones settlement must refuse as a whole.
func TestSettleableStats(t *testing.T) {
	document := flowSpecFixture(t)
	if len(document.Flows) != FLOW_COUNT {
		t.Fatalf("flow-spec flows = %d, want %d", len(document.Flows), FLOW_COUNT)
	}
	settleable := 0
	blocked := 0
	for _, flow := range document.Flows {
		hasNullTarget := false
		for _, effect := range flow.Effects {
			if effect.Target == nil {
				hasNullTarget = true
				break
			}
		}
		if hasNullTarget {
			blocked++
		} else {
			settleable++
		}
	}
	if settleable != SETTLEABLE_FLOW_COUNT || blocked != BLOCKED_FLOW_COUNT {
		t.Fatalf("settleable/blocked = %d/%d, want %d/%d", settleable, blocked, SETTLEABLE_FLOW_COUNT, BLOCKED_FLOW_COUNT)
	}
	t.Logf("settleable ok: %d/%d flows all-targets-mapped, %d blocked by a null target", settleable, FLOW_COUNT, blocked)
}
