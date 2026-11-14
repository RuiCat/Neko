package emote

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// hashEmbedder is a deterministic test embedder with configurable dimension.
type hashEmbedder struct {
	dim   int
	calls atomic.Int64
}

// Embed implements Embedder without any network access.
func (h *hashEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	h.calls.Add(1)
	vector := make([]float32, h.dim)
	for i := 0; i < h.dim; i++ {
		vector[i] = float32((len(text)*31+i*7)%11) / 10
	}
	return vector, nil
}

// buildFixture writes an index with n rows and returns the directory.
func buildFixture(t *testing.T, n int) string {
	t.Helper()
	dir := t.TempDir()
	entries := make([]indexEntryFixture, n)
	for i := range entries {
		entries[i] = indexEntryFixture{Name: "e" + itoa(i) + ".jpg", Desc: "描述" + itoa(i), Categories: []string{"开心"}}
	}
	writeIndexFile(t, dir, entries)
	return dir
}

// itoa avoids importing strconv in fixtures.
func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := []byte{}
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}

// TestRunBuildSmall runs the full pipeline against a fake embedder.
func TestRunBuildSmall(t *testing.T) {
	dir := buildFixture(t, 3)
	embedder := &hashEmbedder{dim: 4}
	progress := 0
	report, err := RunBuild(context.Background(), BuildOptions{
		Dir: dir, Embedder: embedder, Workers: 2, Clusters: 2, Iterations: 3,
		Progress: func(done, total int) { progress = done },
	})
	if err != nil {
		t.Fatalf("RunBuild: %v", err)
	}
	if report.Rows != 3 || report.Dim != 4 || report.Clusters != 2 || report.Embedded != 3 {
		t.Fatalf("report = %+v", report)
	}
	if progress != 3 {
		t.Fatalf("progress = %d, want 3", progress)
	}
	database, err := readVecDB(filepath.Join(dir, VECTOR_DB_FILE_NAME))
	if err != nil {
		t.Fatalf("readVecDB: %v", err)
	}
	if database.Rows != 3 || database.Dim != 4 {
		t.Fatalf("database = %d/%d, want 3/4", database.Rows, database.Dim)
	}
	raw, err := os.ReadFile(filepath.Join(dir, VECTOR_META_FILE_NAME))
	if err != nil {
		t.Fatalf("read meta: %v", err)
	}
	var meta VectorMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatalf("unmarshal meta: %v", err)
	}
	if meta.Count != 3 || meta.Dim != 4 || meta.Model == "" || len(meta.IndexSha256) != 64 {
		t.Fatalf("meta = %+v", meta)
	}
}

// TestRunBuildResume skips existing vectors and keeps null rows for retry.
func TestRunBuildResume(t *testing.T) {
	dir := buildFixture(t, 3)
	existing := map[string][]float32{"e0.jpg": {1, 2, 3, 4}, "e1.jpg": nil}
	raw, err := json.Marshal(existing)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, VECTORS_FILE_NAME), raw, 0o644); err != nil {
		t.Fatalf("write vectors: %v", err)
	}
	embedder := &hashEmbedder{dim: 4}
	report, err := RunBuild(context.Background(), BuildOptions{
		Dir: dir, Embedder: embedder, Workers: 1, Clusters: 2, Iterations: 2,
	})
	if err != nil {
		t.Fatalf("RunBuild: %v", err)
	}
	if report.Embedded != 2 || report.Skipped != 1 {
		t.Fatalf("report embedded=%d skipped=%d, want 2/1", report.Embedded, report.Skipped)
	}
	if report.Rows != 3 {
		t.Fatalf("rows = %d, want 3 (null row is re-embedded)", report.Rows)
	}
}

// TestRunBuildDryRun writes nothing.
func TestRunBuildDryRun(t *testing.T) {
	dir := buildFixture(t, 2)
	report, err := RunBuild(context.Background(), BuildOptions{Dir: dir, DryRun: true, Embedder: &hashEmbedder{dim: 4}})
	if err != nil {
		t.Fatalf("RunBuild: %v", err)
	}
	if report.Entries != 2 || report.DryRun != true {
		t.Fatalf("report = %+v", report)
	}
	for _, name := range []string{VECTORS_FILE_NAME, VECTOR_DB_FILE_NAME, VECTOR_META_FILE_NAME} {
		if _, err := os.Stat(filepath.Join(dir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s written during dry run", name)
		}
	}
}

// TestRunBuildLimit caps processed entries.
func TestRunBuildLimit(t *testing.T) {
	dir := buildFixture(t, 5)
	report, err := RunBuild(context.Background(), BuildOptions{
		Dir: dir, Embedder: &hashEmbedder{dim: 4}, Limit: 2, Workers: 1, Iterations: 2,
	})
	if err != nil {
		t.Fatalf("RunBuild: %v", err)
	}
	if report.Entries != 2 || report.Rows != 2 {
		t.Fatalf("report = %+v", report)
	}
}

// TestKMeansDeterministic checks the fixed-seed reproducibility used by A6.
func TestKMeansDeterministic(t *testing.T) {
	x := []float32{
		0, 0, 0.1, 0.1, 5, 5, 5.1, 5.1, 0.2, 0, 5, 5.2,
	}
	firstCenters, firstLabels, err := kmeans(x, 6, 2, 2, 10, DEFAULT_SEED, 2)
	if err != nil {
		t.Fatalf("kmeans: %v", err)
	}
	secondCenters, secondLabels, err := kmeans(x, 6, 2, 2, 10, DEFAULT_SEED, 2)
	if err != nil {
		t.Fatalf("kmeans: %v", err)
	}
	for i := range firstCenters {
		if firstCenters[i] != secondCenters[i] {
			t.Fatalf("centers[%d] = %v vs %v", i, firstCenters[i], secondCenters[i])
		}
	}
	for i := range firstLabels {
		if firstLabels[i] != secondLabels[i] {
			t.Fatalf("labels[%d] = %d vs %d", i, firstLabels[i], secondLabels[i])
		}
	}
}

// TestAssembleMatrixRejectsMixedDimensions surfaces the R1 dimension guard.
func TestAssembleMatrixRejectsMixedDimensions(t *testing.T) {
	vectors := map[string][]float32{"a.jpg": {1, 2}, "b.jpg": {1, 2, 3}}
	if _, _, _, err := assembleMatrix(vectors, []string{"a.jpg", "b.jpg"}); !errors.Is(err, ErrVectorDimension) {
		t.Fatalf("err = %v, want ErrVectorDimension", err)
	}
}

// TestWriteIndexPreservesOrder guards the ordered decoder used by the keyword tier.
func TestWriteIndexPreservesOrder(t *testing.T) {
	dir := t.TempDir()
	entries := []indexEntryFixture{
		{Name: "z.jpg", Desc: "z"}, {Name: "a.jpg", Desc: "a"}, {Name: "m.jpg", Desc: "m"},
	}
	writeIndexFile(t, dir, entries)
	index, order, err := loadIndex(filepath.Join(dir, INDEX_FILE_NAME))
	if err != nil {
		t.Fatalf("loadIndex: %v", err)
	}
	if len(index) != 3 || strings.Join(order, ",") != "z.jpg,a.jpg,m.jpg" {
		t.Fatalf("order = %v", order)
	}
}
