// data.go loads the emote directory into a read-only DataSet and resolves image
// paths. The index keeps its JSON key order because the keyword tier's tie
// order (and the intent direct-hit scan) depends on it, exactly like the
// Python dict iteration order.
package emote

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DataSet is one loaded emote directory: index always, vector database and
// cluster analysis when present, flat vectors only on demand.
type DataSet struct {
	// Dir is the emote directory.
	Dir string
	// Index maps file name to its description/categories/keywords.
	Index map[string]IndexEntry
	// IndexOrder is emote_index.json's key order.
	IndexOrder []string
	// Meta is vec_db.json (empty when the npz database is absent).
	Meta VectorMeta
	// X, Centers and Labels are the clustered vectors (row-major N×Dim).
	X       []float32
	Centers []float32
	Labels  []int64
	// Clusters maps cluster number to its visual analysis (optional).
	Clusters map[string]ClusterAnalysis
	// ImgDir is <Dir>/img.
	ImgDir string
	// HasDB reports whether vec_db.npz was loaded.
	HasDB bool
	// Dim is the vector dimension of the database.
	Dim int

	vectors   map[string][]float32
	vectorsOK bool
}

// loadDataSet reads the index, metadata, vector database and optional cluster
// analysis from dir.
func loadDataSet(dir string) (*DataSet, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("%w: 目录未配置", ErrIndexMissing)
	}
	index, order, err := loadIndex(filepath.Join(dir, INDEX_FILE_NAME))
	if err != nil {
		return nil, err
	}
	data := &DataSet{
		Dir:        dir,
		Index:      index,
		IndexOrder: order,
		ImgDir:     filepath.Join(dir, IMAGE_DIR_NAME),
	}
	meta, err := loadVectorMeta(filepath.Join(dir, VECTOR_META_FILE_NAME))
	if err != nil {
		return nil, err
	}
	data.Meta = meta
	info, statErr := os.Stat(filepath.Join(dir, VECTOR_DB_FILE_NAME))
	if statErr == nil && info.Size() > 0 {
		if len(meta.Names) == 0 {
			return nil, fmt.Errorf("%w: 缺少 %s", ErrVectorDBCorrupt, VECTOR_META_FILE_NAME)
		}
		database, err := readVecDB(filepath.Join(dir, VECTOR_DB_FILE_NAME))
		if err != nil {
			return nil, err
		}
		if meta.Dim > 0 && meta.Dim != database.Dim {
			return nil, fmt.Errorf("%w: vec_db.json dim=%d, npz dim=%d", ErrVectorDimension, meta.Dim, database.Dim)
		}
		if meta.Count > 0 && meta.Count != database.Rows {
			return nil, fmt.Errorf("%w: vec_db.json count=%d, npz rows=%d", ErrVectorDBCorrupt, meta.Count, database.Rows)
		}
		if len(meta.Names) != database.Rows {
			return nil, fmt.Errorf("%w: names=%d, npz rows=%d", ErrVectorDBCorrupt, len(meta.Names), database.Rows)
		}
		data.X = database.X
		data.Centers = database.Centers
		data.Labels = database.Labels
		data.Dim = database.Dim
		data.HasDB = true
	}
	data.Clusters = loadClusterAnalysis(filepath.Join(dir, CLUSTER_ANALYSIS_FILE_NAME))
	return data, nil
}

// loadIndex decodes emote_index.json while preserving key order.
func loadIndex(path string) (map[string]IndexEntry, []string, error) {
	handle, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrIndexMissing, err)
	}
	defer handle.Close()
	decoder := json.NewDecoder(bufio.NewReaderSize(handle, 1<<20))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, nil, fmt.Errorf("%w: %s 不是 JSON 对象", ErrIndexMissing, path)
	}
	index := map[string]IndexEntry{}
	order := make([]string, 0, 4096)
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %v", ErrIndexMissing, err)
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, nil, fmt.Errorf("%w: 键不是字符串", ErrIndexMissing)
		}
		var entry IndexEntry
		if err := decoder.Decode(&entry); err != nil {
			return nil, nil, fmt.Errorf("%w: %s 解析失败: %v", ErrIndexMissing, key, err)
		}
		if entry.Categories == nil {
			entry.Categories = []string{}
		}
		if entry.Keywords == nil {
			entry.Keywords = []string{}
		}
		index[key] = entry
		order = append(order, key)
	}
	if _, err := decoder.Token(); err != nil {
		return nil, nil, fmt.Errorf("%w: 尾部截断", ErrIndexMissing)
	}
	return index, order, nil
}

