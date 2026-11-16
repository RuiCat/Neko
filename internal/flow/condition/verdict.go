// verdict.go declares the K2 judgment carrier: Verdict, its classified Reason
// rows and the frozen reason-code set. The carrier is additive-only: new
// reason codes may be added, existing codes keep their meaning.
package condition

// VERDICT_VERSION is the K2 carrier version. Adding fields or reason codes is
// compatible; removing or re-typing them is not.
const VERDICT_VERSION = 1

// Reason codes. Every rejection carries exactly one code so a caller can route
// on the code instead of parsing human text.
const (
	// REASON_MISSING_ITEM means the required item (name, scene, quantity)
	// was not found in the item view.
	REASON_MISSING_ITEM = "missing_item"
	// REASON_MISSING_MONEY means 交易.货币 is below the flow cost.
	REASON_MISSING_MONEY = "missing_money"
	// REASON_WRONG_SCENE means 位置.场景 differs from the template scene.
	REASON_WRONG_SCENE = "wrong_scene"
	// REASON_LOW_AFFINITY means 关系.数值.好感度 is below the floor.
	REASON_LOW_AFFINITY = "low_affinity"
	// REASON_NOT_READY means the requirement kind has no snapshot source yet
	// (time/day is the current placeholder).
	REASON_NOT_READY = "not_ready"
	// REASON_EVAL_ERROR means the snapshot value could not be read or typed.
	REASON_EVAL_ERROR = "eval_error"
)

// Reason is one classified rejection. Path names the snapshot location that
// failed; Want and Got render the expected and observed value as text so the
// struct stays JSON-stable and directly comparable across replays.
type Reason struct {
	Code string `json:"code"`
	Path string `json:"path"`
	Want string `json:"want"`
	Got  string `json:"got"`
}

// Verdict is the K2 judgment result. OK is true only when Reasons is empty;
// Reasons is never nil so two judgments of one snapshot compare equal.
type Verdict struct {
	OK      bool     `json:"ok"`
	Reasons []Reason `json:"reasons"`
}

// makeVerdict normalizes an empty result to a non-nil slice and derives OK from
// it, keeping OK and Reasons consistent by construction.
func makeVerdict(reasons []Reason) Verdict {
	if reasons == nil {
		reasons = []Reason{}
	}
	return Verdict{OK: len(reasons) == 0, Reasons: reasons}
}
