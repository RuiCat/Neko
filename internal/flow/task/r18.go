// r18.go declares the C7 artifact shapes. r18-flows.json is the R18 counterpart of
// flow-spec.json: one row per corpus event with its source anchor, the ordered
// stage steps and the effects settlement may write.
package task

import "github.com/RuiCat/Neko/internal/flow/spec"

// R18_SCHEMA_VERSION is the artifact version; new fields may be added, existing
// fields keep their meaning.
const R18_SCHEMA_VERSION = 1

// R18_CORPUS is the C7 corpus: the staged event digest. cat_r18_flows.neko is not
// usable here because its twelve rows are an index plus a three-line dialogue
// digest and carry no effects at all.
const R18_CORPUS = "/home/ruicat/Desktop/备份/scripts/r18/cat_r18_events.neko"

// R18Flow is one R18 flow row. ID and Source anchor the row to its corpus event,
// Steps are the ordered stage beats, and Effects reuse the K1 effect shape so a
// later corpus with real effects needs no new type.
type R18Flow struct {
	ID      string            `json:"id"`
	Source  spec.Source       `json:"source"`
	Steps   []string          `json:"steps"`
	Effects []spec.EffectSpec `json:"effects"`
}

// R18FlowsFile is the artifact root. Gaps records every C7 field the corpus cannot
// supply; it is evidence for the C7 judgment, never a placeholder for guessed data.
type R18FlowsFile struct {
	SchemaVersion int       `json:"schemaVersion"`
	Corpus        string    `json:"corpus"`
	Gaps          []string  `json:"gaps,omitempty"`
	R18Flows      []R18Flow `json:"r18Flows"`
}

// DefaultR18Corpus returns the read-only C7 corpus path.
func DefaultR18Corpus() string {
	return R18_CORPUS
}
