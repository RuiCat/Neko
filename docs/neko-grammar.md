# `.neko` 语法（完整 EBNF）

本文给出 `.neko` 源文件的完整 EBNF：词法、顶层块、模板、规则、事件与 `@函数` 体内嵌语言。

语义（合并、匹配、求值、事件、执行限制等）见 [neko-semantics.md](neko-semantics.md)。

约定：

- 本文只描述**语法**；合并、冲突、求值等语义不在此展开。
- 规则体、事件体按行解析，换行在其中有分隔意义；EBNF 给出规范写法，实际解析器对空白和空行更宽容。
- `(* ... *)` 为 EBNF 注释；`? ... ?` 为无法用 EBNF 直接表达的说明性终结符。

## 1. 词法与文件

```ebnf
file          := { spacing | topLevel } ;

spacing       := space | tab | newline | lineComment | hashComment ;
space         := ' ' ;
tab           := '\t' ;
newline       := '\r\n' | '\n' | '\r' ;
lineComment   := '//' { notNewline } [ newline ] ;
hashComment   := '#' ' ' { notNewline } [ newline ] ;   (* 仅行首 `# ` *)

notNewline    := ? 除 CR、LF 外的任意字符 ? ;
restOfLine    := { notNewline } ;

ident         := identStart { identPart } ;
identStart    := unicodeLetter | '_' ;
identPart     := unicodeLetter | unicodeDigit | '_' ;
unicodeLetter := ? Unicode 字母（\p{L}） ? ;
unicodeDigit  := ? Unicode 数字（\p{N}） ? ;

typeName      := ( unicodeLetter | unicodeDigit | '_' | ':' )+ ;
templateName  := ? `{` 之前的文本；规范写法同 typeName ? ;

ws            := ( space | tab | newline | lineComment )+ ;

stringLiteral := '"' { quotedChar } '"'
               | "'" { quotedChar } "'" ;
quotedChar    := '\' anyChar | ? 除对应引号、换行外的任意字符 ? ;
anyChar       := ? 任意字符 ? ;

number        := [ '+' | '-' ] numDigits [ '.' numDigits ]
                 [ ( 'e' | 'E' ) [ '+' | '-' ] numDigits ]
               | ? inf / infinity / nan（不区分大小写，可带正负号） ? ;
numDigits     := digit { digit } { '_' digit { digit } } ;
```

说明：

- `//` 到行尾是注释，块头与块体内均可出现。
- 行首 `# `（井号加空格）是整行注释；行首 `#规则`、`#键: 值` 不是注释，见第 4、5 节。
- 类型名允许中文、字母、数字、`_`、`:`；`:` 只用于继承链（`NPC:猫猫`）。

## 2. 顶层块

```ebnf
topLevel      := entityBlock | defineBlock | dataBlock | descBlock
               | ruleDecl | multiRuleBlock | lineRule
               | eventBlock | funcBlock | metaLine ;
```

扫描器按行首前缀区分块类型；块头可跨行，块体是配对的 `{ ... }`。

## 3. 模板块

```ebnf
entityBlock   := '@实体' ws typeName [ ws '[' [ parentList ] ws ']' ] ws jsonObject ;
parentList    := typeName { ws ( ',' | '，' ) ws typeName } ;

defineBlock   := ( '@定义' | '@Data' ) ws templateName ws jsonObject ;
dataBlock     := '@data' ws templateName ws jsonObject ;
descBlock     := '@描述' ws templateName ws balancedBlock ;
balancedBlock := ? 配对的 `{ ... }`，内容忽略 ? ;
```

说明：

- `@定义` 与 `@Data` 是实体模板的同义写法；`@data` 不编译为模板；`@描述` 仅占位，不参与执行。
- `@实体` 的类型名与父列表之间用空白分隔；父列表可省略，也可写成 `[]`。
- `@定义` / `@Data` 的名称取 `{` 之前的文本，规范写法同 `typeName`。
- `@定义` / `@Data` 的块体只能是 JSON；`@事件`、`@函数`、`@规则` 是顶层兄弟块，不能嵌进模板体。模板字段声明的组合约束与运行时写入的边界见 [neko-semantics.md](neko-semantics.md) §1.6。

