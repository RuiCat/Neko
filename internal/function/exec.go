// Statement execution for interpreted .neko functions: blocks, assignment,
// if/else chains, for loops and return propagation.
package function

import (
	"errors"
	"fmt"
	"sort"

	"github.com/ruicat/astrbox/internal/entity"
)

// MAX_EXEC_STEPS bounds one function invocation to keep runaway loops finite.
const MAX_EXEC_STEPS = 100000

// runner executes one function invocation with its own locals.
type runner struct {
	executor *Executor
	current  *entity.Entity
	locals   map[string]any
	depth    int
	steps    int
}

// run executes the function body and returns its 返回 value (nil when absent).
func (r *runner) run(body []statement) (any, error) {
	value, returned, err := r.execStatements(body)
	if err != nil {
		return nil, err
	}
	if returned {
		return value, nil
	}
	return nil, nil
}

// execStatements runs statements in order until a return or an error.
func (r *runner) execStatements(statements []statement) (any, bool, error) {
	for _, statement := range statements {
		r.steps++
		if r.steps > MAX_EXEC_STEPS {
			return nil, false, fmt.Errorf("函数执行步数超过上限 %d", MAX_EXEC_STEPS)
		}
		value, returned, err := r.execStatement(statement)
		if err != nil || returned {
			return value, returned, err
		}
	}
	return nil, false, nil
}

// execStatement dispatches one statement.
func (r *runner) execStatement(statement statement) (any, bool, error) {
	switch node := statement.(type) {
	case *assignStatement:
		return nil, false, r.execAssign(node)
	case *exprStatement:
		_, err := r.eval(node.value)
		return nil, false, err
	case *ifStatement:
		return r.execIf(node)
	case *returnStatement:
		if node.value == nil {
			return nil, true, nil
		}
		value, err := r.eval(node.value)
		return value, true, err
	case *forStatement:
		return r.execFor(node)
	case *whileStatement:
		return r.execWhile(node)
	case *tryStatement:
		return r.execTry(node)
	case *multiAssignStatement:
		return nil, false, r.execMultiAssign(node)
	case *breakStatement:
		return nil, false, errBreak
	case *continueStatement:
		return nil, false, errContinue
	}
	return nil, false, fmt.Errorf("不支持的语句: %T", statement)
}

// execIf evaluates the condition and runs the matching branch.
func (r *runner) execIf(node *ifStatement) (any, bool, error) {
	condition, err := r.eval(node.condition)
	if err != nil {
		return nil, false, err
	}
	if isTruthy(condition) {
		return r.execStatements(node.then)
	}
	return r.execStatements(node.elseBlock)
}

// execFor iterates a list, text or map and runs the body per item.
func (r *runner) execFor(node *forStatement) (any, bool, error) {
	iterable, err := r.eval(node.iterable)
	if err != nil {
		return nil, false, err
	}
	items := iterableItems(iterable)
	for _, item := range items {
		r.steps++
		if r.steps > MAX_EXEC_STEPS {
			return nil, false, fmt.Errorf("函数执行步数超过上限 %d", MAX_EXEC_STEPS)
		}
		if err := r.bindLoopVariables(node.variables, item); err != nil {
			return nil, false, err
		}
		value, returned, err := r.execStatements(node.body)
		if errors.Is(err, errBreak) {
			return nil, false, nil
		}
		if errors.Is(err, errContinue) {
			continue
		}
		if err != nil || returned {
			return value, returned, err
		}
	}
	return nil, false, nil
}

// bindLoopVariables binds one or several loop variables (tuple unpacking).
func (r *runner) bindLoopVariables(variables []string, item any) error {
	if len(variables) == 1 {
		r.locals[variables[0]] = item
		return nil
	}
	values, ok := item.([]any)
	if !ok || len(values) != len(variables) {
		return fmt.Errorf("无法解包循环值到 %d 个变量", len(variables))
	}
	for index, name := range variables {
		r.locals[name] = values[index]
	}
	return nil
}

