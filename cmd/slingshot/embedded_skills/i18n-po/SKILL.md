---
name: i18n-po
description: Slingshot .po 翻译管理 — sync → check → translate → stats
keywords: i18n, po, translation, zh_CN, en_US, slingshot, sync, check, translate, stats
---

# i18n-po — Slingshot 翻译管理

管理 slingshot 的 `.po` 翻译文件。翻译在编译时通过 `//go:embed` 打包进二进制文件。

## 工作流

### 1. 在源码中添加翻译

给用户可见的字符串加上 `i18n.G()`：

```go
i18n.G("Hello, world!")                       // 简单字符串
i18n.G(`多行描述\n跨越多行\n`)                   // 多行 (raw string literal)
```

> 给 AI 看的（error/warning）不需要翻译。给人看的（Short、Long、输出消息）需要。

### 2. 同步

```bash
slingshot i18n sync           # 提取新 msgid → .po
slingshot i18n sync --delete  # 同步并删除孤立条目
```

从源码提取 `i18n.G()` 调用，新 msgid 写入 `en_US`（msgstr = msgid），传播到其他 locale（msgstr 为空待翻译）。

### 3. 检查

```bash
slingshot i18n stats                  # 翻译覆盖率统计
slingshot i18n check                  # 检查所有 locale 一致性
slingshot i18n check zh_CN            # 只检查指定 locale
slingshot i18n check --exit-code      # 发现问题时非零退出
slingshot i18n show zh_CN <id>        # 查看完整 msgid（不截断）
```

### 4. 翻译

```bash
slingshot i18n translate zh_CN \
  --msgid "Manage translation (.po) files" \
  --msgstr "管理翻译（.po）文件"
```

`.po` 转义规则：`\n` 是换行，`\"` 是双引号，`\\` 是反斜杠。

### 5. 验证

```bash
slingshot i18n stats          # 应 100%
slingshot i18n check --exit-code  # 应无报错
go build ./cmd/slingshot/     # 编译验证
```

## 示例：添加新翻译

```bash
# 1. 源码加 i18n.G()
# 2. slingshot i18n sync
# 3. slingshot i18n check zh_CN
# 4. slingshot i18n translate zh_CN --msgid "..." --msgstr "..."
# 5. slingshot i18n stats && go build ./cmd/slingshot/
```

## 多行 msgid 的 .po 格式

```po
msgid ""
"Line one.\n"
"Line two.\n"
"Line three"
msgstr ""
"第一行。\n"
"第二行。\n"
"第三行"
```

- 每个 `"..."` 行尾的 `\n` 表示换行（与 Go raw string literal 中的换行符对应）
- 最后一行无 `\n` 表示字符串到此结束，必须与 Go 源码完全一致
- `msgid ""` 的 `""` 是空行标记，表示多行 msgid 开始

## 多语言测试

```bash
LANG=zh_CN.UTF-8 slingshot --help
LANG=en_US.UTF-8 slingshot --help
```