### 3.1 模板体 JSON

```ebnf
jsonObject    := '{' jsonWs '}'
               | '{' jsonWs jsonMember { jsonWs ',' jsonWs jsonMember } jsonWs '}' ;
jsonMember    := jsonString jsonWs ':' jsonWs jsonValue ;
jsonValue     := jsonObject | jsonArray | jsonString | jsonNumber
               | 'true' | 'false' | 'null' ;

jsonArray     := '[' jsonWs ']'
               | '[' jsonWs jsonValue { jsonWs ',' jsonWs jsonValue } jsonWs ']' ;
jsonWs        := { space | tab | newline | lineComment } ;

jsonString    := '"' { jsonChar } '"' ;
jsonChar      := unicodeChar | jsonEscape ;
unicodeChar   := ? 除 '"'、'\' 与控制字符（U+0000–U+001F）外的任意字符 ? ;
jsonEscape    := '\' ( '"' | '\' | '/' | 'b' | 'f' | 'n' | 'r' | 't'
                     | 'u' hexDigit hexDigit hexDigit hexDigit ) ;

jsonNumber    := [ '-' ] jsonInt [ jsonFrac ] [ jsonExp ] ;
jsonInt       := '0' | nonzeroDigit { digit } ;
jsonFrac      := '.' digit { digit } ;
jsonExp       := ( 'e' | 'E' ) [ '+' | '-' ] digit { digit } ;
digit         := '0' | nonzeroDigit ;
nonzeroDigit  := '1' | '2' | '3' | '4' | '5' | '6' | '7' | '8' | '9' ;
hexDigit      := digit | 'a' | 'b' | 'c' | 'd' | 'e' | 'f'
                       | 'A' | 'B' | 'C' | 'D' | 'E' | 'F' ;
```

### 3.2 字段注解

字段注解写在 JSON 键的字符串内容里：

```ebnf
annotatedName := fieldName [ '[' annotationList ']' ] ;
annotationList := annotation { ',' annotation } ;
annotation    := 'int' | 'float' | 'str' | 'bool'
               | 'readonly' | 'nullable'
               | '@' ident
               | 'enum:' enumItem { ';' enumItem }
               | 'min:' number
               | 'max:' number ;
fieldName     := ? 不含 `[`、`]` 的字段名 ? ;
enumItem      := ? 不含 `;` 的文本 ? ;
```

示例：`"体力[int,@体力管理,min:0,max:100]": 75`。

## 4. 规则块

```ebnf
ruleDecl      := '@规则' [ ruleHeader ] ruleBody
               | '@规则' ruleHeader ':' rangeField rangeBody ;

ruleHeader    := '[' subjectList ']' ;
subjectList   := subject { '，' subject } ;          (* 全角逗号分隔 *)

subject       := '$' [ '[' subjectQual ']' ]
               | '$' digits [ '[' subjectQual ']' ]
               | typeName [ '[' typeName ']' ] ;
subjectQual   := typeName [ '.' conditionExpr ] ;
digits        := digit { digit } ;
```

说明：

- `$` / `$[NPC]` 是单主体；`$1` / `$1[NPC]` / `$1[NPC.性别=女]` 是多主体。
- 全局实体写作类型名，例如 `场景`、`世界`。
- 没有 `[...]` 头时视为单主体 `$`。

### 4.1 规则体的三种写法

```ebnf
ruleBody      := '{' ( keyValueBody | conditionBranchBody | multicondBody ) '}' ;
```

**键值式：**

```ebnf
keyValueBody  := { keyValueLine | bareLine } ;
bareLine      := action | conditionExpr ;
keyValueLine  := 'id' ':' restOfLine { continuationLine }
               | '优先级' ':' restOfLine { continuationLine }
               | '状态' ':' restOfLine { continuationLine }
               | '条件' ':' restOfLine { continuationLine }
               | '动作' ':' restOfLine { continuationLine }
               | '分支' ':' '{' { branchLine } '}' ;
continuationLine := ? 不以已识别键开头的非空行 ? ;
```

`id`、`优先级` 为元数据；`状态` / `条件` 参与触发条件；`动作` 是一条动作；`分支` 的值是 `{ ... }` 块。值可以跨行，直到下一个键或块结束。

