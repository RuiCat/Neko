# 脚本目录分层与内容层（scripts/）

`scripts/` 只放**规则与内核定义**；实际实体数据（角色/物品/场景/事件条目）不放进来。

```text
scripts/
  kernel/                       # 规则与内核定义（唯一会被注册为脚本的部分）
    base/ metabolism/ physiology/ organs/ injury/ reproductive/ status/
    material/ interact/ action/ plan/ education/ trade/ memory/
    behavior/ sleep/ events/ world/ scene/ location/ relation/ task/ traits/
    personality/ growth/ curiosity/ social/ environment/ expression/
    event/                      # 事件底层（L13；事件数据属私有参考，不入 private/）
rules/                        # 规则库（L14）：任务/行为等业务规则，只写 @规则/@函数
private/                        # 私有内容蓝图（.gitignore，不进版本库）
  characters/                   # 私有角色数据
  events/                       # 私有事件数据（事件模板实例）
```

- `scripts/kernel/**` 会被注册表扫描并 apply（规则/模板/函数）。
- `private/**` **不在 scripts 下、不注册为脚本**，只由 `neko-data apply` 读取。
- 仓库里没有任何角色/物品/场景/事件的实例数据；它们要么是本机 `private/`，要么由数据层工具生成。
- **任务与行为用规则表达**：写在 `scripts/rules/`（`@规则`/`@函数`），不写 `@实例`，也不放进 `scripts/kernel/`；
  状态字段由内核提供（如 `任务状态`、`行为.主动`），规则只做状态机与结算。

## 1. 内容文件格式

一个文件可以放多个实例块，块名是 `类型:实体ID`（类型必须在内核里有模板或继承链）：

```neko
// scripts/entities/foods/food.neko
@实例 食物:苹果 {
  "名称": "苹果",
  "类别": "食物",
  "数量": 3,
  "重量": 180,
  "价格": 5,
  "描述": "一颗脆苹果",
  "食物": { "热量": 95, "水分": 85, "饱腹": 20, "保质期": 10080, "可食": true }
}
```

### 类型与实例：`@实体` 建类型，`@实例` 建实体

关键字有三套：**`@定义` 建纯模板**，**`@实体` 建组合类型**，**`@实例` 建实体**。
旧写法 `@Data`（模板别名）与 `@data`（实例）已移除，写成它们会**直接报错并提示替代写法**；
任何未知的 `@关键字` 同样在解析期报错，不再静默跳过整块内容。

**冒号只表示命名空间/分类路径，不表示继承/从属**：字段组合只来自方括号列表。

```neko
// 类型：名字可带冒号（命名空间），字段 = [..] 里列出的模板组合
@实体 角色定义:猫娘 [角色基础, 猫娘, 猫耳, 猫尾, 肉垫] {}

// 实体：末段是实体名，前面是分类路径；字段来自 [] 里的类型
@实例 角色:猫猫 [角色定义:猫娘] { "基础": { "名字": "猫猫", "性别": "女", "年龄": 20 } }
@实例 角色:梦梦 [角色定义:猫娘] { "基础": { "名字": "梦梦", "性别": "女", "年龄": 1 } }
```

- 实体的 `type` 存**分类路径**（上例为 `角色`），`types` 存 `[角色定义:猫娘]`；
- `neko-data apply` 创建实体时把 `[]` 里类型的模板链合并进实体数据，再叠加块体里的值；
- 规则选择器按分类路径前缀匹配：`$[角色]` 命中 `角色:猫猫`、`角色:梦梦`；
- 不带 `[]` 时仍按旧式 `类型:名称` 工作（如 `@实例 食物:苹果 { … }`，类型 `食物`）。

**`@实例` 里不能写字段注解**。注解（`"体力[int,min:0,max:100]": 75`）只在模板里展开；
写在 `@实例` 里会把整个字符串当成字段名写进实体，真正的 `体力` 反而没有赋值。
解析器会直接拒绝并指出具体路径。

| 写法 | 实体 type | id | 说明 |
|---|---|---|---|
| `@实例 角色:猫猫 [角色定义:猫娘] {…}` | `角色` | 猫猫 | 猫娘，`$[角色]` 命中 |
| `@实例 角色:梦梦 [角色定义:猫娘] {…}` | `角色` | 梦梦 | 同上 |
| `@实例 食物:苹果 {…}` | `食物` | 苹果 | 旧式：类型即块名冒号前 |
| `@实例 事件模板:1-1初次口交 {…}` | `事件模板` | 1-1初次口交 | 事件数据 |

#### 模型：基础 + 修饰，修饰写自己的状态

`角色基础` 是**完整的基础角色定义**（组合全部内核），自身不带修饰时就是人族。
种族、生命阶段、外形特征都是**修饰模板**，通过组合叠加在同一份自身数据上：

```neko
@实体 角色基础 [基础内核, …, 任务状态] {}                       // 基础（内核组合）
@实体 角色定义:猫娘 [角色基础, 猫娘, 猫耳, 猫尾, 肉垫] {}         // 基础 + 猫娘修饰
@实体 角色定义:猫娘:幼年 [角色基础, 猫娘, 猫耳, 猫尾, 肉垫, 幼年] {}  // 再叠加幼年修饰
```

