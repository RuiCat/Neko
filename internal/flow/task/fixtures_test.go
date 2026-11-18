// fixtures_test.go holds the doubles and builders shared by the task tests: a
// recording in-memory store and one task template whose reward is non-zero on both
// reward rows, so single settlement is observable on each of them.
package task

import (
	"fmt"
	"sync"
	"testing"

	"github.com/RuiCat/Neko/internal/flow/settle"
	"github.com/RuiCat/Neko/internal/flow/spec"
)

// TEST_ENTITY is the actor every fixture advances.
const TEST_ENTITY = "猫猫"

// fakeWrite is one recorded Set call.
type fakeWrite struct {
	entity string
	path   string
	value  any
}

// fakeStore is a map-backed Store that records every write in order and can fail a
// configured path, which is how the tests prove a refusal writes nothing.
type fakeStore struct {
	mu     sync.Mutex
	values map[string]any
	writes []fakeWrite
	failOn string
}

// newFakeStore returns a store seeded with values keyed by path for TEST_ENTITY.
func newFakeStore(values map[string]any) *fakeStore {
	seeded := map[string]any{}
	for path, value := range values {
		seeded[storeKey(TEST_ENTITY, path)] = value
	}
	return &fakeStore{values: seeded}
}

// storeKey separates the entity from the field path so one map can hold both.
func storeKey(entity, path string) string {
	return entity + "\x00" + path
}

// Get implements Store.
func (s *fakeStore) Get(entity, path string) (any, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[storeKey(entity, path)]
	return value, ok
}

// Set implements Store.
func (s *fakeStore) Set(entity, path string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failOn == path {
		return fmt.Errorf("fake store refuses %s", path)
	}
	s.writes = append(s.writes, fakeWrite{entity: entity, path: path, value: value})
	s.values[storeKey(entity, path)] = value
	return nil
}

// writeCount reports how often path was written.
func (s *fakeStore) writeCount(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, write := range s.writes {
		if write.path == path {
			count++
		}
	}
	return count
}

// totalWrites reports how many writes happened at all.
func (s *fakeStore) totalWrites() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.writes)
}

// valueAt returns the current value of one field.
func (s *fakeStore) valueAt(path string) any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[storeKey(TEST_ENTITY, path)]
}

// intAt returns the current integer value of one field, failing when it is absent
// or not numeric.
func (s *fakeStore) intAt(t *testing.T, path string) int {
	t.Helper()
	value := s.valueAt(path)
	if value == nil {
		t.Fatalf("field %s is absent", path)
	}
	parsed, err := asInt(value)
	if err != nil {
		t.Fatalf("field %s: %v", path, err)
	}
	return parsed
}

// rewardTemplate is the corpus 收集任务 template (experience 20, affinity 5): both
// reward rows are non-zero, so a double payment would be visible on each.
func rewardTemplate() spec.TaskTemplateSpec {
	return spec.TaskTemplateSpec{
		ID:     "收集任务",
		Stages: []string{STAGE_INACTIVE, STAGE_ACTIVE, STAGE_COMPLETED},
		Reward: spec.TaskReward{Experience: 20, Affinity: 5},
	}
}

// completionFixture builds one successful K4 completion for the reward template.
func completionFixture() settle.Completion {
	return settle.Completion{
		FlowID: "收集任务",
		Entity: TEST_ENTITY,
		Result: settle.RESULT_COMPLETED,
		Tick:   7,
	}
}

// isEmptyTransition reports whether Advance returned the zero Transition. The
// carrier holds slices, so tests compare fields instead of values.
func isEmptyTransition(transition Transition) bool {
	return transition.FlowID == "" &&
		transition.Entity == "" &&
		transition.Tick == 0 &&
		transition.FromStage == "" &&
		transition.ToStage == "" &&
		len(transition.Stages) == 0 &&
		transition.Progress == 0 &&
		len(transition.Rewards) == 0 &&
		!transition.Replay
}
