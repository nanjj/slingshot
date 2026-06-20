# `slingshot i18n` 设计文档 & 实施跟踪

## 设计概述

`slingshot i18n` 子命令管理 Go 项目的 .po 翻译文件工作流。
核心抽象：提取 → 管理 → 验证。设计为通用工具，适用于任何使用 .po + 哨兵 locale 的 Go 项目。

### 命令树

```
slingshot i18n
  check     [locale...]   检查 locale 一致性（缺失/未翻译/冗余条目）
    --exit-code           有 issue 时退出码非 0（CI 用）
    --dir                 自定义 locales 目录（默认: internal/i18n/locales）
  stats                   显示各 locale 统计信息
    --dir                 自定义 locales 目录（默认: internal/i18n/locales）
  extract   [path...]     从 Go 源码提取 i18n.G() 的 msgid（Phase 3）
  sync      [locale...]   从 en_US 同步到指定 locale（Phase 2）
  add       <locale> <msgid> [msgstr]  手动添加条目（Phase 2）
  init      <locale>      从 en_US 模板初始化新 locale（Phase 2）
```

### Phase 划分

| Phase | 文件 | 内容 | 状态 |
|-------|------|------|------|
| **Phase 1** | `i18n.go`, `i18n_po.go`, `i18n_check.go`, `i18n_stats.go` | 根命令 + check + stats | ✅ Done |

| **Phase 2** | `i18n_sync.go`, `i18n_add.go`, `i18n_init.go` | sync / add / init | ⬜ |
| **Phase 3** | `i18n_extract.go` | AST 提取器，替代 gen-en-us.sh | ⬜ |
| **Phase 4** | — | 通用化参数（--pattern, --sentinel, --name） | ⬜ |
| **Phase 5** | `internal/i18n/locales.go` | go generate 集成 | ⬜ |

---

## 变更记录

| 日期 | 变更 |
|------|------|
| 2026-06 | Phase 1: i18n root + check + stats |
