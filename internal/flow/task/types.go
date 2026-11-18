// types.go declares the carriers of task advancement: the store/ledger seam, the
// Transition returned by Advance and the Reward rows it applied.
package task

// Deps is the run-time seam of one advance: the entity store and the in-process
// ledger. Both are interfaces so the host can swap a real store in and a test can
// assert every write.
type Deps struct {
	Store  Store
	Ledger Ledger
}

// Reward is one applied reward row: the source-layer path and the amount added.
type Reward struct {
	Path   string `json:"path"`
	Amount int    `json:"amount"`
}

// Transition records one task advancement driven by one K4 completion. FromStage
// and ToStage bracket the logical stage transition the write set causes; Stages is
// the ordered path traversed on the way to 完成. Replay marks a completion that
// wrote nothing because it was already advanced, and Rewards is empty in that case.
type Transition struct {
	FlowID    string   `json:"flowID"`
	Entity    string   `json:"entity"`
	Tick      int      `json:"tick"`
	FromStage string   `json:"fromStage"`
	ToStage   string   `json:"toStage"`
	Stages    []string `json:"stages"`
	Progress  int      `json:"progress"`
	Rewards   []Reward `json:"rewards"`
	Replay    bool     `json:"replay"`
}
