// writeArtifacts.go serializes the three K1 artifacts with canonical JSON:
// object keys are sorted, arrays keep their semantic order, and repeated runs
// produce identical bytes.
package extract

import . "github.com/RuiCat/Neko/internal/flow/spec"

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
)

// MarshalCanonical encodes v with sorted object keys, two-space indentation and
// a trailing newline. Numbers keep their literal form through json.Number.
func MarshalCanonical(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var tree any
	if err := decoder.Decode(&tree); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(tree); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// WriteArtifacts writes flow-spec.json, effect-map.json and migration-report.json
// into dir and returns their paths in that order. Existing files are replaced.
func WriteArtifacts(dir string, artifacts Artifacts) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	outputs := []struct {
		name  string
		value any
	}{
		{name: "flow-spec.json", value: artifacts.Flows},
		{name: "effect-map.json", value: artifacts.Effects},
		{name: "migration-report.json", value: artifacts.Report},
	}
	paths := make([]string, 0, len(outputs))
	for _, output := range outputs {
		data, err := MarshalCanonical(output.value)
		if err != nil {
			return nil, err
		}
		path := filepath.Join(dir, output.name)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}
