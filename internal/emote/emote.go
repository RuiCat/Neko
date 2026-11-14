// Package emote implements the cat-girl emote library: semantic lookup over a
// precomputed QQ emote index plus the offline pipeline that rebuilds its
// vectors and cluster database.
//
// The on-disk formats match the Python implementation this package replaces
// (numpy npz + JSON), so the existing data directory keeps working and either
// side can read the other's build artifacts.
package emote

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// DEFAULT_MODEL is the Ollama embedding model the current 768-dim index was
	// built with; a different model invalidates the index.
	DEFAULT_MODEL = "nomic-embed-text"
	// DEFAULT_EMBED_URL is the Ollama embeddings endpoint used by the Python
	// version (kept for byte-compatible request/response behavior).
	DEFAULT_EMBED_URL = "http://127.0.0.1:11434/api/embeddings"
	// DEFAULT_TIMEOUT bounds one embedding request.
	DEFAULT_TIMEOUT = 60 * time.Second
	// DEFAULT_TOP is the default result count.
	DEFAULT_TOP = 3
	// MIN_TOP and MAX_TOP clamp the caller's result count (new protection: the
	// Python version accepted any integer).
	MIN_TOP = 1
	MAX_TOP = 20
	// DEFAULT_CLUSTERS, DEFAULT_ITERATIONS and DEFAULT_SEED mirror
	// emote_vec_db.build_db (N_CLUSTERS=128, seed 42, at most 20 rounds).
	DEFAULT_CLUSTERS   = 128
	DEFAULT_ITERATIONS = 20
	DEFAULT_SEED       = int64(42)
	// Source names of the three lookup tiers.
	SOURCE_VEC_DB  = "vec_db"
	SOURCE_VECTORS = "vectors"
	SOURCE_KEYWORD = "keyword"
	// File names inside the emote directory.
	INDEX_FILE_NAME            = "emote_index.json"
	VECTORS_FILE_NAME          = "emote_vectors.json"
	VECTOR_DB_FILE_NAME        = "vec_db.npz"
	VECTOR_META_FILE_NAME      = "vec_db.json"
	CLUSTER_ANALYSIS_FILE_NAME = "cluster_analysis.json"
	IMAGE_DIR_NAME             = "img"
	// SIM_THRESHOLD is the minimum cosine similarity of a non-intent candidate.
	SIM_THRESHOLD = 0.35
	// SCORE_CAP bounds the intent bonus before mixing (Python: min(x, 2.5)).
	SCORE_CAP = 2.5
	// EMBED_DIM_NOMIC is the dimension of the shipped nomic-embed-text index.
	// It is only a fallback; the authoritative value comes from vec_db.json.
	EMBED_DIM_NOMIC = 768
)

var (
	// ErrIndexMissing means emote_index.json is absent or unreadable.
	ErrIndexMissing = errors.New("表情库索引缺失")
	// ErrEmbedding wraps a failed embedding request.
	ErrEmbedding = errors.New("表情向量化失败")
	// ErrVectorDimension means the query or stored vector dimension disagrees
	// with the database metadata.
	ErrVectorDimension = errors.New("表情向量维度不一致")
	// ErrVectorDBCorrupt means the npz database cannot be decoded.
	ErrVectorDBCorrupt = errors.New("表情向量库损坏")
)

// IndexEntry is one emote row in emote_index.json.
type IndexEntry struct {
	Desc       string   `json:"desc"`
	Categories []string `json:"categories"`
	Keywords   []string `json:"keywords"`
}

// VectorMeta is vec_db.json: the name list and shape metadata of vec_db.npz.
// Model/BuiltAt/IndexSha256 are emitted by the Go builder and optional when
// reading an older file.
type VectorMeta struct {
	Names       []string `json:"names"`
	NClusters   int      `json:"n_clusters"`
	Dim         int      `json:"dim"`
	Count       int      `json:"count"`
	Model       string   `json:"model,omitempty"`
	BuiltAt     string   `json:"built_at,omitempty"`
	IndexSha256 string   `json:"indexSha256,omitempty"`
}

// ClusterAnalysis is one cluster's visual analysis in cluster_analysis.json.
type ClusterAnalysis struct {
	Name     string    `json:"name"`
	Analysis string    `json:"analysis"`
	Desc     string    `json:"desc"`
	Vec      []float32 `json:"vec"`
}

// Result is one emote hit. Field names are the MCP JSON contract (decision d4).
type Result struct {
	File        string   `json:"file"`
	Desc        string   `json:"desc"`
	Score       float64  `json:"score"`
	Categories  []string `json:"categories"`
	IntentBonus float64  `json:"intent_bonus"`
	Cluster     int      `json:"cluster"`
	Path        string   `json:"path"`
	Missing     bool     `json:"missing"`
}

// FindResult is the whole answer of one lookup.
type FindResult struct {
	Source  string   `json:"source"`
	Results []Result `json:"results"`
}

// Config locates the emote data and the embedding backend.
type Config struct {
	// Dir is the emote directory (emote_index.json, vec_db.*, img/). Empty
	// resolves to DefaultDir().
	Dir string
	// Model overrides DEFAULT_MODEL.
	Model string
	// EmbedURL overrides DEFAULT_EMBED_URL.
	EmbedURL string
	// Timeout overrides DEFAULT_TIMEOUT.
	Timeout time.Duration
}

// normalized fills defaults without touching the caller's struct.
func (c Config) normalized() Config {
	if c.Dir == "" {
		c.Dir = DefaultDir()
	}
	if c.Model == "" {
		c.Model = DEFAULT_MODEL
	}
	if c.EmbedURL == "" {
		c.EmbedURL = DEFAULT_EMBED_URL
	}
	if c.Timeout <= 0 {
		c.Timeout = DEFAULT_TIMEOUT
	}
	return c
}

// Embedder turns query text into a vector.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// DefaultDir resolves the emote directory: ASTRBOX_EMOTE_DIR, else the Python
// layout ~/data/catworkspace/emotes.
func DefaultDir() string {
	if value := os.Getenv("ASTRBOX_EMOTE_DIR"); value != "" {
		return value
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "data", "catworkspace", "emotes")
}

// clampTop keeps the result count inside [MIN_TOP, MAX_TOP].
func clampTop(top int) int {
	if top <= 0 {
		return DEFAULT_TOP
	}
	if top < MIN_TOP {
		return MIN_TOP
	}
	if top > MAX_TOP {
		return MAX_TOP
	}
	return top
}

// truncateRunes cuts a string to at most n code points, like Python's s[:n].
func truncateRunes(value string, n int) string {
	if n <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= n {
		return value
	}
	return string(runes[:n])
}

// roundScore mirrors Python's round(score, 4).
func roundScore(score float64) float64 {
	return math.Round(score*10000) / 10000
}

// containsAny reports whether text contains one of the keywords.
func containsAny(text string, keywords []string) bool {
	for _, keyword := range keywords {
		if keyword != "" && strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}
