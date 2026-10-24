// Package function interprets .neko @函数 bodies against an entity store and
// exposes them for engine conditions and action values.
package function

import (
	"fmt"
	"sync"

	"github.com/ruicat/astrbox/internal/entity"
	"github.com/ruicat/astrbox/internal/neko"
)

// MAX_CALL_DEPTH bounds nested function calls, mirroring the Python executor.
const MAX_CALL_DEPTH = 20

// compiledFunction is one registered @函数 definition plus its parsed body.
type compiledFunction struct {
	definition neko.FuncDef
	body       []statement
}

// Executor owns the registered .neko functions and runs them against one store.
type Executor struct {
	mu        sync.RWMutex
	store     *entity.Store
	functions map[string]*compiledFunction
}

// New creates an executor bound to store. The store may be nil for pure
// functions that never touch entity state.
func New(store *entity.Store) *Executor {
	return &Executor{store: store, functions: map[string]*compiledFunction{}}
}

// Register parses and stores one function definition. Re-registering a name
// replaces the previous definition.
func (e *Executor) Register(def neko.FuncDef) error {
	name := def.Name
	if name == "" {
		return fmt.Errorf("函数名不能为空")
	}
	body, err := parseBody(def.Body)
	if err != nil {
		return fmt.Errorf("函数 '%s' 解析失败: %w", name, err)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.functions == nil {
		e.functions = map[string]*compiledFunction{}
	}
	e.functions[name] = &compiledFunction{definition: def, body: body}
	return nil
}

// Unregister removes a function; unknown names are ignored.
func (e *Executor) Unregister(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.functions, name)
}

// Has reports whether a function is registered.
func (e *Executor) Has(name string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	_, ok := e.functions[name]
	return ok
}

// Call executes a registered function with positional arguments. current is the
// entity that relative paths ("状态.身体") resolve against.
func (e *Executor) Call(name string, args []any, current *entity.Entity) (any, error) {
	return e.callFunction(name, args, current, 0)
}

// callFunction is the re-entrant call path used by nested calls.
func (e *Executor) callFunction(name string, args []any, current *entity.Entity, depth int) (any, error) {
	if depth >= MAX_CALL_DEPTH {
		return nil, fmt.Errorf("函数调用过深（> %d），疑似递归循环: %s", MAX_CALL_DEPTH, name)
	}
	e.mu.RLock()
	function := e.functions[name]
	e.mu.RUnlock()
	if function == nil {
		return nil, fmt.Errorf("函数 '%s' 未定义", name)
	}
	locals := make(map[string]any, len(function.definition.Params))
	for index, param := range function.definition.Params {
		if index < len(args) {
			locals[param.Name] = args[index]
			continue
		}
		locals[param.Name] = nil
	}
	run := &runner{executor: e, current: current, locals: locals, depth: depth}
	return run.run(function.body)
}

// hasFunction reports whether name is registered as a user function.
func (e *Executor) hasFunction(name string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	_, ok := e.functions[name]
	return ok
}

// lookupEntity returns a stored entity by id, or nil.
func (e *Executor) lookupEntity(entityID string) *entity.Entity {
	if e.store == nil || entityID == "" {
		return nil
	}
	return e.store.Get(entityID)
}
