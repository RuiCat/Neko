// migration.go assembles the C2 EffectMap and the C1 MigrationReport from
// per-file extraction results, including the five-file sha256 snapshot that
// binds the report to the read-only corpus.
package extract

import . "github.com/RuiCat/Neko/internal/flow/spec"

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
)

func buildEffectMap(flows []FlowSpec, unclassified []UnclassifiedEffect) EffectMap {
	effects := []EffectSpec{}
	for _, flow := range flows {
		effects = append(effects, flow.Effects...)
	}
	if unclassified == nil {
		unclassified = []UnclassifiedEffect{}
	}
	return EffectMap{
		SchemaVersion:     SCHEMA_VERSION,
		Effects:           effects,
		Unclassified:      len(unclassified),
		UnclassifiedItems: unclassified,
	}
}

func buildMigrationReport(
	hashes []FileHash,
	combined string,
	manifest string,
	records []MigrationRecord,
	containers []Container,
	skipped []SkippedBlock,
	issues []ParseIssue,
) MigrationReport {
	return MigrationReport{
		SchemaVersion:        SCHEMA_VERSION,
		SnapshotSha256:       combined,
		SnapshotSha256Compat: manifest,
		SnapshotFiles:        hashes,
		Count:                len(records),
		Records:              records,
		ContainerCount:       len(containers),
		Containers:           containers,
		SkippedCount:         len(skipped),
		Skipped:              skipped,
		ErrorCount:           len(issues),
		Errors:               issues,
	}
}

// snapshotOf hashes the corpus files in order. It returns per-file hashes, the
// sha256 of the concatenated bytes, and the exact GNU sha256sum manifest text
// (two spaces between hash and path) used by the C1 judgment field.
func snapshotOf(files []string) ([]FileHash, string, string, error) {
	hashes := make([]FileHash, 0, len(files))
	combined := sha256.New()
	var manifest strings.Builder
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, "", "", err
		}
		sum := sha256.Sum256(data)
		sumHex := hex.EncodeToString(sum[:])
		hashes = append(hashes, FileHash{File: file, Sha256: sumHex})
		combined.Write(data)
		manifest.WriteString(sumHex)
		manifest.WriteString("  ")
		manifest.WriteString(file)
		manifest.WriteByte('\n')
	}
	return hashes, hex.EncodeToString(combined.Sum(nil)), manifest.String(), nil
}