// execTry runs the body, handles errors, and always runs finally.
func (r *runner) execTry(node *tryStatement) (any, bool, error) {
	value, returned, err := r.execStatements(node.body)
	if err != nil && !errors.Is(err, errBreak) && !errors.Is(err, errContinue) && len(node.handler) > 0 {
		if node.exceptType == "" || exceptMatches(node.exceptType, err) {
			r.locals[node.errorName] = err.Error()
			value, returned, err = r.execStatements(node.handler)
		}
	}
	if len(node.finalizer) > 0 {
		finalValue, finalReturned, finalErr := r.execStatements(node.finalizer)
		if finalErr != nil {
			return nil, false, finalErr
		}
		if finalReturned {
			return finalValue, true, nil
		}
	}
	return value, returned, err
}

// errBreak and errContinue are loop-control sentinels.
var (
	errBreak    = errors.New("break")
	errContinue = errors.New("continue")
)

// execWhile loops while the condition is truthy, bounded by MAX_EXEC_STEPS.
func (r *runner) execWhile(node *whileStatement) (any, bool, error) {
	for {
		condition, err := r.eval(node.condition)
		if err != nil {
			return nil, false, err
		}
		if !isTruthy(condition) {
			return nil, false, nil
		}
		r.steps++
		if r.steps > MAX_EXEC_STEPS {
			return nil, false, fmt.Errorf("函数执行步数超过上限 %d", MAX_EXEC_STEPS)
		}
		value, returned, err := r.execStatements(node.body)
		if errors.Is(err, errBreak) {
			return nil, false, nil
		}
		if errors.Is(err, errContinue) {
			continue
		}
		if err != nil || returned {
			return value, returned, err
		}
	}
}

// iterableItems converts a Python-style iterable into a slice of items.
func iterableItems(iterable any) []any {
	switch typed := iterable.(type) {
	case []any:
		return typed
	case string:
		runes := []rune(typed)
		items := make([]any, len(runes))
		for index, ch := range runes {
			items[index] = string(ch)
		}
		return items
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		items := make([]any, len(keys))
		for index, key := range keys {
			items[index] = key
		}
		return items
	}
	return nil
}

// execAssign evaluates the value and writes it to the target.
func (r *runner) execAssign(node *assignStatement) error {
	value, err := r.eval(node.value)
	if err != nil {
		return err
	}
	if node.operator == "+=" || node.operator == "-=" {
		previous, evalErr := r.eval(node.target)
		if evalErr != nil {
			return evalErr
		}
		value = applyBinary(string(node.operator[0]), previous, value)
	}
	return r.assignTarget(node.target, value)
}

// execMultiAssign unpacks a list into several assignment targets.
func (r *runner) execMultiAssign(node *multiAssignStatement) error {
	value, err := r.eval(node.value)
	if err != nil {
		return err
	}
	values, ok := value.([]any)
	if !ok || len(values) != len(node.targets) {
		return fmt.Errorf("无法解包赋值到 %d 个目标", len(node.targets))
	}
	for index, target := range node.targets {
		if err := r.assignTarget(target, values[index]); err != nil {
			return err
		}
	}
	return nil
}

// assignTarget writes one path or index target.
func (r *runner) assignTarget(target expression, value any) error {
	switch typed := target.(type) {
	case *pathExpression:
		r.assignPath(typed, value)
		return nil
	case *indexExpression:
		return r.assignIndex(typed, value)
	}
	return fmt.Errorf("不支持的赋值目标: %T", target)
}

// assignIndex writes one list element or map key.
func (r *runner) assignIndex(target *indexExpression, value any) error {
	container, err := r.eval(target.target)
	if err != nil {
		return err
	}
	index, err := r.eval(target.low)
	if err != nil {
		return err
	}
	switch typed := container.(type) {
	case []any:
		position, ok := toIntValue(index)
		if !ok || position < 0 || position >= len(typed) {
			return nil
		}
		typed[position] = value
	case map[string]any:
		typed[pyText(index)] = value
	}
	return nil
}