关键点：**修饰不依赖人族**。`猫娘` 这个修饰只声明自己的标识字段（`种族标识 = "猫娘"`），
再由同名规则把 `种族参数.* / 基础.种族 / 基础.特性` 写到**自身**
（见 `scripts/kernel/traits/traitRules.neko`）。它不继承人族，只要求自身满足
`角色基础` 的字段定义——组合时已保证。`幼年` 同理，只把自身状态标成幼年，不碰种族。

因此叠加是正交的：`角色定义:猫娘:幼年` 里，种族来自猫娘修饰、生命阶段来自幼年修饰，
互不覆盖。任意「基础 + 若干修饰」都是合法类型，不要求凑齐某族全套（如 `角色定义:猫耳`）。

#### 种族从哪来

| 情况 | 种族 | 来源 |
|---|---|---|
| 无种族修饰（裸 `角色基础`、只加外形特征） | 人族 | `default` |
| 组合了 `人族/猫娘/犬娘/狐娘/精灵` 修饰 | 对应种族 | `trait` |

规则：

1. **块体必须是合法 JSON**（不能写注解、不能有尾逗号、键必须带引号）。
2. **实体 ID 全局唯一**：跨文件重复会让 `apply` 直接失败（不静默覆盖）。
3. **分类是块名最后一个冒号前的部分**：`食物` / `角色` / `用户:人类` 都行；
   带 `[]` 时它只作分类用，字段由 `[]` 里的类型提供。
4. **内容文件里不要写 `@规则`/`@函数`/`@定义`**：规则属于 `kernel/`，内容只描述实例。
5. `类型` 是 `素材` 内核的普通字段（`"物品"`），不是实体类型；实体类型只由块名决定。

## 2. 私有内容（角色与事件数据）

`private/` 被 `.gitignore` 排除，只在本机存在：

- `private/characters/*.neko`：私有角色数据，块名用 `分类:名称` + `[类型…]`（`@实例 角色:猫猫 [角色定义:猫娘] { ... }`）；
- 事件语料（如 `性行为事件.neko`）是**参考材料，不是脚本**：放在 `archive/reference/`，
  不参与 `neko-data apply`（`private/` 才会被扫描）。事件底层 `scripts/kernel/event/` 只按
  「角色自己的 事件.当前」推进阶段，任何组合了 `事件内核` 的实体都能用；
- 私有人设/称呼/对话偏好也放这里；
- `neko-data apply` 默认只读 `private/`（存在才读），`-content` 可指定别的蓝图目录；
- **备份**：整仓 tar 备份必须排除 `private/`。

## 3. 内容 → 实体：`neko-data apply`（内容蓝图在 private/）

```bash
make build                      # 产出 bin/neko-data
./bin/neko-data -data-dir ~/.dsh-neko/Neko/data -scripts-dir ~/.dsh-neko/Neko/scripts apply -dry-run
./bin/neko-data -data-dir ~/.dsh-neko/Neko/data -scripts-dir ~/.dsh-neko/Neko/scripts apply
```

- 默认只读 `<repo>/private`；`-content <dir>` 可指定其他蓝图目录。
- **幂等且不覆盖**：已存在的实体默认 `skip`，不会把角色/物品的运行时状态（持有、耐久、腐败、位置）洗掉。
- `-overwrite` 只写内容块里出现的字段（叶子路径逐个 `Set`），其余状态保留。
- `-dry-run` 只校验并列出 would create / would update。
- 保存走 `store.Save()`：公开实体进 `entities.json`，`角色基础*` 进 `user/entities.json`。

## 4. 蓝图与存档的边界

| 位置 | 性质 | 谁写 |
|---|---|---|
| `scripts/kernel/**` | 规则与内核（含事件底层、类型账本） | 人写、进版本库 |
| `private/**`（本机） | 内容蓝图（初始字段，私密） | 人写、**不进版本库** |
| `data/entities.json`、`data/user/entities.json` | 运行时存档（状态） | 引擎写、不进版本库 |
| `data/templates/`、`data/scripts.json` | 派生索引（从 kernel 重建） | 引擎写、可删 |

因此：**调数值/加物品改 `scripts/`，游戏进程产生的状态留在 `data/`**；要换机或回滚内容，
`neko-data apply` 把蓝图重新物化即可（已存在的实体保持原状态）。

## 5. 与内核层的分界

- 物品/食物的**字段与结算规则**在 `scripts/kernel/material/`、`scripts/kernel/interact/`（见
  [material-layer.md](material-layer.md)）；
- 行动/规划/教育/交易/记忆的内核在 `scripts/kernel/{action,plan,education,trade,memory}/`
  （见 [cognition-layers.md](cognition-layers.md)）；
- 内容层只提供实例，不承担任何规则逻辑；
- 公共资料库**按条目拆分**为带类型模板的实例（`任务模板`/`行为模板`/`性行为流程`/`性行为事件`），
  一个条目一个 `@实例` 块（`scripts/entities/library/`，52 条），规则引擎可按类型逐条读取；
  通用容器 `资料库` 只留给暂时无法归类的条目。

## 6. 验证

```bash
GOCACHE=$PWD/.local/gocache go test ./cmd/neko-data ./internal/registry -count=1
./bin/neko-data -data-dir /tmp/x -scripts-dir scripts extract -from 旧scripts目录 -out .local/legacy-extract
# extract 会按条目拆分资料库：任务模板/行为模板/性行为流程/性行为事件
GOCACHE=$PWD/.local/gocache go run ./.local/probe        # 内核冒烟（不读内容层）
./bin/neko-data -data-dir /tmp/x -scripts-dir scripts apply -dry-run
```
