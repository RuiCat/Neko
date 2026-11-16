// build.go rebuilds the vector tier: descriptions → nomic embeddings
// (resumable, concurrent) and vectors → KMeans clusters → vec_db.npz.
// The npz writer is numpy compatible, so Python can still np.load the result.
package emote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SNAPSHOT_EVERY mirrors build_emote_vectors.py's save-every-100-completions.
const SNAPSHOT_EVERY = 100

// DEFAULT_WORKERS bounds concurrent embedding requests.
const DEFAULT_WORKERS = 4

// BuildOptions configures one rebuild run.
type BuildOptions struct {
	// Dir is the input directory holding emote_index.json.
	Dir string
	// OutDir receives emote_vectors.json, vec_db.npz and vec_db.json; empty
	// means Dir.
	OutDir string
	// Model and EmbedURL configure the default Ollama embedder.
	Model    string
	EmbedURL string
	Timeout  time.Duration
	// Workers bounds concurrent embedding requests (default DEFAULT_WORKERS).
	Workers int
	// Limit caps the number of index entries (0 = all); useful for smoke tests.
	Limit int
	// DryRun only reports the plan, without writing anything.
	DryRun bool
	// Clusters, Iterations and Seed configure KMeans (numpy defaults).
	Clusters   int
	Iterations int
	Seed       int64
	// Embedder overrides the Ollama client (tests).
	Embedder Embedder
	// Progress receives (done, total) after every snapshot; optional.
	Progress func(done, total int)
}

// BuildReport summarizes what a run did.
type BuildReport struct {
	Entries     int    `json:"entries"`
	Embedded    int    `json:"embedded"`
	Skipped     int    `json:"skipped"`
	Failed      int    `json:"failed"`
	Rows        int    `json:"rows"`
	Dim         int    `json:"dim"`
	Clusters    int    `json:"clusters"`
	VectorsPath string `json:"vectors_path"`
	DBPath      string `json:"db_path"`
	MetaPath    string `json:"meta_path"`
	DryRun      bool   `json:"dry_run"`
}

// normalized fills the build defaults.
func (o BuildOptions) normalized() BuildOptions {
	if o.OutDir == "" {
		o.OutDir = o.Dir
	}
	if o.Model == "" {
		o.Model = DEFAULT_MODEL
	}
	if o.EmbedURL == "" {
		o.EmbedURL = DEFAULT_EMBED_URL
	}
	if o.Timeout <= 0 {
		o.Timeout = DEFAULT_TIMEOUT
	}
	if o.Workers <= 0 {
		o.Workers = DEFAULT_WORKERS
	}
	if o.Clusters <= 0 {
		o.Clusters = DEFAULT_CLUSTERS
	}
	if o.Iterations <= 0 {
		o.Iterations = DEFAULT_ITERATIONS
	}
	if o.Seed == 0 {
		o.Seed = DEFAULT_SEED
	}
	return o
}

// RunBuild executes the two offline steps and returns a report.
func RunBuild(ctx context.Context, options BuildOptions) (BuildReport, error) {
	options = options.normalized()
	report := BuildReport{
		VectorsPath: filepath.Join(options.OutDir, VECTORS_FILE_NAME),
		DBPath:      filepath.Join(options.OutDir, VECTOR_DB_FILE_NAME),
		MetaPath:    filepath.Join(options.OutDir, VECTOR_META_FILE_NAME),
		DryRun:      options.DryRun,
	}
	index, order, err := loadIndex(filepath.Join(options.Dir, INDEX_FILE_NAME))
	if err != nil {
		return report, err
	}
	if options.Limit > 0 && options.Limit < len(order) {
		order = order[:options.Limit]
	}
	report.Entries = len(order)
	if options.DryRun {
		return report, nil
	}
	if err := os.MkdirAll(options.OutDir, 0o755); err != nil {
		return report, err
	}
	vectors, embedded, skipped, failed, err := buildVectors(ctx, options, index, order)
	if err != nil {
		return report, err
	}
	report.Embedded, report.Skipped, report.Failed = embedded, skipped, failed
	names, x, dim, err := assembleMatrix(vectors, order)
	if err != nil {
		return report, err
	}
	if len(names) == 0 {
		return report, fmt.Errorf("没有可用向量（嵌入全部失败？）")
	}
	report.Rows, report.Dim = len(names), dim
	clusters := options.Clusters
	if clusters > len(names) {
		clusters = len(names)
	}
	centers, labels, err := kmeans(x, len(names), dim, clusters, options.Iterations, options.Seed, options.Workers)
	if err != nil {
		return report, err
	}
	report.Clusters = clusters
	if err := writeVecDB(report.DBPath, x, centers, labels, len(names), dim, clusters); err != nil {
		return report, fmt.Errorf("写入 %s 失败: %w", VECTOR_DB_FILE_NAME, err)
	}
	if err := writeVectorMeta(report.MetaPath, options, names, dim, clusters); err != nil {
		return report, err
	}
	return report, nil
}

