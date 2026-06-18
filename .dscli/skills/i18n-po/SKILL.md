---
name: i18n-po
description: Slingshot i18n .po 翻译管理 — en_US 哨兵、zh_CN 翻译、缺失检测、自动生成条目
author: Curie <curie@dscli.io>
keywords:
- i18n
- po
- translation
- zh_CN
- en_US
- sentinel
- G()
- locale
- slingshot
- missing
---

---
author: Curie <curie@dscli.io>
description: Slingshot i18n .po 翻译管理 — en_US 哨兵、zh_CN 翻译、缺失检测、自动生成条目
keywords: i18n, po, translation, zh_CN, en_US, sentinel, G(), locale, slingshot, missing
---

# i18n-po — Slingshot 翻译管理实战

Slingshot 使用嵌入式 `.po` 文件实现国际化（i18n），所有翻译在编译时通过 `//go:embed` 打包进二进制文件。

## 架构

```
internal/i18n/
├── i18n.go          # 核心：detectLang, parsePO, G() 函数
├── locales.go       # //go:embed locales/*/*.po
├── locales/
│   ├── en_US/
│   │   └── slingshot.po   ← 哨兵文件（所有 msgid 必须在此注册）
│   └── zh_CN/
│       └── slingshot.po   ← 中文翻译
```

### 关键约定

| 概念 | 说明 |
|------|------|
| **en_US 是哨兵** | en_US 的 `msgstr` 与 `msgid` 相同。它的唯一职责是：**在编译期声明所有 msgid**。没有 en_US 条目的 msgid 都是未注册的。 |
| **zh_CN 是完整翻译** | 必须包含所有 en_US 中的 msgid 的中文翻译。缺失会导致 `i18n.G()` panic。 |
| **panic 是特性** | `i18n.G()` 在找不到 msgid 时 panic——这是故意的。它强制开发者在加新翻译调用时同步更新 .po 文件，防止遗漏。 |
| **空字符串已翻译** | `msgstr ""` 表示已注册但暂未翻译，可以容忍，不会 panic。 |
| **大写格式字符串** | `\n` 在 .po 的 `"..."` 字符串中是换行转义。多行 msgid 使用 `msgid ""` + 多行 `"..."` 格式。 |

## 工作流

### 1. 在 Go 源码中添加新的翻译调用

```go
// 简单字符串
i18n.G("Hello, world!")
// 多行（Raw string literal）
i18n.G(`This is a long description
that spans multiple lines.
`)
// 带格式参数
fmt.Errorf(i18n.G("request failed: %w"), err)
```

### 2. 更新 en_US 哨兵

en_US 必须永远走在最前面——它是所有 msgid 的权威注册表。

```bash
bash .dscli/skills/i18n-po/scripts/gen-en-us.sh
```

验证：

```bash
LANG=en_US.UTF-8 go run ./cmd/slingshot/ --help
# en_US 模式下永远不应 panic
```

### 3. 更新 zh_CN 翻译

```bash
# 查找缺失条目
python3 .dscli/skills/i18n-po/scripts/find-missing.py

# 用内置翻译映射表生成
python3 .dscli/skills/i18n-po/scripts/gen-zh-cn.py --with-translations

# 或用 JSON 翻译文件
python3 .dscli/skills/i18n-po/scripts/gen-zh-cn.py --translations my-translations.json
```

验证：

```bash
LANG=zh_CN.UTF-8 go run ./cmd/slingshot/ --help
# 不应 panic；检查输出是否完整
```

### 4. 完整编译验证

```bash
go build ./cmd/slingshot/
```

## 多行 msgid 的 .po 格式

Go 的 raw string literal（反引号）在 .po 中需要拆成多行：

```po
msgid ""
"Query Jaeger tracing data via the Jaeger Query HTTP API.\n"
"\n"
"Subcommands:\n"
"  services              List all registered services\n"
"  deps                  Get service dependency graph"
msgstr ""
"通过 Jaeger Query HTTP API 查询追踪数据。\n"
"\n"
"子命令：\n"
"  services              列出所有已注册服务\n"
"  deps                  获取服务依赖关系图"
```

注意：
- 每个 `"..."` 行尾的 `\n` 表示换行
- 最后一行不加 `\n` 表示字符串到此结束，无尾随换行——必须与 Go 源码完全一致
- `msgid ""` 的 `""` 是空行，表示多行 msgid 的开始

## i18n.G() 核心逻辑

```go
func G(msgid string) string {
    if table, ok := translations[detectedLang]; ok {
        if translated, ok := table[msgid]; ok {
            if translated != "" {
                return translated
            }
            // 空字符串 = 已注册但未翻译，容忍
        } else {
            // 没找到 = 开发者忘记加条目 → PANIC
            panic(...)
        }
    }
    // 短语言代码回退（zh_CN → zh）
    ...
    return msgid
}
```

## 常见问题

### Q: 为什么 en_US 的 msgstr 必须和 msgid 一样？

因为 en_US 的角色是哨兵，不是翻译。Go 源码中的 msgid 本身就是英文，en_US 只需要说"这个 msgid 我已经知道了"。真正的翻译工作在 zh_CN 完成。

### Q: 为什么不用 msgid 直接回退？

因为 msgid 可能包含占位符（`%s`、`%d`），回退时格式不受影响。但缺少 en_US 条目意味着**根本没有注册这个翻译键**，这是开发流程的漏洞。

### Q: 为什么 panic 而不是 warning？

这是 @nanjunjie 的设计决策——panic 让问题在开发阶段就被发现，而不是等到生产环境才暴露。每次添加新的 `i18n.G()` 调用后，开发者的第一反应应该是更新 .po 文件。

### Q: 如何测试两端？

```bash
LANG=zh_CN.UTF-8 go run ./cmd/slingshot/ jaeger --help
LANG=en_US.UTF-8 go run ./cmd/slingshot/ jaeger --help
```