// loadVectorMeta reads vec_db.json; a missing file yields an empty meta.
func loadVectorMeta(path string) (VectorMeta, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return VectorMeta{}, nil
		}
		return VectorMeta{}, err
	}
	var meta VectorMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return VectorMeta{}, fmt.Errorf("解析 %s 失败: %w", VECTOR_META_FILE_NAME, err)
	}
	return meta, nil
}

// loadClusterAnalysis reads the optional cluster analysis; absent or corrupt
// files disable the intent bonus without failing the lookup.
func loadClusterAnalysis(path string) map[string]ClusterAnalysis {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var clusters map[string]ClusterAnalysis
	if err := json.Unmarshal(raw, &clusters); err != nil {
		return nil
	}
	return clusters
}

// loadVectors lazily decodes emote_vectors.json for the second fallback tier.
func (d *DataSet) loadVectors() (map[string][]float32, bool) {
	if d.vectorsOK {
		return d.vectors, d.vectors != nil
	}
	d.vectorsOK = true
	handle, err := os.Open(filepath.Join(d.Dir, VECTORS_FILE_NAME))
	if err != nil {
		return nil, false
	}
	defer handle.Close()
	vectors := map[string][]float32{}
	if err := json.NewDecoder(bufio.NewReaderSize(handle, 1<<20)).Decode(&vectors); err != nil {
		return nil, false
	}
	d.vectors = vectors
	return vectors, len(vectors) > 0
}

// row returns the i-th vector of the database.
func (d *DataSet) row(i int) []float32 {
	if d.Dim <= 0 || i < 0 || (i+1)*d.Dim > len(d.X) {
		return nil
	}
	return d.X[i*d.Dim : (i+1)*d.Dim]
}

// name returns the file name of the i-th database row.
func (d *DataSet) name(i int) string {
	if i < 0 || i >= len(d.Meta.Names) {
		return ""
	}
	return d.Meta.Names[i]
}

// analysisFor returns the cluster's visual analysis text (empty when absent).
func (d *DataSet) analysisFor(cluster int) string {
	if d.Clusters == nil {
		return ""
	}
	analysis, ok := d.Clusters[strconv.Itoa(cluster)]
	if !ok {
		return ""
	}
	return analysis.Analysis
}

// pathFor resolves the image path and reports whether it is missing.
func (d *DataSet) pathFor(file string) (string, bool) {
	candidate := filepath.Join(d.ImgDir, file)
	if exists(candidate) {
		return candidate, false
	}
	base := strings.TrimSuffix(file, filepath.Ext(file))
	for _, extension := range []string{".jpg", ".gif", ".png", ".jpeg", ".webp"} {
		candidate = filepath.Join(d.ImgDir, base+extension)
		if exists(candidate) {
			return candidate, false
		}
	}
	return "", true
}

// exists reports whether the path is a regular file or directory.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// fileStamp fingerprints the data files so the client can reload after a rebuild.
type fileStamp struct {
	indexMod    int64
	indexSize   int64
	dbMod       int64
	dbSize      int64
	vectorsMod  int64
	vectorsSize int64
	anaMod      int64
	anaSize     int64
}

// stampOf reads the current fingerprint of the index, database, flat vectors
// and cluster analysis, so replacing any of them reloads the cached DataSet.
func stampOf(dir string) fileStamp {
	stamp := fileStamp{}
	if info, err := os.Stat(filepath.Join(dir, INDEX_FILE_NAME)); err == nil {
		stamp.indexMod = info.ModTime().UnixNano()
		stamp.indexSize = info.Size()
	}
	if info, err := os.Stat(filepath.Join(dir, VECTOR_DB_FILE_NAME)); err == nil {
		stamp.dbMod = info.ModTime().UnixNano()
		stamp.dbSize = info.Size()
	}
	if info, err := os.Stat(filepath.Join(dir, VECTORS_FILE_NAME)); err == nil {
		stamp.vectorsMod = info.ModTime().UnixNano()
		stamp.vectorsSize = info.Size()
	}
	if info, err := os.Stat(filepath.Join(dir, CLUSTER_ANALYSIS_FILE_NAME)); err == nil {
		stamp.anaMod = info.ModTime().UnixNano()
		stamp.anaSize = info.Size()
	}
	return stamp
}