键值体也接受未归入任何键的裸动作/裸条件行：含 `=`（或 `>=`/`<=`）的行按动作解析，含 `→` 的行按条件保存。规范写法仍然是把它们写进 `动作:` / `条件:`。

**条件块式：**

```ebnf
conditionBranchBody := conditionBlock { ',' conditionBlock } ;
conditionBlock      := conditionExpr '{' { actionLine } '}' ;
```

一个块内出现两个及以上 `条件 { 动作 }` 时，展开为多条规则；逗号可省略。

**多条件式：**

```ebnf
multicondBody := { ruleMetaLine } childRule { [ ',' ] childRule } ;
ruleMetaLine  := ( 'id' | '优先级' ) ':' restOfLine newline ;
childRule     := ident ':' '{' { childLine } '}' ;
childLine     := ( '条件' | 'condition' ) ':' conditionExpr
               | ( '动作' | 'action' ) ':' action
               | action ;
```

`@多规则` / `@多条件` 块直接把 `childRule` 列表写在块体里：

```ebnf
multiRuleBlock := ( '@多规则' | '@多条件' )
                  [ ruleHeader | ident ]
                  '{' multicondBody '}' ;
```

### 4.2 阈值式规则

```ebnf
rangeBody     := '{' { rangeBranch [ ',' ] } '}' ;
rangeField    := '$' [ digits ] ( '.' path )?
               | '{e}' ( '.' path )?
               | typeName ( '.' path )? ;
rangeBranch   := rangeLabel ':' '{' { actionLine } '}' ;
rangeLabel    := number
               | ( '>=' | '<=' | '>' | '<' | '=' ) [ ws ] number
               | text ;
text          := ? 非空文本 ? ;
```

示例：`@规则[$[NPC]]:$.状态.饥饿 { 30: {...}, >= 80: {...} }`。

### 4.3 条件表达式

```ebnf
conditionExpr := ( '!' | '~' ) conditionExpr | condOr ;

condOr        := condAnd { ( '∨' | 'or' ) condAnd } ;
condAnd       := condPrimary { ( '∧' | 'and' ) condPrimary } ;
condPrimary   := '(' conditionExpr ')' | comparison | operand ;

comparison    := operand condOp operand ;
condOp        := '>=' | '<=' | '!=' | '==' | 'contains' | 'in'
               | '>' | '<' | '=' ;

operand       := arithExpr | setLiteral | stringLiteral | number
               | 'true' | 'false' | refPath ;
arithExpr     := arithTerm { ( '+' | '-' ) arithTerm } ;
arithTerm     := arithFactor { ( '*' | '/' | '%' ) arithFactor } ;
arithFactor   := '-' arithFactor
               | number | stringLiteral | refPath
               | '(' arithExpr ')' ;

setLiteral    := '{' [ setItem { ( ',' | '，' ) setItem } ] '}' ;
setItem       := ? 除逗号、花括号外的文本 ? ;

refPath       := '$' [ digits ] ( '.' pathSeg )*
               | '{e}' ( '.' pathSeg )*
               | pathSeg ( '.' pathSeg )* ;
pathSeg       := ident | digits ;
path          := pathSeg ( '.' pathSeg )* ;
```

说明：

- `=` 与 `==` 等价；`in` / `contains` 两侧需要词边界。
- 只有一侧没有比较运算符时，按“路径存在且为真”求值。
- 右侧的 `{a,b,c}` 是集合字面量，配合 `in`、`==`、`!=` 使用。

### 4.4 动作

```ebnf
actionLine    := action ;
action        := target actionOp value ;
target        := '$' [ digits ] ( '.' path )?
               | '{e}' ( '.' path )?
               | typeName ( '.' path )?
               | path ;
actionOp      := '>=' | '<=' | '!=' | '==' | '+=' | '-=' | '*=' | '/='
               | '=' | '>' | '<' | 'in' | 'contains' ;
value         := scalar | refPath ;
scalar        := stringLiteral | number | 'true' | 'false'
               | jsonObject | jsonArray | rawText ;
rawText       := ? 未被解析为其它字面量的剩余文本 ? ;
```

