// client.go is the lookup entry point: it caches the loaded data directory
// (reloading after a rebuild changes the index or the npz file), selects the
// three tiers in the same order as the Python tool and resolves image paths.
package emote

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Client is a cached front end over one emote directory.
type Client struct {
	cfg      Config
	embedder Embedder

	mu     sync.Mutex
	data   *DataSet
	stamp  fileStamp
	loaded bool
}

// NewClient builds a client with defaults applied.
func NewClient(cfg Config) *Client {
	return &Client{cfg: cfg.normalized()}
}

// Dir returns the emote directory the client points at.
func (c *Client) Dir() string {
	return c.cfg.Dir
}

// SetEmbedder injects an embedder (tests and custom backends).
func (c *Client) SetEmbedder(embedder Embedder) {
	c.embedder = embedder
}

// embedderOf returns the injected embedder or the Ollama default.
func (c *Client) embedderOf() Embedder {
	if c.embedder != nil {
		return c.embedder
	}
	return NewOllamaEmbedder(c.cfg.EmbedURL, c.cfg.Model, c.cfg.Timeout)
}

// DataSet returns the loaded data, reloading when the files changed on disk.
func (c *Client) DataSet() (*DataSet, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	stamp := stampOf(c.cfg.Dir)
	if c.loaded && stamp == c.stamp {
		return c.data, nil
	}
	data, err := loadDataSet(c.cfg.Dir)
	if err != nil {
		return nil, err
	}
	c.data = data
	c.stamp = stamp
	c.loaded = true
	return data, nil
}

// Find runs one lookup through the tier chain.
func (c *Client) Find(ctx context.Context, text string, top int) (*FindResult, error) {
	top = clampTop(top)
	data, err := c.DataSet()
	if err != nil {
		return nil, err
	}
	embedder := c.embedderOf()
	if data.HasDB {
		results, err := searchVecDB(ctx, data, embedder, text, top)
		if err != nil {
			return nil, err
		}
		return &FindResult{Source: SOURCE_VEC_DB, Results: results}, nil
	}
	if data.hasVectorsFile() {
		results, err := searchFlatVectors(ctx, data, embedder, text, top)
		if err != nil {
			return nil, err
		}
		return &FindResult{Source: SOURCE_VECTORS, Results: results}, nil
	}
	results := searchKeyword(data, text, top)
	return &FindResult{Source: SOURCE_KEYWORD, Results: results}, nil
}

// hasVectorsFile reports whether emote_vectors.json exists.
func (d *DataSet) hasVectorsFile() bool {
	info, err := os.Stat(filepath.Join(d.Dir, VECTORS_FILE_NAME))
	return err == nil && info.Size() > 0
}

// Validate checks the directory can answer a lookup without touching Ollama.
func (c *Client) Validate() error {
	data, err := c.DataSet()
	if err != nil {
		return err
	}
	if data.HasDB || data.hasVectorsFile() || len(data.Index) > 0 {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrIndexMissing, c.cfg.Dir)
}

// Summary describes the loaded data for CLI output.
func (c *Client) Summary() (string, error) {
	data, err := c.DataSet()
	if err != nil {
		return "", err
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "目录: %s\n", c.cfg.Dir)
	fmt.Fprintf(&builder, "索引: %d 条\n", len(data.Index))
	if data.HasDB {
		clusters := 0
		if data.Dim > 0 {
			clusters = len(data.Centers) / data.Dim
		}
		fmt.Fprintf(&builder, "向量库: %d 行 × %d 维, %d 簇\n", len(data.Labels), data.Dim, clusters)
	} else {
		builder.WriteString("向量库: 未加载\n")
	}
	fmt.Fprintf(&builder, "簇分析: %d 簇\n", len(data.Clusters))
	fmt.Fprintf(&builder, "图片目录: %s\n", data.ImgDir)
	return builder.String(), nil
}
