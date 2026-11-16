package emote

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestClientTierKeyword uses the keyword tier when no vector artifact exists.
func TestClientTierKeyword(t *testing.T) {
	dir := t.TempDir()
	writeIndexFile(t, dir, []indexEntryFixture{{Name: "a.jpg", Desc: "日常早安", Categories: []string{"日常"}}})
	client := NewClient(Config{Dir: dir})
	result, err := client.Find(context.Background(), "早安", 3)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if result.Source != SOURCE_KEYWORD {
		t.Fatalf("source = %q, want %q", result.Source, SOURCE_KEYWORD)
	}
	if len(result.Results) != 1 || result.Results[0].File != "a.jpg" {
		t.Fatalf("results = %+v", result.Results)
	}
	if !result.Results[0].Missing {
		t.Fatal("missing image should be flagged")
	}
}

// TestClientTierVectors falls back to the flat vectors file.
func TestClientTierVectors(t *testing.T) {
	dir := t.TempDir()
	writeIndexFile(t, dir, []indexEntryFixture{{Name: "a.jpg", Desc: "日常早安", Categories: []string{"日常"}}})
	raw, _ := json.Marshal(map[string][]float32{"a.jpg": {1, 0}})
	if err := os.WriteFile(filepath.Join(dir, VECTORS_FILE_NAME), raw, 0o644); err != nil {
		t.Fatalf("write vectors: %v", err)
	}
	client := NewClient(Config{Dir: dir})
	client.SetEmbedder(fakeEmbedder{vector: []float32{1, 0}})
	result, err := client.Find(context.Background(), "早安", 3)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if result.Source != SOURCE_VECTORS || len(result.Results) != 1 {
		t.Fatalf("source=%q results=%+v", result.Source, result.Results)
	}
}

// TestClientTierVectorDB prefers the clustered database.
func TestClientTierVectorDB(t *testing.T) {
	dir := t.TempDir()
	names := []string{"a.jpg", "b.jpg", "c.jpg"}
	writeIndexFile(t, dir, []indexEntryFixture{
		{Name: names[0], Desc: "开心地笑", Categories: []string{"开心"}},
		{Name: names[1], Desc: "难过大哭", Categories: []string{"难过"}},
		{Name: names[2], Desc: "普通表情", Categories: []string{"日常"}},
	})
	writeDBFiles(t, dir, names,
		[]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0},
		[]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0},
		[]int64{0, 1, 2}, 4, 3)
	touchImage(t, dir, names[0])
	client := NewClient(Config{Dir: dir})
	client.SetEmbedder(fakeEmbedder{vector: []float32{1, 0, 0, 0}})
	result, err := client.Find(context.Background(), "开心", 2)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if result.Source != SOURCE_VEC_DB {
		t.Fatalf("source = %q, want %q", result.Source, SOURCE_VEC_DB)
	}
	if len(result.Results) == 0 || result.Results[0].File != names[0] {
		t.Fatalf("results = %+v", result.Results)
	}
	if result.Results[0].Missing || result.Results[0].Path == "" {
		t.Fatalf("path = %q missing=%v", result.Results[0].Path, result.Results[0].Missing)
	}
}

// TestClientMissingIndex reports ErrIndexMissing.
func TestClientMissingIndex(t *testing.T) {
	client := NewClient(Config{Dir: t.TempDir()})
	if _, err := client.Find(context.Background(), "早安", 3); err == nil {
		t.Fatal("want ErrIndexMissing")
	}
}

// TestClientReloadsAfterRebuild picks up a rewritten index.
func TestClientReloadsAfterRebuild(t *testing.T) {
	dir := t.TempDir()
	writeIndexFile(t, dir, []indexEntryFixture{{Name: "a.jpg", Desc: "日常早安", Categories: []string{"日常"}}})
	client := NewClient(Config{Dir: dir})
	if _, err := client.Find(context.Background(), "早安", 3); err != nil {
		t.Fatalf("Find: %v", err)
	}
	writeIndexFile(t, dir, []indexEntryFixture{
		{Name: "a.jpg", Desc: "日常早安", Categories: []string{"日常"}},
		{Name: "b.jpg", Desc: "日常晚安", Categories: []string{"日常"}},
	})
	time.Sleep(5 * time.Millisecond)
	data, err := client.DataSet()
	if err != nil {
		t.Fatalf("DataSet: %v", err)
	}
	if len(data.Index) != 2 {
		t.Fatalf("index = %d, want 2 after reload", len(data.Index))
	}
}

// TestClientReloadsAfterVectorsChange picks up a rewritten flat vectors file:
// the cache fingerprint must cover emote_vectors.json, not only the index/npz.
func TestClientReloadsAfterVectorsChange(t *testing.T) {
	dir := t.TempDir()
	writeIndexFile(t, dir, []indexEntryFixture{
		{Name: "a.jpg", Desc: "日常早安", Categories: []string{"日常"}},
		{Name: "b.jpg", Desc: "日常晚安", Categories: []string{"日常"}},
	})
	writeVectors := func(vectors map[string][]float32) {
		raw, err := json.Marshal(vectors)
		if err != nil {
			t.Fatalf("marshal vectors: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, VECTORS_FILE_NAME), raw, 0o644); err != nil {
			t.Fatalf("write vectors: %v", err)
		}
	}
	writeVectors(map[string][]float32{"a.jpg": {1, 0}})
	client := NewClient(Config{Dir: dir})
	client.SetEmbedder(fakeEmbedder{vector: []float32{1, 0}})
	first, err := client.Find(context.Background(), "早安", 3)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(first.Results) != 1 {
		t.Fatalf("results = %+v, want only a.jpg", first.Results)
	}
	time.Sleep(5 * time.Millisecond)
	writeVectors(map[string][]float32{"a.jpg": {1, 0}, "b.jpg": {1, 0}})
	second, err := client.Find(context.Background(), "早安", 3)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(second.Results) != 2 {
		t.Fatalf("results = %+v, want both rows after the vectors rewrite", second.Results)
	}
}

// TestClampTop checks the new top argument guard.
func TestClampTop(t *testing.T) {
	cases := map[int]int{0: DEFAULT_TOP, -5: DEFAULT_TOP, 1: 1, 3: 3, 20: 20, 999: MAX_TOP}
	for input, want := range cases {
		if got := clampTop(input); got != want {
			t.Fatalf("clampTop(%d) = %d, want %d", input, got, want)
		}
	}
}
