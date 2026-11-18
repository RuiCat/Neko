// harness_test.go holds the shared fixtures of the verification package: the
// in-memory entity seed, the V1 flow chain walker, the V2 double advance and the
// assertion helpers. Tests stay readable and none of them reuses an m5 test
// helper; each walk is driven through the public K1-K4 API only.
package verify

import (
	"testing"

	"github.com/RuiCat/Neko/internal/flow/condition"
	"github.com/RuiCat/Neko/internal/flow/settle"
	"github.com/RuiCat/Neko/internal/flow/spec"
	"github.com/RuiCat/Neko/internal/flow/step"
	"github.com/RuiCat/Neko/internal/flow/task"
)

// Harness identity shared by the flows and the task completion.
const (
	// TEST_ENTITY is the actor every verification walk settles.
	TEST_ENTITY = "猫猫"
	// TEST_TICK is the world tick recorded on completions.
	TEST_TICK = 7
	// EXECUTION_PREFIX makes each flow's execution id distinct.
	EXECUTION_PREFIX = "exec-"
)

// flowSpecFixture loads m1's flow-spec artifact.
func flowSpecFixture(t *testing.T) spec.FlowSpecFile {
	t.Helper()
	document, err := loadFlowSpec()
	if err != nil {
		t.Fatalf("load flow spec: %v", err)
	}
	return document
}

// migrationReportFixture loads m1's migration report.
func migrationReportFixture(t *testing.T) spec.MigrationReport {
	t.Helper()
	report, err := loadMigrationReport()
	if err != nil {
		t.Fatalf("load migration report: %v", err)
	}
	return report
}

// r18Fixture loads m5's R18 artifact.
func r18Fixture(t *testing.T) r18Artifact {
	t.Helper()
	document, err := loadR18Artifact()
	if err != nil {
		t.Fatalf("load r18 flows: %v", err)
	}
	return document
}

// probeCaseFixture loads the checked-in probe cases.
func probeCaseFixture(t *testing.T) []probeCase {
	t.Helper()
	cases, err := loadProbeCases()
	if err != nil {
		t.Fatalf("load probe cases: %v", err)
	}
	return cases
}

// flowByID returns the one flow with id, or fails the test.
func flowByID(t *testing.T, document spec.FlowSpecFile, id string) spec.FlowSpec {
	t.Helper()
	for _, flow := range document.Flows {
		if flow.ID == id {
			return flow
		}
	}
	t.Fatalf("flow-spec has no flow %q", id)
	return spec.FlowSpec{}
}

// taskTemplateByID returns the first template with id, or fails the test.
func taskTemplateByID(t *testing.T, document spec.FlowSpecFile, id string) spec.TaskTemplateSpec {
	t.Helper()
	for _, template := range document.TaskTemplates {
		if template.ID == id {
			return template
		}
	}
	t.Fatalf("flow-spec has no task template %q", id)
	return spec.TaskTemplateSpec{}
}

// newFixtureStore seeds the entity the walks share: the actor stands at the
// purchase scene with money, one kitchen ingredient, an inactive task and the
// reward baseline. Transporting items between scenes is a world step outside
// the flow engine, so the kitchen stock is seeded instead of simulated. 代谢.
// 胃内容 starts at 300 so the 吃饭 set (not add) can be observed.
func newFixtureStore() *fakeStore {
	return newFakeStore(map[string]any{
		condition.PATH_SCENE:        "S008",
		condition.PATH_MONEY:        100,
		condition.PATH_ITEMS:        []any{itemRow("食材", "S001-KT", 1)},
		settle.PATH_STOMACH_CONTENT: 300,
		task.PATH_TASK_STAGE:        task.STAGE_INACTIVE,
		task.PATH_GROWTH_EXPERIENCE: 0,
		task.PATH_RELATION_AFFINITY: 50,
	})
}

// runFlowChain walks the V1 chain 购买食材 → 做饭 → 吃饭 in corpus order.
func runFlowChain(t *testing.T, store *fakeStore, ledger *settle.MemoryLedger) []settle.Completion {
	t.Helper()
	document := flowSpecFixture(t)
	ids := []string{"购买食材", "做饭", "吃饭"}
	completions := make([]settle.Completion, 0, len(ids))
	for _, id := range ids {
		completions = append(completions, runFlow(t, store, ledger, flowByID(t, document, id)))
	}
	return completions
}

