# Slingshot Code Intelligence — Phase 4 进度

## ✅ 已完成

### P4B (张衡) — search_code 新工具 ✅
- 新建 `internal/code/mcp/handler_search_code.go`
- 注册在 `server.go` (Search & Navigation 组)
- 功能: `rg` JSON 模式搜索 + 图节点去重 + 函数级富化 + 排序
- 三种模式: compact (默认), full (源码), files (仅路径)
- 参数: pattern, project, filePattern, pathFilter, context, mode, limit

### P4C (居里) — get_architecture 升级 ✅
- 新建 `internal/code/base/architecture.go` + `architecture_test.go`
- Hotspots: 按 fan-in 排序取 Top N
- FileTree: 目录分组统计
- PackageDeps: 跨包 CALLS 聚合
- handler_search.go: aspects 参数支持

### P4A (张衡) — 附加改动
- `internal/code/base/crud.go`: 新增 `GetNodesByFile()` 方法（按文件路径查节点）
- `internal/code/mcp/handler_search.go`: 取消注释 P4C 块

## 📋 待完成

### P4D (居里) — detect_changes 影响分析
### P4E (居里) — trace_path data_flow/cross_service
### P4A (张衡) — search_graph BM25 结构提升 + 分页

## 测试状态
```
internal/code/base/  34 tests ✅  (含 P4C 新增)
internal/code/lsp/   36 tests ✅
internal/code/mcp/   39 tests ✅  (含 search_code 注册)
总计: 109+ ✅
```

## 未提交改动
```
?? internal/code/base/architecture.go           (居里)
?? internal/code/base/architecture_test.go       (居里)
?? internal/code/mcp/handler_search_code.go      (张衡)
 M internal/code/base/crud.go                   (张衡: GetNodesByFile)
 M internal/code/mcp/handler_search.go           (居里+张衡)
 M internal/code/mcp/server.go                  (张衡: 注册 search_code)
```