// buildVectors embeds every description, resuming from an existing file.
func buildVectors(ctx context.Context, options BuildOptions, index map[string]IndexEntry, order []string) (map[string][]float32, int, int, int, error) {
	vectors := map[string][]float32{}
	if existing, err := readVectorsFile(filepath.Join(options.OutDir, VECTORS_FILE_NAME)); err == nil {
		for name, vector := range existing {
			vectors[name] = vector
		}
	}
	embedder := options.Embedder
	if embedder == nil {
		embedder = NewOllamaEmbedder(options.EmbedURL, options.Model, options.Timeout)
	}
	pending := make([]string, 0, len(order))
	skipped := 0
	for _, name := range order {
		if vector, ok := vectors[name]; ok && len(vector) > 0 {
			skipped++
			continue
		}
		pending = append(pending, name)
	}
	if len(pending) == 0 {
		return vectors, 0, skipped, 0, nil
	}
	type outcome struct {
		name   string
		vector []float32
		failed bool
	}
	jobs := make(chan string)
	results := make(chan outcome, options.Workers)
	var workers sync.WaitGroup
	for worker := 0; worker < options.Workers; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for name := range jobs {
				vector, err := embedder.Embed(ctx, index[name].Desc)
				if err != nil || len(vector) == 0 {
					results <- outcome{name: name, failed: true}
					continue
				}
				results <- outcome{name: name, vector: vector}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, name := range pending {
			select {
			case jobs <- name:
			case <-ctx.Done():
				return
			}
		}
	}()
	// results is closed only after every worker returned, so the drain loop
	// below always terminates: a cancelled context stops the producer early,
	// and a failed snapshot keeps draining instead of blocking the workers.
	go func() {
		workers.Wait()
		close(results)
	}()
	embedded, failed, done := 0, 0, 0
	var writeErr error
	for result := range results {
		done++
		if result.failed {
			failed++
			vectors[result.name] = nil
		} else {
			embedded++
			vectors[result.name] = result.vector
		}
		if writeErr == nil && done%SNAPSHOT_EVERY == 0 {
			writeErr = writeVectorsFile(options.vectorsPath(), vectors)
			if options.Progress != nil {
				options.Progress(done, len(pending))
			}
		}
	}
	if writeErr != nil {
		return vectors, embedded, skipped, failed, writeErr
	}
	if err := ctx.Err(); err != nil && done < len(pending) {
		return vectors, embedded, skipped, failed, err
	}
	if err := writeVectorsFile(options.vectorsPath(), vectors); err != nil {
		return vectors, embedded, skipped, failed, err
	}
	if options.Progress != nil {
		options.Progress(done, len(pending))
	}
	return vectors, embedded, skipped, failed, nil
}

// vectorsPath returns the vectors file path inside OutDir.
func (o BuildOptions) vectorsPath() string {
	return filepath.Join(o.OutDir, VECTORS_FILE_NAME)
}

// readVectorsFile decodes an existing vectors file (resume input).
func readVectorsFile(path string) (map[string][]float32, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	vectors := map[string][]float32{}
	if err := json.Unmarshal(raw, &vectors); err != nil {
		return nil, err
	}
	return vectors, nil
}

// writeVectorsFile writes the vectors JSON through a temp file + rename.
func writeVectorsFile(path string, vectors map[string][]float32) error {
	raw, err := json.Marshal(vectors)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, raw)
}

// assembleMatrix turns the vector map into a row-major matrix in index order.
func assembleMatrix(vectors map[string][]float32, order []string) ([]string, []float32, int, error) {
	names := make([]string, 0, len(order))
	dim := 0
	for _, name := range order {
		vector := vectors[name]
		if len(vector) == 0 {
			continue
		}
		if dim == 0 {
			dim = len(vector)
		}
		if len(vector) != dim {
			return nil, nil, 0, fmt.Errorf("%w: %s 为 %d 维, 期望 %d 维", ErrVectorDimension, name, len(vector), dim)
		}
		names = append(names, name)
	}
	if dim == 0 {
		return names, nil, 0, nil
	}
	x := make([]float32, 0, len(names)*dim)
	for _, name := range names {
		x = append(x, vectors[name]...)
	}
	return names, x, dim, nil
}

