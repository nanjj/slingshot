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

**QN（包限定名称）** 是最重要的概念——多个工具（`code_locate` / `code_find_references` / `trace_path`）
都基于 QN 定位符号。

## 索引策略

| 模式 | 速度 | 能力 | 适用场景 |
|------|------|------|---------|
| `fast` | 最快 | 仅过滤文件，无向量/语义 | 大型 monorepo 快速扫描 |
| `moderate` | 中等 | 过滤文件 + 语义/相似度 | 日常开发（默认推荐） |
| `full` | 最慢 | 全部文件 + 语义/相似度 | 首次索引 / 发布前全覆盖 |
| `cross-repo-intelligence` | 快 | 跨项目 Route/Channel 匹配 | 微服务跨仓库追踪 |

- 所有搜索/追踪工具都依赖索引，首次使用先 `index_repository`
- 跨仓库需两步：先单独索引各项目，再 `cross-repo-intelligence` 创建 `CROSS_HTTP_CALLS` 等跨项目边
- `persistence=true` 写入压缩制品，团队可直接加载免重复索引

## 工具概览：两套 API

当前有两套功能基本重叠的工具集。**优先用 `code_*`**（新版，统一命名风格），
`codebase_*` 是旧版，仅在后备场景使用。核心差异：

- 命名风格：`code_snake_case`（新）vs `codebaseCamelCase`（旧）
- `code_*` 的参数名是 `qualifiedName`/`project`，`codebase_*` 是 `qualified_name`/`project`
- 功能完全对应，新版的边界处理稍完善

如果你调用的工具找不到（例如用了 `code_search_graph` 但提示不存在），
回退到 `codebase_search_graph`。

## 场景总览

| # | 场景 | 工具链 | AI agent 需要知道的 |
|---|------|--------|-------------------|
| 1 | 初识项目 | `get_architecture` → `search_graph` → `get_code_snippet` | `aspects` 过滤（`packageDeps`/`hotspots`/`fileTree`）；Leiden 聚类揭示实际模块边界（常与目录不一致）；`includeNeighbors=true` 看函数上下文 |
| 2 | 调用链追踪 | `trace_path(mode=calls)` | 三模式：`calls`（默认调用链）→ `data_flow`（关注特定参数值传递）→ `cross_service`（跨 HTTP/async 路由）；`riskLabels` 按路径长度标记 CRITICAL~LOW；默认过滤测试文件，`includeTests=true` 包含 |
| 3 | 搜索代码 | `search_graph` / `search_code` / `query_graph` | BM25 支持 camelCase 分词 + 定义加权；`namePattern` 正则精确匹配；`semanticQuery` 为**数组**（`["send","publish"]`）做语义桥接；`minDegree`/`maxDegree` 按调用热度过滤；检测 `totalResults` 判断截断 |
| 4 | 引用分析 | `code_find_references` + `trace_path(inbound)` | `depth=0` 直接引用，`depth=1` 传递引用；QN 大小写敏感 |
| 5 | 评估变更 | `detect_changes(scope=impact)` | 支持 `since=` / `baseBranch=`；返回变更符号 + 图谱传播影响；基于 git，未 commit 的不检测 |
| 6 | 代码编辑 | `code_locate` → `code_edit` / `code_edit_body` | AST 感知编辑不破坏语法；selector 格式 `function:<name>`/`method:<name>`/`struct:<name>`；编辑前先定位，小步提交 |
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
- **search_code 截断**：返回 `totalGrepMatches`/`totalResults`，接近 limit 时加 file_pattern 缩小
- **trace_path 默认排除测试文件**：要看测试调用链加 `includeTests=true`
- **Cypher 100k 行上限**：宽泛查询务必加 `LIMIT`；分页用 `search_graph` 的 offset/limit
- **变更检测基于 git**：未 commit 的修改不纳入。先 commit 再 detect
- **首次使用先索引**：搜索/追踪工具都需要已索引的项目
- **search_graph 分页**：响应有 `has_more` 字段，true 表示结果被截断，递增 offset 继续翻页
- **code_locate 模糊匹配**：如果 QN 不精确，会返回候选列表——从候选里选对的确切 QN
- **code_edit 的 selector 大小写敏感**：selector 中的函数名/方法名/结构体名与源码完全一致
