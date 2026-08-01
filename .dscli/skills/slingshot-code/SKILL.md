---
name: slingshot-code
description: AI 驱动的代码智能 — 搜索、图谱、AST 分析、编辑、架构洞察、项目索引，27 个 MCP 工具
keywords: slingshot, code, MCP, code search, graph, AST, architecture, intelligence, 代码搜索, 图谱, 代码分析, 架构, 索引
author: JUN JIE NAN <nanjunjie@gmail.com>
auto_inject: true
---

# slingshot code — 代码智能 MCP 服务

构建代码知识图谱（符号节点 + 调用/导入/包含/实现边），
支持图搜索、AST 分析、调用追踪、变更检测。

**QN（包限定名称）** 是最重要的概念——多个工具（`locate` / `find_references` / `trace_path`）
都基于 QN 定位符号。

## 项目绑定（重要）

- 工具名**没有** `code_`/`codebase_` 前缀：`search_code`、`edit`、`locate`、`find_references` 等
- **project 参数可选**：先 `open_project`（接受项目名/根路径/路径后缀，如 `dscli`、`/home/me/src/dscli`）或 `index_repository` 绑定后，后续调用可省略 project
- **绑定跨会话持久**：open_project 的绑定写入状态库，服务重启/panic 后自动恢复——无需重新绑定
- **not found 时错误会附带可用项目列表**，照抄其中的名字即可
- 参数别名（LLM 直觉命名也可用）：`regex`=pattern、`file_pattern`=filePattern、`file`=pathFilter（search_code）、`function`=functionName、`symbol`=qualifiedName、`path`=repoPath、`cypher`=query

## 索引策略

| 模式 | 速度 | 能力 | 适用场景 |
|------|------|------|---------|
| `fast` | 最快 | 仅过滤文件，无向量/语义 | 大型 monorepo 快速扫描 |
| `moderate` | 中等 | 过滤文件 + 语义/相似度 | 日常开发（默认推荐） |
| `full` | 最慢 | 全部文件 + 语义/相似度 | 首次索引 / 发布前全覆盖 |
| `cross-repo-intelligence` | 快 | 跨项目 Route/Channel 匹配 | 微服务跨仓库追踪 |

- 所有搜索/追踪工具都依赖索引，首次使用先 `index_repository`
- `index_repository` 成功后自动绑定该项目
- 索引过期（文件已删/改名）：报错会提示 `run index_repository to refresh`，重新索引即可
- `persistence=true` 写入压缩制品，团队可直接加载免重复索引

## 场景总览

| # | 场景 | 工具链 | AI agent 需要知道的 |
|---|------|--------|-------------------|
| 1 | 初识项目 | `get_architecture` → `search_graph` → `get_code_snippet` | `aspects` 过滤（`packageDeps`/`hotspots`/`fileTree`）；Leiden 聚类揭示实际模块边界（常与目录不一致）；`includeNeighbors=true` 看函数上下文 |
| 2 | 调用链追踪 | `trace_path(mode=calls)` | 三模式：`calls`（默认调用链）→ `data_flow`（关注特定参数值传递）→ `cross_service`（跨 HTTP/async 路由）；`riskLabels` 按路径长度标记 CRITICAL~LOW；默认过滤测试文件，`includeTests=true` 包含；**方法调用已解析 receiver**（`h.dispatchToServer` → `pkg.dispatchToServer`），未解析的调用按方法名兜底匹配 |
| 3 | 搜索代码 | `search_graph` / `search_code` / `query_graph` | BM25 支持 camelCase 分词 + 定义加权，**自然语言查询可用**（`"register tool"` 命中 `RegisterTool`）；`namePattern` 正则精确匹配；`semanticQuery` 为**数组**（`["send","publish"]`）做语义桥接；`minDegree`/`maxDegree` 按调用热度过滤；检测 `totalResults` 判断截断 |
| 4 | 引用分析 | `find_references` + `trace_path(inbound)` | `depth=0` 直接引用，`depth=1` 传递引用；QN 大小写敏感；**方法引用按方法名聚合**——查 `pkg.method` 也能找到 receiver 无法解析的调用（metadata 含 `receiver`/`method` 可区分） |
| 5 | 评估变更 | `detect_changes(scope=impact)` | 支持 `since=` / `baseBranch=`；返回变更符号 + 图谱传播影响；基于 git，未 commit 的不检测 |
| 6 | 代码编辑 | `edit`（推荐） / `edit_body` / `locate` | **文本替换**：`{mode:'replace', oldText:'foo()', newText:'bar()'}`，服务端自动定位，重复出现时加 `occurrence=N`；结构化模式用字节偏移 `startByte`/`endByte` 或 selector；`edit_body` 的 selector 格式 `function:<name>`/`method:<name>`/`struct:<name>`；编辑前先 `locate`，小步提交 |
| 7 | 模式分析 | `query_graph(Cypher)` | 见下方实用模式；有 100k 行硬上限，加 LIMIT；不支持 offset |
| 8 | 持久化 | `save_memo` / `manage_adr` / `search_memos` | 频繁搜索的知识存为 memo；ADR 记录架构决策；跨会话避免重复劳动 |
| 9 | 文件浏览 | `get_definitions` / `get_structure` / `validate` / `get_text` | 按需使用，无特殊边界条件 |