说明：

- `=` 写入；`+=` / `-=` 数值加减（值为容器或全局实体时转为追加）。
- 值不加引号且形如路径时，编译为运行时引用。
- 规则体中的 `动作:` 行只解析一条动作；多条动作用条件块式。

### 4.5 分支

```ebnf
branchLine    := ( '默认' | 'else' )
               | conditionExpr '→' stateAssign
               | conditionExpr ;
stateAssign   := '状态' [ '.' ident ] '=' scalar ;
```

`状态 = "活跃"` 是 `状态.身体 = "活跃"` 的语法糖；子组名可省略。分支箭头右侧只识别这条状态赋值；其它目标不会编译成动作。

### 4.6 行式规则与元数据

```ebnf
lineRule      := '#规则' [ ':' ] [ conditionExpr ] [ '→' action ] newline ;
metaKey       := ( unicodeLetter | unicodeDigit | '_' )+ ;
metaLine      := '#' metaKey ':' restOfLine newline ;
```

## 5. 事件块

```ebnf
eventBlock    := '@事件' [ ':' ] ident '{' { eventLine } '}' ;
eventLine     := ( '条件' | 'condition' ) ':' conditionExpr
               | ( '处理器' | '处理' | 'handler' ) ':' ident
               | action
               | lineComment ;
```

说明：

- `@事件:名称` 与 `@事件 名称` 等价。
- 未写 `处理器:` 时，处理器名默认与事件名相同。
- 事件体中的动作行按“含 `=` 且不以 `动作` 开头”识别。
- 事件处理器是 `@函数`；语法上处理器体内的写入路径不受模板组合约束，运行时边界见 [neko-semantics.md](neko-semantics.md) §1.6。

## 6. 函数块

```ebnf
funcBlock     := ( '@函数' | '@func' ) ws funcHeader ws '{' { statementSep | statement } '}' ;
funcHeader    := ident '(' [ param { ',' param } ] ')' [ '->' typeName ] ;
param         := ident [ ':' typeName ] ;
```

### 6.1 语句

```ebnf
statement     := ifStatement | forStatement | whileStatement | tryStatement
               | breakStatement | continueStatement | returnStatement
               | assignment | multiAssignment | expressionStatement ;

statementSep  := newline | ';' ;
block         := '{' { statementSep | statement } '}' ;

ifStatement   := ( 'if' | '如果' ) expression block [ elsePart ] ;
elsePart      := ( '否则如果' ) expression block [ elsePart ]
               | ( 'else' | '否则' ) ( ifStatement | block ) ;

forStatement  := ( 'for' | '循环' ) ident { ',' ident }
                 'in' expression block ;
whileStatement := ( 'while' | '当' ) expression block ;

tryStatement  := ( 'try' | '尝试' ) block tryTail ;
tryTail       := ( 'except' | 'catch' | '捕获' ) [ tryBinding ] block [ finallyPart ]
               | finallyPart ;
finallyPart   := ( 'finally' | '最终' ) block ;
tryBinding    := ident [ 'as' ident ] ;

breakStatement    := ( 'break' | '跳出' ) ;
continueStatement := 'continue' ;
returnStatement   := ( 'return' | '返回' ) [ expression ] ;
assignment        := expression ( '=' | '+=' | '-=' ) expression ;
multiAssignment   := expression ',' expression { ',' expression }
                     '=' expression { ',' expression } ;
expressionStatement := expression ;
```

说明：

- 语句分隔符是换行或 `;`，缩进不参与语法。
- `try` 至少要有 `except` 或 `finally` 之一。
- `except 值错误 as e` 中 `值错误` 是异常类型，`e` 是绑定名；`except e` 则把 `e` 当绑定名。
- `continue` 没有中文别名；`break` 可写 `跳出`。

### 6.2 表达式

