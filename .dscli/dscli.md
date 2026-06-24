# Slingshot Code Intelligence — Phase 4 进度

## ✅ 已完成

### P4B (张衡) — search_code 新工具 ✅
- 新建 `internal/code/mcp/handler_search_code.go`
- 注册在 `server.go` (Search & Navigation 组)
- 功能: `rg` JSON 模式搜索 + 图节点去重 + 函数级富化 + 排序
- 三种模式: compact (默认), full (源码), files (仅路径)

### P4C (居里) — get_architecture 升级 ✅
- 新建 `internal/code/base/architecture.go` + `architecture_test.go`
- Hotspots: 按 fan-in 排序取 Top N
- FileTree: 目录分组统计
- PackageDeps: 跨包 CALLS 聚合
- handler_search.go: aspects 参数支持

### P4D (居里) — detect_changes 影响分析 ✅
- 新建 `internal/code/base/impact.go` + `impact_test.go`
- ImpactAnalysis: 沿 CALLS 双向传播变更影响
- handler_search.go: scope="impact" 返回 impactedSymbols
- 深度控制: 默认 2，最大 10

### P4E (居里) — trace_path 升级 ✅
- TracePath 替代 TraceCallChain (旧函数委托调用)
- 支持 mode: calls / data_flow (传递参数) / cross_service (HTTP_CALLS 边)
- RiskLabels: depth≤2→HIGH, 3-4→MEDIUM, >4→LOW
- IncludeTests: 默认过滤测试文件
- 新建 `tracepath_test.go`: 5 个测试

### P4A (张衡) — 附加改动
- `internal/code/base/crud.go`: 新增 `GetNodesByFile()` 方法
- `internal/code/mcp/server.go`: 注册 search_code

## 📋 待完成

### P4A (张衡) — search_graph BM25 结构提升 + 分页

## 测试状态

```
internal/code/base/  55 tests ✅  (34 原 + 9 P4C + 7 P4D + 5 P4E)
internal/code/lsp/   36 tests ✅
internal/code/mcp/   39 tests ✅
总计: 130 ✅
```
