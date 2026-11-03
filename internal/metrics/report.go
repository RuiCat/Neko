// report.go defines the metric vocabulary: one metric per named judgement,
// with an explicit status so an unavailable metric is never mistaken for zero.
package metrics

import "time"

// Metric statuses.
const (
	STATUS_COMPUTED       = "computed"
	STATUS_NOT_COMPUTABLE = "not_computable"
)

// Metric is one architecture-level measurement.
type Metric struct {
	Name        string  `json:"name"`
	Status      string  `json:"status"`
	Reason      string  `json:"reason,omitempty"`
	Value       float64 `json:"value"`
	Numerator   int     `json:"numerator"`
	Denominator int     `json:"denominator"`
}

// Report is the whole offline pass over one journal directory.
type Report struct {
	GeneratedAt time.Time `json:"generated_at"`
	JournalDir  string    `json:"journal_dir"`
	Records     int       `json:"records"`
	// Annotated counts human-labelled turns used by the B-class metrics.
	Annotated int      `json:"annotated"`
	Metrics   []Metric `json:"metrics"`
}
