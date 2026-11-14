package emote

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
)

// fakeEmbedder returns a fixed vector or error.
type fakeEmbedder struct {
	vector []float32
	err    error
}

// Embed implements Embedder.
func (f fakeEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.vector, nil
}

// syntheticDataSet builds a six-row dataset with three clusters:
// cluster0 = a,b; cluster1 = c,d; cluster2 = e,f.
func syntheticDataSet() *DataSet {
	index := map[string]IndexEntry{
		"a.jpg": {Desc: "开心地笑", Categories: []string{"开心"}},
		"b.jpg": {Desc: "双手抱头", Categories: []string{"惊讶"}},
		"c.jpg": {Desc: "紧紧拥抱", Categories: []string{"抱抱"}},
		"d.jpg": {Desc: "抱着枕头", Categories: []string{"抱抱"}},
		"e.jpg": {Desc: "裸体的恐怖场景", Categories: []string{"害怕"}},
		"f.jpg": {Desc: "亲吻", Categories: []string{"暧昧", "抱抱"}},
	}
	order := []string{"a.jpg", "b.jpg", "c.jpg", "d.jpg", "e.jpg", "f.jpg"}
	x := []float32{
		0.99, 0.1, 0, 0,
		0.8, 0.6, 0, 0,
		0.1, 0.99, 0, 0,
		0.2, 0.98, 0, 0,
		0.1, 0, 0.99, 0,
		0.2, 0.9, 0.4, 0,
	}
	centers := []float32{
		1, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 1, 0,
	}
	return &DataSet{
		Index:      index,
		IndexOrder: order,
		Meta:       VectorMeta{Names: order, NClusters: 3, Dim: 4, Count: len(order)},
		X:          x,
		Centers:    centers,
		Labels:     []int64{0, 0, 1, 1, 2, 2},
		Clusters: map[string]ClusterAnalysis{
			"0": {Analysis: "开心场景"},
			"1": {Analysis: "拥抱场景"},
			"2": {Analysis: "恐怖场景"},
		},
		HasDB: true,
		Dim:   4,
	}
}

// TestSearchVecDBFiltersR18AndScores checks the R18 filter and score ordering.
func TestSearchVecDBFiltersR18AndScores(t *testing.T) {
	data := syntheticDataSet()
	results, err := searchVecDB(context.Background(), data, fakeEmbedder{vector: []float32{0, 0, 1, 0}}, "害怕", 3)
	if err != nil {
		t.Fatalf("searchVecDB: %v", err)
	}
	for _, result := range results {
		if result.File == "e.jpg" {
			t.Fatalf("R18 row leaked: %+v", result)
		}
	}
	if len(results) == 0 {
		t.Fatal("no results, want at least the intent hit")
	}
	for i := 1; i < len(results); i++ {
		if results[i-1].Score < results[i].Score {
			t.Fatalf("results not sorted: %v", results)
		}
	}
}

// directHitDataSet builds 25 single-row clusters around a circle. With a query
// vector of [1,0,0,0] the top-20 cluster pool drops the five farthest rows
// (11..13 among them); row 12 carries an intent word so the direct-hit channel
// must pull it back in.
func directHitDataSet() *DataSet {
	const rows = 25
	const dim = 4
	names := make([]string, rows)
	index := map[string]IndexEntry{}
	x := make([]float32, 0, rows*dim)
	centers := make([]float32, 0, rows*dim)
	labels := make([]int64, rows)
	for i := 0; i < rows; i++ {
		angle := 2 * math.Pi * float64(i) / rows
		vector := []float32{float32(math.Cos(angle)), float32(math.Sin(angle)), 0, 0}
		names[i] = fmt.Sprintf("r%02d.jpg", i)
		x = append(x, vector...)
		centers = append(centers, vector...)
		labels[i] = int64(i)
		desc := "普通表情"
		if i == 12 {
			desc = "紧紧拥抱"
		}
		index[names[i]] = IndexEntry{Desc: desc, Categories: []string{"抱抱"}}
	}
	return &DataSet{
		Index:      index,
		IndexOrder: names,
		Meta:       VectorMeta{Names: names, NClusters: rows, Dim: dim, Count: rows},
		X:          x,
		Centers:    centers,
		Labels:     labels,
		HasDB:      true,
		Dim:        dim,
	}
}

