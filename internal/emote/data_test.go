package emote

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// indexEntryFixture describes one emote_index.json row for tests.
type indexEntryFixture struct {
	Name       string
	Desc       string
	Categories []string
	Keywords   []string
}

// writeIndexFile writes emote_index.json preserving the given order.
func writeIndexFile(t *testing.T, dir string, entries []indexEntryFixture) {
	t.Helper()
	var builder strings.Builder
	builder.WriteString("{")
	for i, entry := range entries {
		if i > 0 {
			builder.WriteString(",")
		}
		raw, err := json.Marshal(IndexEntry{Desc: entry.Desc, Categories: entry.Categories, Keywords: entry.Keywords})
		if err != nil {
			t.Fatalf("marshal index entry: %v", err)
		}
		key, err := json.Marshal(entry.Name)
		if err != nil {
			t.Fatalf("marshal index key: %v", err)
		}
		builder.Write(key)
		builder.WriteString(":")
		builder.Write(raw)
	}
	builder.WriteString("}")
	if err := os.WriteFile(filepath.Join(dir, INDEX_FILE_NAME), []byte(builder.String()), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
}

// writeDBFiles writes vec_db.npz and vec_db.json.
func writeDBFiles(t *testing.T, dir string, names []string, x, centers []float32, labels []int64, dim, clusters int) {
	t.Helper()
	if err := writeVecDB(filepath.Join(dir, VECTOR_DB_FILE_NAME), x, centers, labels, len(labels), dim, clusters); err != nil {
		t.Fatalf("writeVecDB: %v", err)
	}
	meta := VectorMeta{Names: names, NClusters: clusters, Dim: dim, Count: len(names)}
	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, VECTOR_META_FILE_NAME), raw, 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}
}

// writeClusterFile writes cluster_analysis.json.
func writeClusterFile(t *testing.T, dir string, clusters map[string]ClusterAnalysis) {
	t.Helper()
	raw, err := json.Marshal(clusters)
	if err != nil {
		t.Fatalf("marshal clusters: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, CLUSTER_ANALYSIS_FILE_NAME), raw, 0o644); err != nil {
		t.Fatalf("write clusters: %v", err)
	}
}

// touchImage creates an empty image file in <dir>/img.
func touchImage(t *testing.T, dir, name string) {
	t.Helper()
	imgDir := filepath.Join(dir, IMAGE_DIR_NAME)
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		t.Fatalf("mkdir img: %v", err)
	}
	if err := os.WriteFile(filepath.Join(imgDir, name), []byte("img"), 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}
}

// TestLoadDataSet loads index, meta, database, clusters and resolves paths.
func TestLoadDataSet(t *testing.T) {
	dir := t.TempDir()
	writeIndexFile(t, dir, []indexEntryFixture{
		{Name: "a.jpg", Desc: "第一张", Categories: []string{"开心"}},
		{Name: "b.jpg", Desc: "第二张", Categories: []string{"难过"}},
		{Name: "c.jpg", Desc: "第三张", Categories: nil},
	})
	writeDBFiles(t, dir, []string{"a.jpg", "b.jpg", "c.jpg"},
		[]float32{1, 0, 0, 0, 0, 1, 0, 0, 1, 1, 0, 0},
		[]float32{1, 0, 0, 0, 0, 1, 0, 0},
		[]int64{0, 1, 0}, 4, 2)
	writeClusterFile(t, dir, map[string]ClusterAnalysis{"0": {Analysis: "开心场景"}, "1": {Analysis: "难过场景"}})
	touchImage(t, dir, "a.jpg")
	touchImage(t, dir, "c.png")

	data, err := loadDataSet(dir)
	if err != nil {
		t.Fatalf("loadDataSet: %v", err)
	}
	if len(data.Index) != 3 || len(data.IndexOrder) != 3 || data.IndexOrder[0] != "a.jpg" {
		t.Fatalf("index order = %v", data.IndexOrder)
	}
	if !data.HasDB || data.Dim != 4 || len(data.Labels) != 3 {
		t.Fatalf("db = %v dim=%d labels=%d", data.HasDB, data.Dim, len(data.Labels))
	}
	if data.analysisFor(1) != "难过场景" {
		t.Fatalf("analysisFor(1) = %q", data.analysisFor(1))
	}
	if path, missing := data.pathFor("a.jpg"); missing || path == "" {
		t.Fatalf("pathFor(a.jpg) = %q missing=%v", path, missing)
	}
	// c.jpg is missing but c.png exists: the extension fallback applies.
	if path, missing := data.pathFor("c.jpg"); missing || !strings.HasSuffix(path, "c.png") {
		t.Fatalf("pathFor(c.jpg) = %q missing=%v", path, missing)
	}
	if _, missing := data.pathFor("zzz.jpg"); !missing {
		t.Fatal("pathFor(zzz.jpg) should be missing")
	}
}

// TestLoadDataSetMissingIndex returns ErrIndexMissing.
func TestLoadDataSetMissingIndex(t *testing.T) {
	_, err := loadDataSet(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), ErrIndexMissing.Error()) {
		t.Fatalf("err = %v, want ErrIndexMissing", err)
	}
}

// TestLoadDataSetMetaMismatch rejects a metadata/npz dimension conflict.
func TestLoadDataSetMetaMismatch(t *testing.T) {
	dir := t.TempDir()
	writeIndexFile(t, dir, []indexEntryFixture{{Name: "a.jpg", Desc: "x"}})
	writeDBFiles(t, dir, []string{"a.jpg"}, []float32{1, 0, 0, 0}, []float32{1, 0, 0, 0}, []int64{0}, 4, 1)
	raw, err := os.ReadFile(filepath.Join(dir, VECTOR_META_FILE_NAME))
	if err != nil {
		t.Fatalf("read meta: %v", err)
	}
	var meta VectorMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatalf("unmarshal meta: %v", err)
	}
	meta.Dim = 8
	raw, _ = json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(dir, VECTOR_META_FILE_NAME), raw, 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	if _, err := loadDataSet(dir); err == nil || !strings.Contains(err.Error(), ErrVectorDimension.Error()) {
		t.Fatalf("err = %v, want ErrVectorDimension", err)
	}
}

// TestLoadVectorsLazy reads emote_vectors.json only on demand.
func TestLoadVectorsLazy(t *testing.T) {
	dir := t.TempDir()
	writeIndexFile(t, dir, []indexEntryFixture{{Name: "a.jpg", Desc: "x", Categories: []string{"开心"}}})
	vectors := map[string][]float32{"a.jpg": {1, 0}}
	raw, _ := json.Marshal(vectors)
	if err := os.WriteFile(filepath.Join(dir, VECTORS_FILE_NAME), raw, 0o644); err != nil {
		t.Fatalf("write vectors: %v", err)
	}
	data, err := loadDataSet(dir)
	if err != nil {
		t.Fatalf("loadDataSet: %v", err)
	}
	if data.vectors != nil {
		t.Fatal("vectors loaded eagerly")
	}
	if _, ok := data.loadVectors(); !ok {
		t.Fatal("loadVectors failed")
	}
	if got := data.vectors["a.jpg"]; len(got) != 2 || got[0] != 1 {
		t.Fatalf("vectors[a] = %v", got)
	}
}
