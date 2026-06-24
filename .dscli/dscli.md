[07d48d8] ✅ Phase 1 + Phase 2 合并提交，22 files，3712 lines
- Curie 完成了 mcp/ 包（8 文件）和 code serve CLI（cmd/code.go）
- 已回复 Curie 邮件，建议下一步方向：indexer 测试 vs 联调
在 `internal/code/` 下创建 `base`（图存储）和 `lsp`（AST分析）两个包，为 Phase 2（Curie 的 mcp + CLI）打下基础。

## 模块结构
```
internal/code/
  base/        SQLite 图存储 + tree-sitter 索引引擎
    store.go    Store 类型、OpenStore、Close
    schema.go   DDL（projects/nodes/edges/memos/traces + FTS5）
    models.go   数据模型（Node, Edge, Project, TraceHop 等）
    crud.go     Node/Edge CRUD + FTS5 搜索
    graph.go    图遍历（递归 CTE：trace, references, query_graph）
    indexer.go  项目索引引擎（walk → parse → extract → store）
    indexer_test.go
  lsp/         AST 分析层（从 internal/editor/ 提炼，去掉写/生命周期）
    analyzer.go Analyzer 类型、NewAnalyzer
    lsp.go      核心方法：ParseFile, GetStructure, GetNode, GetDefinitions, Validate, QueryAST
    analysis.go 复杂度分析（cyclomatic, cognitive, loop_depth）
    analysis_test.go
```

## 执行路线

### Step 1: base/store.go + schema.go + models.go
- OpenStore (modernc.org/sqlite)
- Schema DDL（7张表 + FTS5 + 索引）
- Node/Edge/ProjectInfo/IndexResult 类型

### Step 2: base/crud.go
- SaveNode, GetNodeByQN, FindSymbols, SearchNodes
- SaveEdge
- ProjectInfo CRUD

### Step 3: base/graph.go
- GetReferences (递归 CTE)
- TraceCallChain (双向递归 CTE)
- QueryGraph (raw SQL)

### Step 4: base/indexer.go
- Walk project 目录
- tree-sitter 解析每个文件
- 提取符号（函数、方法、类、结构体、接口）
- 计算复杂度
- 构建调用关系
- 批量写入 SQLite

### Step 5: lsp/analyzer.go + lsp.go + analysis.go
- ParseFile: 解析文件，返回 tree
- GetStructure: 层次化结构
- GetNode: 按位置获取节点
- GetDefinitions: 提取定义标签
- Validate: 语法检查
- QueryAST: S-表达式查询
- Analyze: 圈复杂度、认知复杂度、循环深度

- [x] Step 1: base schema + store
- [x] Step 2: base CRUD
- [x] Step 3: base graph traversal
- [ ] Step 4: base indexer
- [x] Step 5: lsp analyzer
