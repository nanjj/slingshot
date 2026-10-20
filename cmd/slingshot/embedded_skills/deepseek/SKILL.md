---
name: deepseek
description: DeepSeek 网页搜索与抓取 — 通过 `slingshot deepseek` 子命令调用 DeepSeek 自研搜索引擎：按 query 搜索网页返回结构化结果（标题/URL/摘要/正文），或抓取单个网页转为 Markdown。支持 site:/lang:/after: 等搜索语法。Key 走本地 config 或 DEEPSEEK_API_KEY 环境变量。
keywords:
- deepseek
- 搜索
- 网页搜索
- search
- fetch
- 抓取
- 网页
- web
- markdown
- rag
- 实时信息
- 联网
author: JUN JIE NAN <nanjunjie@gmail.com>
---

# deepseek — 网页搜索与抓取

slingshot 内置 `deepseek` 子命令，直接调用 DeepSeek 的 Search 与 Fetch
接口，为 Agent 与 RAG 应用提供实时网页内容。无需浏览器、无需 MCP 框架。

- **search**：按 query 搜索，返回结构化结果（title / url / snippet / summary / content）
- **fetch**：抓取单个网页，返回 Markdown 正文

## 前置条件（Key 配置）

```bash
slingshot config set deepseek.key <key>   # 写入本地 config（推荐）
export DEEPSEEK_API_KEY=<key>             # 或环境变量，优先级更高
```

- Key 只存在于本地 config/env，不写入代码、不硬编码
- 未配置时报错，并提示上述配置命令

## 子命令

| 命令 | 用途 |
|------|------|
| `slingshot deepseek search <word>...` | 搜索网页，输出标题、URL、摘要 |
| `slingshot deepseek fetch <url>` | 抓取网页，输出 Markdown 正文 |

## 用法示例

```bash
# 基本搜索
slingshot deepseek search "incus VM cloud-init"

# 限定条数
slingshot deepseek search --count 5 "DeepSeek V4 模型 发布"

# 带长摘要（600-800 字符，适合模型阅读）
slingshot deepseek search --summary "incus VM cloud-init"

# 带网页正文（整页，最耗输出）
slingshot deepseek search --content "incus VM cloud-init"

# 抓取单页
slingshot deepseek fetch https://linuxcontainers.org/incus/
```

## 输出层级：snippet / summary / content

DeepSeek 返回三个长度差异巨大的文本字段，按需选择：

| Flag | 字段 | 典型长度 | 用途 |
|------|------|---------|------|
| 默认 | `snippet` | 100-200 字符 | 前端展示，适合快速浏览 |
| `--summary` | `summary` | 600-800 字符 | 给模型阅读，含上下文 |
| `--content` | `content` | 整页正文 | 需要完整内容时用 |

- 默认只打 `snippet`，避免终端被长文本撑爆
- `--content` 隐含 `need_content=true`，API 侧才会返回正文
- 三个 flag 可叠加，输出按 snippet → summary → content 顺序

## 搜索语法

`query` 支持的操作符会原样传给搜索引擎，可组合使用：

| 操作符 | 示例 | 说明 |
|--------|------|------|
| `site:` | `DeepSeek site:github.com` | 限制站点，多个用空格加 `OR` |
| `-site:` | `DeepSeek -site:example.com` | 排除站点 |
| `-` | `DeepSeek -广告` | 排除关键词 |
| `-"..."` | `DeepSeek -"release notes"` | 排除短语 |
| `"..."` | `DeepSeek "精确短语"` | 精确匹配 |
| `lang:` | `DeepSeek lang:zh,en` | 限制语言 |
| `after:` | `DeepSeek after:2025` 或 `after:2025-01-01` | 发布日期下限 |
| `before:` | `DeepSeek before:2026` 或 `before:2026-06-01` | 发布日期上限 |

组合示例：

```bash
slingshot deepseek search 'DeepSeek 模型 site:github.com OR site:huggingface.co lang:zh after:2025'
```

## 响应字段

`search` 每条结果包含：

| 字段 | 类型 | 说明 |
|------|------|------|
| `rerank_score` | number | 0-3 分，≥1.5 为优质 |
| `title` | string | 标题 |
| `url` | string | 链接 |
| `snippet` | string | 短摘要 |
| `summary` | string | 长摘要 |
| `content` | string | 正文（需 need_content） |
| `site_name` | string/null | 站点名 |
| `date` | string/null | UTC RFC 3339 |
| `authority` | integer | 0-4，≥3 为极权威 |

`fetch` 返回 `{title, content}`，`content` 为 Markdown。

## API 映射

| slingshot 子命令 | DeepSeek API |
|------|------|
| search | `POST https://api.deepseek.com/v1/tools/search` |
| fetch | `POST https://api.deepseek.com/v1/tools/fetch` |

鉴权：`Authorization: Bearer <key>` + `Content-Type: application/json`。

## 计费与限速

- **计费**：按请求次数，Search/Fetch 各 5 元 / 1000 次成功请求
- **不计费**：请求失败、搜索无结果、超时
- **限速**：Search 100 QPS，Fetch 25 QPS（按账号，与 API Key 数量无关）
- **超限**：返回 HTTP 429，等后重试

## 错误码

| HTTP | 说明 | 处理 |
|------|------|------|
| 400 | 请求不合法 | 检查参数，不要重试 |
| 401 | Key 缺失或无效 | 配置 key |
| 402 | 余额不足 | 充值 |
| 403 | Fetch 被安全策略拒绝 | 换 URL |
| 404 | Fetch 无法提取正文 | 换 URL |
| 408/429/5xx | 超时/限速/服务端错误 | 等后重试 |

## 已知行为（2026-10-20 实测）

- `count` 默认 10，范围 1-50
- `need_content=false` 时 `content` 恒为空字符串
- 未知字段被静默忽略
- Fetch 结果受目标站点访问策略影响，可能 403 或 404
- 搜索结果可为空数组（正常结束但无匹配），此时命令行报 "no results"
