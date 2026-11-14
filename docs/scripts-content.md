# 脚本目录分层与内容层（scripts/）

`scripts/` 只放**规则与内核定义**；实际实体数据（角色/物品/场景/事件条目）不放进来。

```text
scripts/
  kernel/                       # 规则与内核定义（唯一会被注册为脚本的部分）
    base/ metabolism/ physiology/ organs/ injury/ reproductive/ status/
    material/ interact/ action/ plan/ education/ trade/ memory/
    behavior/ sleep/ events/ world/ scene/ location/ relation/ task/ traits/
    personality/ growth/ curiosity/ social/ environment/ expression/
    event/                      # 事件底层（L13，事件数据在 private/events）
rules/                        # 规则库（L14）：任务/行为等业务规则，只写 @规则/@函数
private/                        # 私有内容蓝图（.gitignore，不进版本库）
  characters/                   # 私有角色数据
  events/                       # 私有事件数据（事件模板实例）
```

- `scripts/kernel/**` 会被注册表扫描并 apply（规则/模板/函数）。
- `private/**` **不在 scripts 下、不注册为脚本**，只由 `neko-data apply` 读取。
- 仓库里没有任何角色/物品/场景/事件的实例数据；它们要么是本机 `private/`，要么由数据层工具生成。
- **任务与行为用规则表达**：写在 `scripts/rules/`（`@规则`/`@函数`），不写 `@data`，也不放进 `scripts/kernel/`；
  状态字段由内核提供（如 `任务状态`、`行为.主动`），规则只做状态机与结算。

## 1. 内容文件格式

一个文件可以放多个实例块，块名是 `类型:实体ID`（类型必须在内核里有模板或继承链）：

```neko
// scripts/entities/foods/food.neko
@data 食物:苹果 {
  "名称": "苹果",
  "类别": "食物",
  "数量": 3,
  "重量": 180,
  "价格": 5,
  "描述": "一颗脆苹果",
  "食物": { "热量": 95, "水分": 85, "饱腹": 20, "保质期": 10080, "可食": true }
}
```

规则：

1. **块体必须是合法 JSON**（不能写注解、不能有尾逗号、键必须带引号）。
2. **实体 ID 全局唯一**：跨文件重复会让 `apply` 直接失败（不静默覆盖）。
3. **类型是块名冒号前的部分**：`食物` / `饮品` / `工具` / `容器` / `衣物` / `场景` / `NPC:猫娘` 都行
   （按最后一个冒号切分，所以带冒号的模板名可用）。
4. **内容文件里不要写 `@规则`/`@函数`/`@定义`**：规则属于 `kernel/`，内容只描述实例。
5. `类型` 是 `素材` 内核的普通字段（`"物品"`），不是实体类型；实体类型只由块名决定。

## 2. 私有内容（角色与事件数据）

`private/` 被 `.gitignore` 排除，只在本机存在：

- `private/characters/*.neko`：私有角色数据（`@data NPC:猫猫 { ... }`）；
- `private/events/*.neko`：私有事件数据（`@data 事件模板:<名称> { 阶段流程/对话/效果 }`），
  由旧语料抽出，**与角色无关**：事件底层 `scripts/kernel/event/` 只按「角色自己的 事件.当前」
  推进阶段，任何组合了 `事件内核` 的实体都能用；
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
- 保存走 `store.Save()`：公开实体进 `entities.json`，`NPC*` 进 `user/entities.json`。

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
  一个条目一个 `@data` 块（`scripts/entities/library/`，52 条），规则引擎可按类型逐条读取；
  通用容器 `资料库` 只留给暂时无法归类的条目。

## 6. 验证

```bash
GOCACHE=$PWD/.local/gocache go test ./cmd/neko-data ./internal/registry -count=1
./bin/neko-data -data-dir /tmp/x -scripts-dir scripts extract -from 旧scripts目录 -out .local/legacy-extract
# extract 会按条目拆分资料库：任务模板/行为模板/性行为流程/性行为事件
GOCACHE=$PWD/.local/gocache go run ./.local/probe        # 内核冒烟（不读内容层）
./bin/neko-data -data-dir /tmp/x -scripts-dir scripts apply -dry-run
```