### 场景选择速查

不知道用哪个工具时，按这个决策树：

- 想**了解项目结构** → 场景 1（初识项目）
- 想**找某个函数/类的定义或调用者** → 场景 4（引用分析）或场景 3（搜索代码）
- 想**跟踪一条调用链路** → 场景 2（调用链追踪）
- 想**改代码** → 场景 6（代码编辑）
- 想**做全局分析（循环复杂度、死代码、耦合）** → 场景 7（Cypher）
- 想**保存知识供下次会话使用** → 场景 8（memo/ADR）

## 实用 Cypher 模式

以下查询直接复制执行：

```cypher
// 高复杂度函数（重构候选）
MATCH (f:Function) WHERE f.cyclomatic > 10
RETURN f.qualifiedName, f.cyclomatic ORDER BY f.cyclomatic DESC LIMIT 20

// 隐藏 O(n²)：循环中线性扫描 / 深层嵌套循环
MATCH (f:Function) WHERE f.linearScanInLoop >= 1 OR f.transitiveLoopDepth >= 3
RETURN f.qualifiedName, f.linearScanInLoop, f.transitiveLoopDepth

// 死代码（未被调用的非 init 非测试函数）
MATCH (f:Function) WHERE NOT (f)-[:CALLS]->() AND NOT ()-[:CALLS]->(f)
  AND NOT f.qualifiedName CONTAINS "init" AND NOT f.qualifiedName CONTAINS "Test"
RETURN f.qualifiedName LIMIT 50

// 跨模块耦合热力图
MATCH (f1:Function)-[:CALLS]->(f2:Function) WHERE f1.package <> f2.package
RETURN f1.package AS source, f2.package AS target, count(*) AS weight
ORDER BY weight DESC LIMIT 20

// 入参过多（设计坏味）
MATCH (f:Function) WHERE f.paramCount >= 6
RETURN f.qualifiedName, f.paramCount ORDER BY f.paramCount DESC LIMIT 20

// 循环内内存分配（性能隐患）
MATCH (f:Function) WHERE f.allocInLoop > 0
RETURN f.qualifiedName, f.allocInLoop ORDER BY f.allocInLoop DESC LIMIT 20

// 热门调用点（被最多函数调用）
MATCH (f:Function)<-[:CALLS]-() RETURN f.qualifiedName, count(*) AS callers
ORDER BY callers DESC LIMIT 20
```

可用节点属性：`cyclomatic`, `cognitive`, `loopDepth`, `linearScanInLoop`,
`paramCount`, `recursive`, `allocInLoop`, `file`, `package`, `qualifiedName`

## 关键注意事项

- **QN 大小写敏感**：`myapp.Pkg.Func` ≠ `myapp.pkg.func`. 如果定位不到，试试调整大小写
- **search_code 截断**：返回 `totalMatches`/`totalResults`，接近 limit 时加 `file_pattern` 缩小
- **search_code 二进制安全**：自动跳过 ELF/PE/Mach-O/Java 二进制（magic 检测 + 扩展名 + `_release`/`dist`/`build` 等目录），不会 panic 或混入二进制噪声
- **trace_path 默认排除测试文件**：要看测试调用链加 `includeTests=true`
- **Cypher 100k 行上限**：宽泛查询务必加 `LIMIT`；分页用 `search_graph` 的 offset/limit
- **变更检测基于 git**：未 commit 的修改不纳入。先 commit 再 detect
- **首次使用先索引**：搜索/追踪工具都需要已索引的项目；`index_repository` 后自动绑定
- **search_graph 分页**：响应有 `has_more` 字段，true 表示结果被截断，递增 offset 继续翻页
- **locate 模糊匹配**：如果 QN 不精确，会返回候选列表——从候选里选对的确切 QN
- **edit 优先用文本替换**：`oldText`+`newText`（或 `text`），重复时加 `occurrence`；selector/字节偏移是高级模式
- **schema 默认 strict**（additionalProperties:false + required）：幻觉字段会被拒绝——只用文档/别名表中的参数名（`regex`、`file_pattern`、`symbol`、`function`、`cypher`、`path` 等都是真实 schema 字段，照常可用）；有别名的主字段（如 `pattern`）不强制 required，传别名即可
- **方法调用边带元数据**：CALLS 边 metadata 含 `callee`（原始文本）+ `receiver`/`method`（方法调用）或 `pkg`（包函数调用），可用 query_graph 的 `json_extract` 过滤