// writeVectorMeta writes vec_db.json with the build provenance fields.
func writeVectorMeta(path string, options BuildOptions, names []string, dim, clusters int) error {
	digest, err := sha256File(filepath.Join(options.Dir, INDEX_FILE_NAME))
	if err != nil {
		return err
	}
	meta := VectorMeta{
		Names:       names,
		NClusters:   clusters,
		Dim:         dim,
		Count:       len(names),
		Model:       options.Model,
		BuiltAt:     time.Now().UTC().Format(time.RFC3339),
		IndexSha256: digest,
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, raw)
}

// sha256File returns the hex sha256 of a file.
func sha256File(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// writeFileAtomic writes data to path via a sibling temp file.
func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// kmeans mirrors emote_vec_db.build_db: random distinct initialization, at
// most iters assignment/update rounds, stop when no label moved.
func kmeans(x []float32, rows, dim, clusters, iters int, seed int64, workers int) ([]float32, []int64, error) {
	if rows == 0 || dim == 0 {
		return nil, nil, fmt.Errorf("向量矩阵为空")
	}
	if clusters <= 0 {
		return nil, nil, fmt.Errorf("簇数必须大于 0")
	}
	if clusters > rows {
		clusters = rows
	}
	rng := rand.New(rand.NewSource(seed))
	permutation := rng.Perm(rows)
	centers := make([]float32, clusters*dim)
	for cluster := 0; cluster < clusters; cluster++ {
		copy(centers[cluster*dim:(cluster+1)*dim], x[permutation[cluster]*dim:(permutation[cluster]+1)*dim])
	}
	labels := make([]int64, rows)
	for iteration := 0; iteration < iters; iteration++ {
		updated := assignClusters(x, rows, dim, centers, clusters, workers)
		moved := 0
		for row := range updated {
			if updated[row] != labels[row] {
				moved++
				labels[row] = updated[row]
			}
		}
		updateCenters(x, rows, dim, labels, centers, clusters)
		if moved == 0 {
			break
		}
	}
	return centers, labels, nil
}

// assignClusters computes each row's nearest center (parallel, first minimum wins).
func assignClusters(x []float32, rows, dim int, centers []float32, clusters, workers int) []int64 {
	labels := make([]int64, rows)
	centerNorms := make([]float64, clusters)
	for cluster := 0; cluster < clusters; cluster++ {
		centerNorms[cluster] = dotProduct(centers[cluster*dim:(cluster+1)*dim], centers[cluster*dim:(cluster+1)*dim])
	}
	if workers <= 0 {
		workers = 1
	}
	var wait sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func(worker int) {
			defer wait.Done()
			for row := worker; row < rows; row += workers {
				vector := x[row*dim : (row+1)*dim]
				rowNorm := dotProduct(vector, vector)
				best := 0
				bestDistance := math.Inf(1)
				for cluster := 0; cluster < clusters; cluster++ {
					center := centers[cluster*dim : (cluster+1)*dim]
					distance := rowNorm + centerNorms[cluster] - 2*dotProduct(vector, center)
					if distance < bestDistance {
						bestDistance = distance
						best = cluster
					}
				}
				labels[row] = int64(best)
			}
		}(worker)
	}
	wait.Wait()
	return labels
}

// updateCenters recomputes each center as its members' mean; empty clusters
// keep their previous center (numpy behavior).
func updateCenters(x []float32, rows, dim int, labels []int64, centers []float32, clusters int) {
	sums := make([]float64, clusters*dim)
	counts := make([]int, clusters)
	for row := 0; row < rows; row++ {
		cluster := int(labels[row])
		counts[cluster]++
		base := row * dim
		for offset := 0; offset < dim; offset++ {
			sums[cluster*dim+offset] += float64(x[base+offset])
		}
	}
	for cluster := 0; cluster < clusters; cluster++ {
		if counts[cluster] == 0 {
			continue
		}
		for offset := 0; offset < dim; offset++ {
			centers[cluster*dim+offset] = float32(sums[cluster*dim+offset] / float64(counts[cluster]))
		}
	}
}
