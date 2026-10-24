// Syntax tree nodes for the .neko function subset: statements, expressions and
// the dotted path forms that bind to entities.
package function

// expression is one evaluable node.
type expression interface{}

// statement is one executable node.
type statement interface{}

// literalExpression is a number, string or boolean literal.
type literalExpression struct{ value any }

// pathExpression is a dotted path rooted at a local variable, an entity id or
// the current entity ({e}).
type pathExpression struct {
	root    string
	current bool
	fields  []string
}

// indexExpression is "target[index]" or the slice form "target[low:high]".
type indexExpression struct {
	target expression
	low    expression
	high   expression
	step   expression
	slice  bool
}

// chainExpression is a Python-style chained comparison (a < b < c).
type chainExpression struct {
	operands  []expression
	operators []string
}

// tryStatement is "try { ... } except [变量] { ... } [finally { ... }]".
type tryStatement struct {
	body       []statement
	handler    []statement
	finalizer  []statement
	errorName  string
	exceptType string
}

// breakStatement leaves the nearest loop.
type breakStatement struct{}

// continueStatement jumps to the next loop iteration.
type continueStatement struct{}

// whileStatement is "while 条件 { ... }".
type whileStatement struct {
	condition expression
	body      []statement
}

// callExpression is a plain function call: 名称(参数...).
type callExpression struct {
	name string
	args []expression
}

// methodCallExpression is a method invocation such as 列表.append(项).
type methodCallExpression struct {
	target expression
	method string
	args   []expression
}

// unaryExpression is "!"/"not" or unary minus.
type unaryExpression struct {
	operator string
	operand  expression
}

// binaryExpression is one infix operator application.
type binaryExpression struct {
	operator string
	left     expression
	right    expression
}

// listCompExpression is a list comprehension [expr for x in iterable if cond].
type listCompExpression struct {
	expression expression
	variable   string
	iterable   expression
	condition  expression
}

// mapExpression is a map literal {"键": 值, ...}.
type mapExpression struct {
	keys   []string
	values []expression
}

// listExpression is a list literal such as [] or [1, 2].
type listExpression struct{ items []expression }

// assignStatement is "target = value", "target += value" or "target -= value".
type assignStatement struct {
	target   expression
	value    expression
	operator string
}

// multiAssignStatement is "a, b = value" (tuple unpacking assignment).
type multiAssignStatement struct {
	targets []expression
	value   expression
}

// exprStatement is a bare expression used for its side effect (a call).
type exprStatement struct{ value expression }

// ifStatement is one if / else if / else chain node.
type ifStatement struct {
	condition expression
	then      []statement
	elseBlock []statement
}

// returnStatement is 返回/return.
type returnStatement struct{ value expression }

// forStatement is "for 项 in 列表 { ... }".
type forStatement struct {
	variables []string
	iterable  expression
	body      []statement
}
