# Slingshot code intelligence — 开发追踪

## 最新状态 (9131975)

✅ **Phase 1** (a23b482) — code_edit, code_edit_body, code_locate (base layer)
✅ **Phase 2** (08a98ef) — code_find_references, code_analysis, linear_scan_in_loop (6 files, +725/-10)
  - Curie: indexer + analyzer + MCP handlers + 5 integration tests
  - 39 tests 全部通过
✅ **Phase 3 — 3 个 MCP 缺口修复** (9131975) — 张衡
  - `index_repository.mode` — 支持 full/moderate/fast 三级过滤
  - `detect_changes` — git diff 真实实现（替代 stub）
  - `search_graph.semanticQuery` — BM25 保底搜索

### 当前工具 (28个)

| 类别 | 工具 | 说明 |
|------|------|------|
| 编辑 | code_edit, code_edit_body, code_locate | 基于 tree-sitter 的精确编辑 |
| 分析 | code_analysis, code_find_references | 复杂度分析 + 引用查找 |
| 搜索 | search_graph, search_code, get_code_snippet | BM25/语义/正则搜索 |
| 图 | query_graph, trace_path, get_architecture | Cypher/依赖追踪/架构 |
| AST | get_structure, get_node, get_text, get_definitions | AST 导航 |
| 项目 | index_repository, delete_project, list_projects, index_status | 项目管理 |
| 其他 | detect_changes, ingest_traces, manage_adr, get_graph_schema | 变更/ADR/写入 |
| 编辑器 | open_project, get_project_root, validate, query_ast | 编辑器基础设施 |
| 记忆 | save_memo, search_memos | 持久化记忆 |

### 测试覆盖
```
internal/code/base/  32 tests
internal/code/mcp/   42 tests (Phase 1: 37 + Phase 2: 5)
internal/code/lsp/    0 tests  ← 仍需补
```

## 待办

- [ ] **lsp 包测试**（analyer.go + lsp.go）→ 已分配居里
- [ ] **联调** — `slingshot code serve` + codebase-memory-mcp MCP client → 居里（lsp 测试之后）
- [ ] **多语言支持验证**（Python/JS/Rust/Java 等）→ 排期中
