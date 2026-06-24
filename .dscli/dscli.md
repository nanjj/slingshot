# Phase 3+4: 非 MCP tracing 铺开

## Phase 3 — CLI 命令组（已通过 PersistentPreRunE 有根 span，补子 span + LogKV）

| # | 组 | 文件 | 优先级 | 状态 |
|---|-----|------|--------|------|
| 3a | draft | `draft_add.go`, `draft_update.go`, `draft_convert.go`, `draft_convert_upload.go`, `draft_sub.go` | 🔴 最高 |
| 3b | material | `meterial_add.go` | 🔴 高 |
| 3c | site | `site.go` (rsync/doRsync/doBuild), `site_optimize.go` | 🟡 中 |
| 3d | page | `page_add.go` | 🟢 低 |
| 3e | config/i18n | `config.go`, `i18n_add.go`, `i18n_sync.go` | 🟢 最低 |

## Phase 4 — 内部包（需 threading context，较侵入）

| # | 包 | 入口函数 | 状态 |
|---|-----|---------|------|
| 4a | `internal/draft/` | `Add`, `List`, `Get`, `Update`, `Delete` | ⏳ |
| 4b | `internal/uploadimage/` | `Upload`, `UploadThumb` | ⏳ |
| 4c | `internal/material/` | `Add`, `List`, `Remove`, `Get` | ⏳ |
| 4d | `internal/mdtowx/` | `ConvertFile`, `ConvertOrgFile` | ⏳ |
| 4e | `internal/config/` | `Load`, `Save` | ⏳ |
| 4f | `internal/highlight/` | `HighlightDir` | ⏳ |
| 4g | `internal/getaccesstoken/` | `GetToken` | ⏳ |

## 模式

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
}
```

## Commit 边界

- Phase 3a: draft 组 → 一个 commit
- Phase 3b: material 组 → 一个 commit
- Phase 3c: site 组 → 一个 commit
- Phase 3d-e: page/config/i18n → 一个 commit
- Phase 4: 每次一个包

完成后删除此文件并通知张衡。
