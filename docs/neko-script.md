# `.neko` 脚本语言

`.neko` 是 AstrBox 的块级脚本方言，用来声明实体模板、状态机规则、字段事件与函数。文件按 UTF-8 文本读取，`//` 与行首 `#` 为注释。

## 顶层块

| 块 | 形式 | 产物 |
|---|---|---|
| 实体模板 | `@Data NPC:猫猫 { ...JSON... }` | 模板 `NPC:猫猫` |
| 通用数据块 | `@data 名称 { ... }` | 不编译为模板 |
| 规则块 | `@规则[...] { ... }` | 一条或多条规则 |
| 行式规则 | `#规则: 条件 → 动作` | 一条规则 |
| 事件 | `@事件 名称 { 条件: ... 处理器: ... }` | `EventDef` |
| 函数 | `@func 名称(a:int, b:int):int { ... }` | `FuncDef` |
| 元数据 | `#世界观: ...` | 元数据（不参与执行） |

`@Data` 的块体必须是合法 JSON；解析失败会记录错误，但同一文件里的规则仍会被应用（fail-soft，与 Python 版一致）。

## 规则主语

规则头用方括号选择作用对象：

```neko
@规则[$[NPC]]        { ... }   // 类型 NPC 的全部实体，match = "NPC[*]"
@规则[$]             { ... }   // 全部实体，match = "[*]"
@规则[场景]          { ... }   // 全局实体「场景」
@规则[$1[NPC], $2[NPC]] { ... } // 多主体，$1/$2 在动作里交叉绑定
@规则[NPC.性别=女]   { ... }   // 带属性条件
```

单主体规则里，`$` 与 `$[类型]` 在编译期重写为 `{e}`（当前实体）；多主体的 `$1`/`$2` 保留，由引擎按笛卡尔积绑定。

## 规则体

```neko
@规则[$[NPC]] {
  id: r_thirst              // 可选，缺省用内容哈希
  优先级: 10                // 可选，数值越大越先执行
  状态: 状态.身体 = "活跃"   // 可选，作为 when 的一部分
  条件: 身体.水分 < 20
  动作: 状态.身体 = "口渴"
      标记.口渴 = true
  分支: {
    身体.体力 < 10 → 状态 = "昏厥"
    身体.体力 < 20 → 状态 = "疲惫"
    默认 → 状态 = "活跃"
  }
}
```

- `条件` / `状态` 参与 `when`；多个条件用 `∧` 连接。
- `分支` 从上到下短路，第一个命中生效；`默认` 为兜底。
- 分支行 `状态 = "昏厥"` 是语法糖，编译为 `state: {"身体": "昏厥"}`；`状态.情绪 = "难过"` 写入 `情绪`。
- 只有分支没有条件时，分支的 `if` 即条件；分支和动作都为空时编译报错。

### 条件表达式

支持 `∧`/`∨`、`!`、括号、比较（`= == != > >= < <=`）、`in`、`contains`、集合字面量、路径引用与函数调用。

裸字段会被补全到正确分组：

```neko
体力 < 10            → {e}.身体.体力 < 10
主人行为 in {抚摸}    → {e}.行为.主人行为 in {抚摸}
疼痛 > 95 ∨ 出血 > 70 → {e}.状态.疼痛 > 95 ∨ {e}.伤害.出血 > 70
```

裸字段到分组的映射表在 `internal/compiler/rewrite.go` 的 `BareFieldMap`；未命中时默认挂 `身体`。`场景`/`世界`/`时间`/`天气`/`全局` 开头的引用保持原样。

### 动作

| 写法 | 编译结果 |
|---|---|
| `状态.身体 = "疲惫"` | `{"op":"set","path":"{e}.状态.身体","value":"疲惫"}` |
| `身体.体力 += 5` | `{"op":"add","path":"{e}.身体.体力","value":5}` |
| `身体.体力 -= 5` | `{"op":"add","path":"{e}.身体.体力","value":-5}` |
| `场景.事件 += "下雨"` | `{"op":"append","entity":"场景","path":"事件","value":"下雨"}` |
| `$1.亲密 += 5` | `{"op":"add","subject":1,"path":"亲密","value":5}` |
| `{e}.标记.疲劳 = true` | 原样路径 |
| 动作值 `体力`（裸路径） | 编译为 `$体力` 引用，运行时解析 |

字符串必须加引号；裸路径（形如 `a.b.c`）会被当作引用，数字/布尔保持字面量。

### 行式规则

```neko
#规则: 身体.体力 < 20 → 状态.身体 = "疲惫"
```

`→` 左侧是条件，右侧是动作；适合一行能表达的小规则。

## 字段注解

`@Data` 的 JSON 键可带注解，用于默认值、类型、事件与范围：

```neko
@Data NPC {
  "体力[int,@体力管理,min:0,max:100]: 75"
}
```

编译展开为字段 `体力 = 75` 与注解表：

```json
{"身体.体力": {"type": "int", "event": "体力管理", "min": 0, "max": 100}}
```

支持的类型/约束：`int` `float` `str` `bool`、`@事件名`、`enum:a;b;c`、`min:N`、`max:N`、`readonly`、`nullable`。子类模板覆盖字段时，未写的注解从父类继承。

## 函数与事件

```neko
@func 加(a:int, b:int):int {
  返回值 a + b
}

@事件 状态变化 {
  条件: 变化量 > 0
  处理器: onStateChange
}
```

Go 版已接入函数执行器与事件总线：字段注解绑定的事件在写入时触发，条件可用 `变化量`/`旧值`/`新值`，处理器是同名或 `处理器:` 指定的 `@函数`。事件触发深度上限 10，防止处理器互相递归。

`@函数` 支持变量、`if/else if/else`、`while`、`try { ... } except 错误 { ... } finally { ... }`（可写 `except 值错误 as e` 按类型捕获）、`break`/`continue`、`返回`、`for 项 in 列表/文本/映射`、`for i, v in enumerate(...)`、元组赋值 `a, b = 1, 2`、map 字面量 `{"键": 值}`、字符串拼接、切片（含步长 `[a:b:c]`）、链式比较（`1 < x < 5`）、`int()`/`float()`（非法输入报错）、`in`/`contains`、函数互调，以及常用方法：列表 `append/extend/insert/remove/pop/clear/count/index/sort/reverse`，文本 `format`（`{}`/`{0}`/`{:.2f}`/`{:>5}`/`{!r}`）/`join/split/rsplit/strip/replace/lower/upper/startswith/endswith/find/rfind/index/rindex/count/partition/removeprefix/removesuffix/title/capitalize/casefold/expandtabs/isdigit/isalpha/isnumeric/isalnum/isspace/islower/isupper/istitle/isascii/isprintable`，映射 `keys/values/items/get/update/setdefault/copy`；支持列表推导 `[x for x in ... if ...]` 与 `range/enumerate/zip/sum/sorted/reversed`。对当前实体（`状态.身体`）和变量绑定实体（`素材.所属场景`）的读写同样可用。

## 解析与错误

`internal/neko.Parse` 逐块解析并 fail-soft：单个块的错误进入 `ScriptAST.Errors`，不影响后续块。注册表会把这些错误作为 warning 返回，但继续应用可解析的规则。
