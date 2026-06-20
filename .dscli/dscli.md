# `slingshot i18n` 设计文档 & 实施跟踪

## 设计概述

`slingshot i18n` 子命令管理 Go 项目的 .po 翻译文件工作流。
核心抽象：提取 → 管理 → 验证。设计为通用工具，适用于任何使用 .po + 哨兵 locale 的 Go 项目。

运行时库已提取为独立模块：`github.com/nanjj/i18n`。
新项目通过 `slingshot i18n init` 生成脚手架，自动引入该库。

### 命令树

```
slingshot i18n
  init                      Scaffold i18n package for a new project
    --name                  自定义 .po 文件名（默认从 go.mod 推导）
    --dir                   自定义 locales 目录（默认: internal/i18n/locales）
  check     [locale...]     检查 locale 一致性（缺失/未翻译/冗余条目）
    --exit-code             有 issue 时退出码非 0（CI 用）
    --dir
  stats                     显示各 locale 统计信息
    --dir
  sync      [--delete]      从源码同步 msgid 到所有 .po 文件
    --delete                删除孤儿 msgid
    --dir
  add       <locale>        从 en_US 模板初始化新 locale
    --dir
  show      <locale> [id]   显示条目（全部/编号/搜索），格式化输出
                            id 格式：数字(1-36)、搜索词、show
  translate <locale>        逐条翻译（一次一条）
    --msgid                 PO-转义形式的 msgid
    --msgstr                翻译文本（PO-转义；空字符串清除翻译）
```

### 设计要点
- **sync 不覆盖已翻译 msgstr**：新条目在非 en_US locale 中以空 msgstr 添加
- **en_US msgstr = msgid**：英文条目的 msgstr 与 msgid 相同
- **--delete 安全删除**：只删除源码中已不存在的条目，保留 header
- **AST 提取器**：用 `go/parser` + `go/ast` + `strconv.Unquote` 替代正则，正确处理 Go 转义和字符串拼接。提取结果已原生转义，无需额外归一化
- **自动去重**：`dedupEntries` 在同步前去除同一 msgid 的重复条目（保留最后一条），消除因 `\\n` vs `\n` 等不同 .po 转义写法造成的重复
- **translate 逐条翻译**：使用 parsePOFull/writePO 基础设施，精确匹配完整 msgid，显示新旧对比，支持 PO 转义 round-trip
- **运行时库独立**：`github.com/nanjj/i18n` 是 Go 模块，`internal/i18n` 退化为薄包装
- **go generate 集成**：`internal/i18n/locales.go` 包含 `//go:generate slingshot i18n sync`，开发者修改代码后运行 `go generate ./internal/i18n/` 即可自动同步 .po 文件
- **项目根目录自动检测**：通过向上寻找 `go.mod`，从任意子目录均可正常运行 i18n 命令

## 实施状态

| Phase | 文件 | 内容 | 状态 |
|-------|------|------|------|
| **Phase 1** | `i18n.go`, `i18n_po.go`, `i18n_check.go`, `i18n_stats.go` | 根命令 + check + stats | ✅ Done |
| **Phase 2** | `i18n_po.go`(扩展), `i18n_extract.go`, `i18n_sync.go`, `i18n_add.go` | sync(--delete) / add | ✅ Done |
| **Phase 3** | `i18n_extract.go` (AST版), `i18n_extract_test.go` | AST 提取器，替代 regex | ✅ Done |
| **Phase 4** | — | ~~通用化参数（--pattern, --sentinel, --name）~~ | ❌ 取消 |
| **Phase 5** | `internal/i18n/locales.go`, `i18n_root.go`, `i18n.go`, `i18n_sync.go` | go generate 集成 + 项目根检测 | ✅ Done |
| **Phase 6** | `i18n_translate.go`, `i18n.go` | 逐条翻译子命令（translate） | ✅ Done |
| **Phase 7** | `github.com/nanjj/i18n` | 提取运行时为独立库 + `init` 命令 | ✅ Done |
