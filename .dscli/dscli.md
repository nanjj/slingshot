# Phase 3+4: 非 MCP tracing 铺开 — 完成状态

## ✅ Phase 3 — CLI 命令组

| # | 组 | 文件 | 状态 | Commit |
|---|-----|------|------|--------|
| 3a | draft | `draft_add.go`, `draft_update.go`, `draft_convert.go`, `draft_convert_upload.go`, `draft_sub.go` | ✅ 完成 | `893d4ec` |
| 3b | material | `meterial_add.go` | ✅ 完成 | `ab5f0bb` |
| 3c | site | `site.go` (rsync run), `site_optimize.go` | ✅ 完成 | `ab5f0bb` |
| 3d | page | `page_add.go` | ⏸️ 低优先级 | — |
| 3e | config/i18n | `config.go`, `i18n_*.go` | ⏸️ 低优先级 | — |

## ⏸️ Phase 4 — 内部包（延期）

内部包（draft, uploadimage, mdtowx, config 等）添加 tracing 需要 threading `context.Context`
through function signatures — 较侵入的 API 变更。当前 CLI handler 层的 span 已提供足够覆盖。
建议改为：当某内部包出现诊断需求时，按需添加。

## 模式总结

所有 CLI handler 使用统一模式：

```go
func (c *cmdXxx) run(cmd *cobra.Command, args []string) (err error) {
    span, _ := clog.StartSpanFromContext(cmd.Context(), "xxx")
    defer func() {
        if err != nil {
            span.LogKV("event", "error", "error", err.Error())
        }
        span.Finish()
    }()
    span.LogKV("event", "xxx", "key", val)
    // ...
    span.LogKV("event", "xxx_result", "key", val)
    return nil
}
```

## 总计

| Phase | Scope | Handlers | Span 模式 |
|-------|-------|----------|-----------|
| 1 | 基础设施 + demo | 6 (handler_search.go) | MCP handler (ctx, span, 三事件) |
| 2 | 全部 MCP handler | 23 | MCP handler (ctx, span, 三事件) |
| 3 | CLI 命令组 | 8 (draft_add/update/convert/upload_pipeline/draft_sub/material_add/site_rsync/site_optimize) | CLI run (named err, deferred finish, start+result events) |
