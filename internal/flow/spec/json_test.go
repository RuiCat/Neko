// json_test.go pins canonical JSON output (sorted keys, identical bytes across
// runs) and writes the three deliverable artifacts into runs/legacy-extraction-p4.
package spec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestMarshalCanonicalSortsKeys(t *testing.T) {
	type sample struct {
		Z int            `json:"z"`
		A int            `json:"a"`
		M map[string]int `json:"m"`
	}
	value := sample{Z: 3, A: 1, M: map[string]int{"b": 20, "a": 10}}
	got, err := MarshalCanonical(value)
	if err != nil {
		t.Fatalf("MarshalCanonical: %v", err)
	}
	want := "{\n  \"a\": 1,\n  \"m\": {\n    \"a\": 10,\n    \"b\": 20\n  },\n  \"z\": 3\n}\n"
	if string(got) != want {
		t.Fatalf("canonical JSON mismatch:\n got: %q\nwant: %q", string(got), want)
	}
	assertSortedJSONKeys(t, got)
}

func TestGenerateAndWriteDeterministic(t *testing.T) {
	corpus := requireCorpus(t)
	first, err := Generate(corpus)
	if err != nil {
		t.Fatalf("first Generate: %v", err)
	}
	second, err := Generate(corpus)
	if err != nil {
		t.Fatalf("second Generate: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("two Generate calls produced different artifacts")
	}
	dirA := t.TempDir()
	dirB := t.TempDir()
	pathsA, err := WriteArtifacts(dirA, first)
	if err != nil {
		t.Fatalf("write A: %v", err)
	}
	pathsB, err := WriteArtifacts(dirB, second)
	if err != nil {
		t.Fatalf("write B: %v", err)
	}
	if len(pathsA) != 3 || len(pathsB) != 3 {
		t.Fatalf("paths A=%v B=%v", pathsA, pathsB)
	}
	for i := range pathsA {
		dataA, err := os.ReadFile(pathsA[i])
		if err != nil {
			t.Fatalf("read A: %v", err)
		}
		dataB, err := os.ReadFile(pathsB[i])
		if err != nil {
			t.Fatalf("read B: %v", err)
		}
		if !bytes.Equal(dataA, dataB) {
			t.Fatalf("%s is not byte-identical across runs", filepath.Base(pathsA[i]))
		}
		assertSortedJSONKeys(t, dataA)
	}
}

func TestWriteCorpusArtifacts(t *testing.T) {
	corpus := requireCorpus(t)
	artifacts, err := Generate(corpus)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	dir := filepath.Join(repoRoot(t), "runs", "legacy-extraction-p4")
	paths, err := WriteArtifacts(dir, artifacts)
	if err != nil {
		t.Fatalf("WriteArtifacts: %v", err)
	}
	if len(paths) != 3 {
		t.Fatalf("paths = %v", paths)
	}
	wantNames := []string{"flow-spec.json", "effect-map.json", "migration-report.json"}
	for i, path := range paths {
		if filepath.Base(path) != wantNames[i] {
			t.Fatalf("path[%d] = %s, want %s", i, path, wantNames[i])
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		assertSortedJSONKeys(t, data)
	}
	var flowFile FlowSpecFile
	decodeJSONFile(t, paths[0], &flowFile)
	if flowFile.SchemaVersion != SCHEMA_VERSION || len(flowFile.Flows) != 44 || len(flowFile.TaskTemplates) != 11 {
		t.Fatalf("flow-spec = %+v", flowFile)
	}
	for _, template := range flowFile.TaskTemplates {
		if len(template.Stages) == 0 || template.Condition.Type == "" ||
			(template.Reward.Experience == 0 && template.Reward.Affinity == 0) ||
			!hasTaskDialog(template.Dialog) {
			t.Fatalf("incomplete task template = %+v", template)
		}
	}
	var effectMap EffectMap
	decodeJSONFile(t, paths[1], &effectMap)
	if effectMap.SchemaVersion != SCHEMA_VERSION || len(effectMap.Effects) != 55 {
		t.Fatalf("effect-map = %+v", effectMap)
	}
	var report MigrationReport
	decodeJSONFile(t, paths[2], &report)
	if report.SchemaVersion != SCHEMA_VERSION || report.Count != 55 {
		t.Fatalf("migration-report = %+v", report)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func decodeJSONFile(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func assertSortedJSONKeys(t *testing.T, data []byte) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := walkSortedKeys(decoder); err != nil {
		t.Fatalf("JSON keys are not sorted: %v", err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		t.Fatalf("trailing JSON content: %v", err)
	}
}

func walkSortedKeys(decoder *json.Decoder) error {
	tok, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		keys := []string{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key is not a string")
			}
			keys = append(keys, key)
			if err := walkSortedKeys(decoder); err != nil {
				return err
			}
		}
		if _, err := decoder.Token(); err != nil {
			return err
		}
		for i := 1; i < len(keys); i++ {
			if keys[i-1] > keys[i] {
				return fmt.Errorf("keys %q then %q", keys[i-1], keys[i])
			}
		}
	case '[':
		for decoder.More() {
			if err := walkSortedKeys(decoder); err != nil {
				return err
			}
		}
		if _, err := decoder.Token(); err != nil {
			return err
		}
	}
	return nil
}