```ebnf
expression    := exprOr ;

exprOr        := exprAnd { ( '∨' | 'or' ) exprAnd } ;
exprAnd       := exprNot { ( '∧' | 'and' ) exprNot } ;
exprNot       := ( '!' | 'not' ) exprNot | exprComparison ;

exprComparison := exprAdditive { comparisonOp exprAdditive } ;
comparisonOp  := '==' | '!=' | '<=' | '>=' | '<' | '>' | 'in' | 'contains' ;

exprAdditive  := exprMultiplicative { ( '+' | '-' ) exprMultiplicative } ;
exprMultiplicative := exprUnary { ( '*' | '/' | '%' ) exprUnary } ;
exprUnary     := '-' exprUnary | exprPostfix ;

exprPostfix   := exprPrimary { postfix } ;
postfix       := '[' indexOrSlice ']'
               | '.' ident [ '(' argList ')' ] ;

indexOrSlice  := expression
               | [ expression ] ':' [ expression ] [ ':' [ expression ] ] ;

exprPrimary   := number | stringLiteral
               | 'true' | '真' | 'false' | '假'
               | ident [ '(' argList ')' ]
               | '(' expression ')'
               | listExpr | mapExpr
               | '{' 'e' '}' ;

argList       := [ expression { ',' expression } ] ;

listExpr      := '[' ']'
               | '[' expression { ',' expression } ']'
               | '[' expression ( 'for' | '循环' ) ident 'in'
                 expression [ 'if' expression ] ']' ;

mapExpr       := '{' [ mapMember { ',' mapMember } [ ',' ] ] '}' ;
mapMember     := ( stringLiteral | ident ) ':' expression ;
```

说明：

- 比较运算符支持链式写法（`1 < x < 5`）。
- `.字段` 只用于路径；`.方法(...)` 是方法调用。
- 列表切片支持省略端点与步长：`[a:b]`、`[a:b:c]`、`[:b]`、`[::c]`。
- `{e}` 表示当前实体。
- 函数体字符串用单引号或双引号，支持 `\n`、`\t`，其余 `\x` 按字面 `x` 转义；数字只支持整数与 `小数`，不支持指数与下划线。

## 7. 关键字别名与运算符

| 英文 | 中文 |
|---|---|
| `if` | `如果` |
| `else` | `否则` |
| `else if` | `否则如果` |
| `for` | `循环` |
| `while` | `当` |
| `try` | `尝试` |
| `except` / `catch` | `捕获` |
| `finally` | `最终` |
| `break` | `跳出` |
| `continue` | 无 |
| `return` | `返回` |
| `true` / `false` | `真` / `假` |
| `and` / `or` / `not` | `∧` / `∨` / `!` |
| `in` | 无 |
| `contains` | 无 |

优先级（从低到高）：

```text
∨/or  →  ∧/and  →  !/not  →  比较（== != <= >= < > in contains）
→  + -  →  * / %  →  一元 -  →  . / [] / ()
```

## 8. 完整示例

```neko
// 模板
@定义 NPC {
  "身体": {
    "体力[int,@体力管理,min:0,max:100]": 75,
    "饱食[int,min:0,max:100]": 82
  },
  "关系": { "亲密[int,min:0,max:100]": 50 }
}

@定义 NPC:猫科 {
  "特征": { "猫科": true }
}

@定义 NPC:成年 {
  "年龄": 18
}

@实体 NPC:猫猫 [NPC:猫科, NPC:成年] {
  "毛色": "橘"
}

// 规则：键值式
@规则[$[NPC:猫猫]] {
  id: r_thirst
  优先级: 10
  条件: 身体.饱食 < 20
  动作: 状态.身体 = "饥饿"
  分支: {
    身体.体力 < 10 → 状态 = "昏厥"
    默认 → 状态 = "活跃"
  }
}

// 规则：条件块式
@规则[$[NPC]] {
  身体.体力 < 20 {
    $.状态.身体 = "疲惫"
  },
  身体.体力 < 10 {
    $.状态.身体 = "虚弱"
  }
}

// 行式规则
#规则: 身体.体力 < 5 → 状态.身体 = "昏厥"

// 事件
@事件:体力管理 {
  条件: 变化量 < -10 ∨ 变化量 > 20
  处理: 体力管理
}

// 函数
@函数 体力管理(变化量:int) -> int {
  if 变化量 < -30 {
    状态.身体 = "昏厥"
  } else if 变化量 < -20 {
    状态.身体 = "疲惫"
  } else {
    状态.身体 = "活跃"
  }
  返回 变化量
}
```