// TestSearchVecDBIntentHitBeatsThreshold verifies intent direct hits are exempt
// from the 0.35 similarity threshold and come from the direct-hit channel.
func TestSearchVecDBIntentHitBeatsThreshold(t *testing.T) {
	data := directHitDataSet()
	results, err := searchVecDB(context.Background(), data, fakeEmbedder{vector: []float32{1, 0, 0, 0}}, "抱抱", 25)
	if err != nil {
		t.Fatalf("searchVecDB: %v", err)
	}
	found := false
	for _, result := range results {
		if result.File == "r12.jpg" {
			found = true
			if result.IntentBonus <= 0 {
				t.Fatalf("intent bonus = %v, want > 0", result.IntentBonus)
			}
		}
	}
	if !found {
		t.Fatalf("r12.jpg should be an intent direct hit, got %+v", results)
	}
}

// TestSearchVecDBThresholdDropsWeakCandidates verifies empty output when every
// candidate is below the threshold and no intent matches.
func TestSearchVecDBThresholdDropsWeakCandidates(t *testing.T) {
	data := syntheticDataSet()
	results, err := searchVecDB(context.Background(), data, fakeEmbedder{vector: []float32{0, 0, 0, 1}}, "zzz", 3)
	if err != nil {
		t.Fatalf("searchVecDB: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("results = %+v, want none", results)
	}
}

// TestSearchVecDBEmbeddingError surfaces embedding failures (decision d3).
func TestSearchVecDBEmbeddingError(t *testing.T) {
	data := syntheticDataSet()
	sentinel := errors.New("ollama down")
	_, err := searchVecDB(context.Background(), data, fakeEmbedder{err: sentinel}, "开心", 3)
	if err == nil || !errors.Is(err, ErrEmbedding) {
		t.Fatalf("err = %v, want ErrEmbedding", err)
	}
}

// TestSearchVecDBDimensionMismatch rejects a model with the wrong width.
func TestSearchVecDBDimensionMismatch(t *testing.T) {
	data := syntheticDataSet()
	_, err := searchVecDB(context.Background(), data, fakeEmbedder{vector: []float32{1, 2, 3}}, "开心", 3)
	if err == nil || !errors.Is(err, ErrVectorDimension) {
		t.Fatalf("err = %v, want ErrVectorDimension", err)
	}
}

// TestDetectIntentsHonorsNegatives checks query-side disambiguation.
func TestDetectIntentsHonorsNegatives(t *testing.T) {
	if got := detectIntents("抱头"); len(got) != 0 {
		t.Fatalf("detectIntents(抱头) = %v, want none (negative scenario)", got)
	}
	if got := detectIntents("今天好开心"); len(got) == 0 || got[0] != "开心" {
		t.Fatalf("detectIntents(今天好开心) = %v", got)
	}
}

// TestNearestClustersOrder checks descending cosine order.
func TestNearestClustersOrder(t *testing.T) {
	centers := []float32{1, 0, 0, 1, 0, 0}
	top := nearestClusters([]float32{0, 1}, centers, 2, 2)
	if len(top) != 2 || top[0] != 1 {
		t.Fatalf("nearestClusters = %v, want [1 0]", top)
	}
}

// TestSearchFlatVectorsFiltersIntimate checks the second tier's 暧昧 gate.
func TestSearchFlatVectorsFiltersIntimate(t *testing.T) {
	data := syntheticDataSet()
	data.HasDB = false
	data.vectors = map[string][]float32{
		"a.jpg": {1, 0, 0, 0},
		"f.jpg": {0.9, 0.1, 0, 0},
	}
	data.vectorsOK = true
	results, err := searchFlatVectors(context.Background(), data, fakeEmbedder{vector: []float32{1, 0, 0, 0}}, "早安", 5)
	if err != nil {
		t.Fatalf("searchFlatVectors: %v", err)
	}
	if len(results) != 1 || results[0].File != "a.jpg" {
		t.Fatalf("results = %+v, want only a.jpg (暧昧 filtered)", results)
	}
}
