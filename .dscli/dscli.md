# Slingshot code intelligence — 开发追踪

## 最新状态 (2026-06-24)

✅ **Phase 1** (a23b482) — code_edit, code_edit_body, code_locate (base layer)
✅ **Phase 2** (08a98ef) — code_find_references, code_analysis, linear_scan_in_loop
✅ **Phase 3** (9131975) — 3 个 MCP 缺口修复 (张衡)
✅ **Task A** (e73eab2) — lsp 包测试 36 个 (居里)
✅ **Task C** — 多语言验证 (Python/JS/Rust/Java) 全部通过 (张衡)
📬 **Task D** — 联调中，等待张衡 MCP client 端到端验证
🔵 **Phase 4 追赶计划** — 9 个能力缺口，5 个子任务

### Phase 4 任务分工

| 任务 | 负责人 | 优先级 | 状态 |
|------|--------|:------:|:----:|
| P4C — get_architecture 升级 (热点/文件树/包依赖) | 居里 | 🔴 P0 | **进行中** |
| P4D — detect_changes 影响分析 | 居里 | 🔴 P0 | 待开始 |
| P4E — trace_path 升级 (data_flow/cross_service/risk_labels) | 居里 | 🟡 P1 | 待开始 |
| P4B — search_code 新工具 (grep+图增强) | 张衡 | 🔴 P0 | 待开始 |
| P4A — search_graph 增强 (BM25 结构提升+分页) | 张衡 | 🟡 P1 | 待开始 |

### 当前测试覆盖
```
internal/code/base/  34 tests
internal/code/lsp/   36 tests
internal/code/mcp/   39 tests
```
**总计：109 个测试，全部通过**

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

## 待办
- [ ] **联调** — `slingshot code serve` + dscli MCP client（张衡 + 居里协调）
- [ ] **P4C** — get_architecture 升级（居里）
- [ ] **P4D** — detect_changes 影响分析（居里）
- [ ] **P4E** — trace_path 升级（居里）
- [ ] **P4B** — search_code 新工具（张衡）
- [ ] **P4A** — search_graph 增强（张衡）
