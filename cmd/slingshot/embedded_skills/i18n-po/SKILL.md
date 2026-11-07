---
name: i18n-po
description: Slingshot .po 翻译管理 — sync → check → translate → verify → pot（--id 按编号翻译）
author: Curie <curie@dscli.io>
keywords:
- i18n
- po
- pot
- translation
- zh_CN
- en_US
- slingshot
- sync
- check
- verify
- translate
- stats
- gettext
---

# i18n-po — Slingshot 翻译管理

管理 slingshot 的 `.po` 翻译文件。翻译在编译时通过 `//go:embed` 打包进二进制文件。

## ⚠️ 铁律（AI 与用户通用）

**修改 .po 的唯一方式是通过 `slingshot i18n` 命令**，不要直接编辑 `.po` 文件，不要写脚本来改它。命令会正确处理转义、保持文件结构稳定。

`.po` 文件是**扁平**格式：只有 `msgid` / `msgstr` 两行一对，不带 `#:` 位置注释、不带 `#,` 格式标记。位置和格式标记只出现在 `.pot` 模板里。

## 运行时语义：缺翻译不崩溃

`i18n.G()` 的查找顺序与 gettext 一致：当前 locale → 短语言码（`zh_CN` → `zh`）→ msgid 原文。

**msgid 缺失或 msgstr 为空时返回 msgid 本身，不会 panic。** 也就是说漏翻译的后果是「这句没翻译」，不是「程序崩溃」。

因此 `i18n verify` 是质量门禁而非安全网：它防止的是某个语言悄悄退化成英文，而不是防止运行时炸掉。

## 工作流

### 1. 在源码中添加翻译

给用户可见的字符串加上 `i18n.G()`：

```go
i18n.G("Hello, world!")   // 简单字符串

i18n.G(`第一行
第二行
第三行`)                   // 多行：反引号里是真正的换行
```

> 反引号是 Go 的 raw string literal，里面的 `\n` 是**字面的反斜杠加 n**，不是换行。要换行就直接按回车。要拼接长句用 `i18n.G("前半句" + "后半句")`，提取器支持 `+` 连接。

> 给 AI 看的（error/warning）不需要翻译。给人看的（Short、Long、输出消息）需要。

### 2. 同步

```bash
slingshot i18n sync           # 提取新 msgid → .po
slingshot i18n sync --delete  # 同步并删除孤立条目
```

从源码提取 `i18n.G()` 调用，新 msgid 写入 `en_US`（msgstr = msgid），传播到其他 locale（msgstr 为空待翻译）。已有译文永远不会被覆盖。

### 3. 检查

```bash
slingshot i18n stats                  # 翻译覆盖率统计
slingshot i18n check                  # 检查所有 locale 一致性
slingshot i18n check zh_CN            # 只检查指定 locale
slingshot i18n check --exit-code      # 发现问题时非零退出
slingshot i18n show zh_CN <id>        # 查看完整 msgid（不截断）
```

`check` 比较**各 locale 之间**是否一致。它发现不了「某个 msgid 在源码里用了，但所有 locale 都没有」这种情况——那是 `verify` 的职责。

### 4. 验证（构建门禁）

```bash
slingshot i18n verify                  # 每个 locale 对照源码树检查
slingshot i18n verify zh_CN            # 只检查指定 locale
slingshot i18n verify --strict-orphans # 孤立条目也视为失败
```

`verify` 把每个 locale 与**源码树**比较：源码里每个 `i18n.G()` 的 msgid 都必须在每个 locale 的 `.po` 里有条目。缺失即非零退出。

孤立条目（在 `.po` 里但源码已不再使用）默认只报告不失败，因为动态构造的 msgid 在 AST 扫描器看来可能像孤立条目。确认是真空洞时用 `sync --delete` 清理。

### 5. 翻译（推荐 `--id`）

```bash
slingshot i18n check zh_CN            # 1. 先看未翻译条目及其编号
slingshot i18n translate zh_CN \
  --id 3 --msgstr "管理翻译（.po）文件"   # 2. 按编号翻译，无需复制 msgid
```

