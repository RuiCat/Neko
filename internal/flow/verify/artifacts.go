// artifacts.go loads the frozen P4 artifacts this harness consumes. Loading is
// read-only: flow-spec, migration-report, r18-flows and probe-cases belong to
// m1/m5/m6 outputs and are never rewritten here.
package verify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/RuiCat/Neko/internal/flow/spec"
)

// Artifact paths relative to the repository root.
const (
	// FLOW_SPEC_PATH is m1's K1 artifact consumed by V1/V2/V4.
	FLOW_SPEC_PATH = "runs/legacy-extraction-p4/flow-spec.json"
	// MIGRATION_REPORT_PATH is m1's C1 artifact consumed by V4.
	MIGRATION_REPORT_PATH = "runs/legacy-extraction-p4/migration-report.json"
	// R18_FLOWS_PATH is m5's C7 artifact consumed by V3.
	R18_FLOWS_PATH = "runs/legacy-extraction-p4/r18-flows.json"
	// PROBE_CASES_PATH is this subtask's probe case file.
	PROBE_CASES_PATH = "runs/legacy-extraction-p4/probe-cases.json"
)

// r18Artifact is the minimal C7 view the replay walks. Effects are deliberately
// not decoded: V3 is descoped on effects, so a replay cannot depend on them.
type r18Artifact struct {
	SchemaVersion int          `json:"schemaVersion"`
	Corpus        string       `json:"corpus"`
	R18Flows      []r18FlowRow `json:"r18Flows"`
}

// r18FlowRow is one R18 flow row: its ID and the ordered stage beats.
type r18FlowRow struct {
	ID    string   `json:"id"`
	Steps []string `json:"steps"`
}

// probeCase is one {entity, path, expected} probe entry. Expected is decoded as
// a JSON scalar (json.Number, bool or string) so the comparison stays exact.
type probeCase struct {
	Entity   string `json:"entity"`
	Path     string `json:"path"`
	Expected any    `json:"expected"`
}

// repoRoot walks up from the test working directory to the module root.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("working directory: %w", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %q", dir)
		}
		dir = parent
	}
}

// readArtifact decodes one artifact relative to the repository root.
func readArtifact(relativePath string, target any) error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(filepath.Join(root, relativePath))
	if err != nil {
		return fmt.Errorf("read %s: %w", relativePath, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", relativePath, err)
	}
	return nil
}

// loadFlowSpec reads m1's K1 artifact.
func loadFlowSpec() (spec.FlowSpecFile, error) {
	var document spec.FlowSpecFile
	if err := readArtifact(FLOW_SPEC_PATH, &document); err != nil {
		return spec.FlowSpecFile{}, err
	}
	return document, nil
}

// loadMigrationReport reads m1's C1 artifact.
func loadMigrationReport() (spec.MigrationReport, error) {
	var report spec.MigrationReport
	if err := readArtifact(MIGRATION_REPORT_PATH, &report); err != nil {
		return spec.MigrationReport{}, err
	}
	return report, nil
}

// loadR18Artifact reads m5's C7 artifact into the minimal replay view.
func loadR18Artifact() (r18Artifact, error) {
	var document r18Artifact
	if err := readArtifact(R18_FLOWS_PATH, &document); err != nil {
		return r18Artifact{}, err
	}
	return document, nil
}

// loadProbeCases reads this subtask's probe case list.
func loadProbeCases() ([]probeCase, error) {
	var cases []probeCase
	if err := readArtifact(PROBE_CASES_PATH, &cases); err != nil {
		return nil, err
	}
	return cases, nil
}
