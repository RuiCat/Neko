// r18Write.go serializes the C7 artifact with the canonical encoder K1 uses, so a
// rerun produces byte-identical output and a test can compare bytes without
// normalizing formatting.
package task

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/RuiCat/Neko/internal/flow/spec"
)

// WriteR18Flows writes file to path as canonical JSON and returns nothing but an
// error: the path is the deliverable and the caller already holds it.
func WriteR18Flows(path string, file R18FlowsFile) error {
	data, err := spec.MarshalCanonical(file)
	if err != nil {
		return fmt.Errorf("encode r18 flows: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create r18 artifact directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write r18 flows: %w", err)
	}
	return nil
}