`--id` 与 `i18n check` 显示的编号一致（也用于 `i18n show`）。适合长/多行 msgid，避免 shell 转义问题。

> ⚠️ **编号会随每次翻译重排。** 某条翻译成功后，未翻译列表少了一条，它后面所有编号都前移一位。所以：**一次只翻译一条，翻完重新 `check` 取号**。要批量翻译多条，改用 `--msgid` 按精确文本定位，不要连续用 `--id`。

如需指定精确 msgid（批量翻译、修改已有译文、或 check 之外的条目）：

```bash
slingshot i18n translate zh_CN \
  --msgid "Manage translation (.po) files" \
  --msgstr "管理翻译（.po）文件"
```

`.po` 转义规则：`\n` 是换行，`\"` 是双引号，`\\` 是反斜杠。`--msgid` 和 `--msgstr` 都要用 `.po` 转义形式。

### 6. 生成 .pot 模板

```bash
slingshot i18n pot                       # 输出到 <locales dir>/slingshot.pot
slingshot i18n pot -o path/to/out.pot    # 指定输出
```

从源码重新生成 `.pot` 模板。排版与 GNU xgettext **字节级一致**：每条带 `#: file:line` 位置、格式串带 `#, c-format`、复数调用带 `msgid_plural`。这是给翻译工具和 `.po` 合并用的模板，不是运行时数据。

限制扫描范围（可重复；默认扫整个模块）：

```bash
slingshot i18n pot \
  --source 'cmd/incus/*.go' \
  --source 'shared/cliconfig/*.go'
```

其他参数：`--package-name`、`--msgid-bugs-address`、`--creation-date`（可复现构建用）。

### 7. 收尾验证

```bash
slingshot i18n check              # 各 locale 一致
slingshot i18n verify             # 源码 ↔ locale 无缺失
slingshot i18n stats              # 应 100%
go build ./cmd/slingshot/         # 编译验证
```

## 示例：添加新翻译

```bash
# 1. 源码加 i18n.G()
# 2. slingshot i18n sync
# 3. slingshot i18n check zh_CN
# 4. slingshot i18n translate zh_CN --id <编号> --msgstr "..."
#    多条时改用 --msgid，避免编号重排
# 5. slingshot i18n check && slingshot i18n verify && go build ./cmd/slingshot/
```

## 多行 msgid 的 .po 格式

`.po` 里长字符串按行拆开，首段带内容，续行缩进对齐：

```po
msgid "Line one.\n"
        "Line two.\n"
        "Line three"
msgstr "第一行。\n"
        "第二行。\n"
        "第三行"
```

- 每段末尾的 `\n` 表示换行，与 Go 源码里该位置是否有换行严格对应
- 最后一段没有 `\n`，表示字符串到此结束
- `.pot` 里用的是同样的拆行方式，但关键字补齐到第 8 列（`msgid   "..."`、`msgstr  "..."`）

**不要手写这个格式。** 拆行、转义、续行对齐都由命令处理；手写迟早会和源码对不上。

## 多语言测试

```bash
LANG=zh_CN.UTF-8 slingshot --help
LANG=en_US.UTF-8 slingshot --help
```

## 常见坑

| 现象 | 原因 | 处理 |
|---|---|---|
| `verify` 报缺失 N 条 | 源码加了新 `i18n.G()` 没 sync | `i18n sync` 后翻译 |
| `verify` 报孤立 N 条 | 源码改了文案，`.po` 还留旧条目 | 确认后 `i18n sync --delete` |
| `translate --id N` 报 out of range | 前一条翻译成功后编号已重排 | 重新 `check` 取号，或改用 `--msgid` |
| 多行文案提取为空或错位 | 用了反引号却写了 `\n` | raw string 里直接换行 |
| `check` 全绿但某语言仍是英文 | 该语言缺条目，而 `check` 只比 locale 之间 | 跑 `i18n verify` |
