// judge.go evaluates requirements against a read-only snapshot and classifies
// every failure. Evaluation is pure: it only calls Snapshot.Get.
package condition

import (
	"fmt"
	"strconv"

	"github.com/RuiCat/Neko/internal/flow/spec"
)

// Judge evaluates the preconditions carried by K1 FlowSpec: scene, required
// items and money cost. A negative cost is invalid under K1 validation and
// becomes REASON_EVAL_ERROR instead of being silently ignored.
func Judge(flow spec.FlowSpec, snap Snapshot) Verdict {
	if flow.Cost < 0 {
		return makeVerdict([]Reason{{
			Code: REASON_EVAL_ERROR,
			Path: "cost",
			Want: ">= 0",
			Got:  strconv.Itoa(flow.Cost),
		}})
	}
	return JudgeRequirements(Requirements(flow), snap)
}

// JudgeRequirements evaluates an explicit requirement list. Callers append
// affinity/time requirements here. Reasons follow reqs order and the item view
// is read at most once, so replaying one snapshot yields one identical verdict.
func JudgeRequirements(reqs []Requirement, snap Snapshot) Verdict {
	if snap == nil {
		return makeVerdict([]Reason{{
			Code: REASON_EVAL_ERROR,
			Path: "",
			Want: "non-nil snapshot",
			Got:  "nil",
		}})
	}
	evaluator := &evaluator{snap: snap}
	for _, req := range reqs {
		evaluator.judge(req)
	}
	return makeVerdict(evaluator.reasons)
}

// evaluator accumulates reasons and caches the item view for one judgment.
type evaluator struct {
	snap        Snapshot
	reasons     []Reason
	items       []itemRecord
	itemsLoaded bool
	itemsErr    error
}

// judge dispatches one requirement to its kind-specific check.
func (e *evaluator) judge(req Requirement) {
	switch req.Kind {
	case KIND_SCENE:
		e.judgeScene(req)
	case KIND_ITEM:
		e.judgeItem(req)
	case KIND_MONEY:
		e.judgeMoney(req)
	case KIND_AFFINITY:
		e.judgeAffinity(req)
	case KIND_TIME:
		e.judgeTime(req)
	default:
		e.fail(REASON_EVAL_ERROR, string(req.Kind), "known requirement kind", string(req.Kind))
	}
}

// judgeScene compares 位置.场景 with the template scene.
func (e *evaluator) judgeScene(req Requirement) {
	if req.Scene == "" {
		return
	}
	got, err := readString(e.snap, PATH_SCENE)
	if err != nil {
		e.evalError(PATH_SCENE, err)
		return
	}
	if got != req.Scene {
		e.fail(REASON_WRONG_SCENE, PATH_SCENE, req.Scene, got)
	}
}

// judgeItem resolves the required item inside the template scene.
func (e *evaluator) judgeItem(req Requirement) {
	if req.Item == "" {
		e.fail(REASON_EVAL_ERROR, PATH_ITEMS, "non-empty item name", "")
		return
	}
	quantity := req.Quantity
	if quantity <= 0 {
		quantity = 1
	}
	e.loadItems()
	if e.itemsErr != nil {
		return
	}
	got := countItemQuantity(e.items, req.Item, req.Scene)
	if got < quantity {
		e.fail(REASON_MISSING_ITEM, PATH_ITEMS+"."+req.Item, strconv.Itoa(quantity), strconv.Itoa(got))
	}
}

// loadItems reads the item view once per judgment and records one eval_error
// when the view itself cannot be decoded.
func (e *evaluator) loadItems() {
	if e.itemsLoaded {
		return
	}
	e.itemsLoaded = true
	e.items, e.itemsErr = readItems(e.snap)
	if e.itemsErr != nil {
		e.evalError(PATH_ITEMS, e.itemsErr)
	}
}

// judgeMoney compares 交易.货币 with the flow cost.
func (e *evaluator) judgeMoney(req Requirement) {
	if req.Threshold <= 0 {
		return
	}
	got, err := readInt(e.snap, PATH_MONEY)
	if err != nil {
		e.evalError(PATH_MONEY, err)
		return
	}
	if got < req.Threshold {
		e.fail(REASON_MISSING_MONEY, PATH_MONEY, strconv.Itoa(req.Threshold), strconv.Itoa(got))
	}
}

// judgeAffinity compares 关系.数值.好感度 with the required floor.
func (e *evaluator) judgeAffinity(req Requirement) {
	if req.Threshold <= 0 {
		return
	}
	path := req.Path
	if path == "" {
		path = PATH_AFFINITY
	}
	got, err := readInt(e.snap, path)
	if err != nil {
		e.evalError(path, err)
		return
	}
	if got < req.Threshold {
		e.fail(REASON_LOW_AFFINITY, path, strconv.Itoa(req.Threshold), strconv.Itoa(got))
	}
}

// judgeTime reports the time/day placeholder as not_ready: no snapshot source
// exists yet, so the condition cannot be evaluated.
func (e *evaluator) judgeTime(req Requirement) {
	path := req.Path
	if path == "" {
		path = PATH_TIME
	}
	e.fail(REASON_NOT_READY, path, fmt.Sprintf("day>=%d", req.Threshold), "")
}

// fail appends one classified rejection.
func (e *evaluator) fail(code, path, want, got string) {
	e.reasons = append(e.reasons, Reason{Code: code, Path: path, Want: want, Got: got})
}

// evalError classifies a snapshot read/decode failure.
func (e *evaluator) evalError(path string, err error) {
	e.fail(REASON_EVAL_ERROR, path, "readable value", err.Error())
}