// runFlow drives one flow through condition.Judge, step.Start/Next and
// settle.Settle against the shared store, then returns its completion.
func runFlow(t *testing.T, store *fakeStore, ledger *settle.MemoryLedger, flow spec.FlowSpec) settle.Completion {
	t.Helper()
	if len(flow.Steps) == 0 {
		t.Fatalf("flow %q has no steps", flow.ID)
	}
	store.setScene(flow.Scene)
	snapshot := store.snapshot()
	verdict := condition.Judge(flow, snapshot)
	if !verdict.OK {
		t.Fatalf("flow %q: judgment refused: %+v", flow.ID, verdict.Reasons)
	}
	state, err := step.Start(&flow, EXECUTION_PREFIX+flow.ID, TEST_TICK)
	if err != nil {
		t.Fatalf("flow %q: step start: %v", flow.ID, err)
	}
	for index := 0; index < len(flow.Steps); index++ {
		state, err = step.Next(&flow, state, 1)
		if err != nil {
			t.Fatalf("flow %q: step %d: %v", flow.ID, index, err)
		}
	}
	if state.Status != step.StatusDone {
		t.Fatalf("flow %q: state = %s, want done", flow.ID, state.Status)
	}
	deps := settle.Deps{
		Snapshot: snapshot,
		Store:    store,
		Ledger:   ledger,
		Entity:   TEST_ENTITY,
		Tick:     TEST_TICK,
	}
	completion, report := settle.Settle(flow, verdict, state, deps)
	if report.Failed() {
		t.Fatalf("flow %q: settlement failed: %v", flow.ID, report.Errors)
	}
	if completion.FlowID != flow.ID || completion.Entity != TEST_ENTITY {
		t.Fatalf("flow %q: completion = %+v", flow.ID, completion)
	}
	return completion
}

// advanceTaskTwice builds one completion from the template and advances it twice
// through task.Advance, which is the V2 single-settlement walk.
func advanceTaskTwice(t *testing.T, store *fakeStore, template spec.TaskTemplateSpec) (task.Transition, task.Transition) {
	t.Helper()
	deps := task.Deps{Store: store, Ledger: task.NewMemoryLedger()}
	completion := settle.Completion{
		FlowID: template.ID,
		Entity: TEST_ENTITY,
		Result: settle.RESULT_COMPLETED,
		Tick:   TEST_TICK,
	}
	first, err := task.Advance(completion, []spec.TaskTemplateSpec{template}, deps)
	if err != nil {
		t.Fatalf("first advance: %v", err)
	}
	second, err := task.Advance(completion, []spec.TaskTemplateSpec{template}, deps)
	if err != nil {
		t.Fatalf("second advance: %v", err)
	}
	return first, second
}

// requireInt asserts one integer entity row.
func requireInt(t *testing.T, store *fakeStore, path string, want int) {
	t.Helper()
	got, ok := store.intAt(path)
	if !ok || got != want {
		t.Fatalf("%s = %v (integer %v), want %d", path, got, ok, want)
	}
}

// requireBool asserts one boolean entity row.
func requireBool(t *testing.T, store *fakeStore, path string, want bool) {
	t.Helper()
	value, err := store.value(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	got, ok := value.(bool)
	if !ok || got != want {
		t.Fatalf("%s = %v, want %v", path, value, want)
	}
}

// requireText asserts one string entity row.
func requireText(t *testing.T, store *fakeStore, path, want string) {
	t.Helper()
	value, err := store.value(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if value != want {
		t.Fatalf("%s = %v, want %q", path, value, want)
	}
}

// requireItemQuantity asserts the material quantity of one (name, scene).
func requireItemQuantity(t *testing.T, store *fakeStore, name, scene string, want int) {
	t.Helper()
	if got := store.itemQuantity(name, scene); got != want {
		t.Fatalf("%s in %s = %d, want %d", name, scene, got, want)
	}
}
