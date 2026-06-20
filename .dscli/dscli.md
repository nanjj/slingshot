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
  sync      [--delete]    从源码同步 msgid 到所有 .po 文件
    --delete              删除孤儿 msgid（源码中已不存在的条目）
    --dir                 自定义 locales 目录
  add       <locale>      从 en_US 模板初始化新 locale
    --dir                 自定义 locales 目录
```

## 设计要点
- **sync 不覆盖已翻译 msgstr**：新条目在非 en_US locale 中以空 msgstr 添加
- **en_US msgstr = msgid**：英文条目的 msgstr 与 msgid 相同
- **--delete 安全删除**：只删除源码中已不存在的条目，保留 header
- **Go 转义归一化**：从源码提取的 `\"`、`\n` 等 Go 转义序列，经由 `normalizeMsgid` (即 unescapePO) 转成实际字符后再与 .po 条目比较，避免因转义方式不同而产生重复条目
- **自动去重**：`dedupEntries` 在同步前去除同一 msgid 的重复条目（保留最后一条），消除因 `\\n` vs `\n` 等不同 .po 转义写法造成的重复

| Phase | 文件 | 内容 | 状态 |
|-------|------|------|------|
| **Phase 1** | `i18n.go`, `i18n_po.go`, `i18n_check.go`, `i18n_stats.go` | 根命令 + check + stats | ✅ Done |
| **Phase 2** | `i18n_po.go`(扩展), `i18n_extract.go`, `i18n_sync.go`, `i18n_add.go` | sync(--delete) / add | ✅ Done |
| **Phase 3** | `i18n_extract.go` (AST版) | AST 提取器，替代 gen-en-us.sh | ⬜ |
| **Phase 4** | — | 通用化参数（--pattern, --sentinel, --name） | ⬜ |
| **Phase 5** | `internal/i18n/locales.go` | go generate 集成 | ⬜ |
