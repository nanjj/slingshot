---
name: site-page
description: slingshot CLI 站点管理参考 — site/page 命令、两种类型（page/zine）、部署全流程
keywords: site, page, static site, deployment, rsync, slingshot, 站点, 部署
author: JUN JIE NAN <nanjunjie@gmail.com>
---

# site-page — slingshot CLI 站点管理

slingshot CLI 的站点/页面管理命令参考。涵盖两种站点类型：

- **page**（默认）— 手动管理页面，slingshot 负责索引生成
- **zine** — [Zine](https://zine-ssg.io/) 生成站点，slingshot 负责构建+部署编排

> Zine 站点深度知识参阅 [site-zine](./site-zine/SKILL.md)。

## 核心概念

每个 **site** 是一个部署目标：

| 字段 | 必需 | 说明 |
|------|------|------|
| `dir` | ✅ | 本地站点目录 |
| `type` | ❌ | 站点类型：`page`（默认）或 `zine` |
| `rsync` | ❌ | 远程部署命令 |
| `title` | ❌ | 站点标题 |

**page 类型**：每个 **page** 是站点目录下一个子目录，包含 `index.html` 及其资产。

**zine 类型**：内容以 Zine 源文件管理（`.smd`、`.shtml`、`.ziggy`），构建输出到 `public/`。

## 站点管理

```bash
# 列出所有站点（显示 dir、type、rsync）
slingshot site list

# 添加 page 类型站点（dir 必需）
slingshot site add mysite dir ~/mysite

# 添加 zine 类型站点
slingshot site add mysite dir ~/mysite type zine \
  rsync 'rsync -avz --delete ./ user@host:/path'

# 更新站点配置
slingshot site update mysite rsync 'rsync -avz --delete ./ user@host:/new-path'
slingshot site update mysite type zine

# 删除站点（不会删除本地目录）
slingshot site remove mysite
```

> `site add` 自动创建站点目录并写入响应式 `style.css`。
> `dir` 路径中的 `$HOME` 自动替换为 `~`，方便跨机器共享配置。

## 类型对比

| 维度 | page（默认） | zine |
|------|-------------|------|
| 内容管理 | `slingshot page add/update/remove` | 直接编辑 `.smd` 文件 |
| 构建 | 无需构建，页面即文件 | `zine release --force` 生成 `public/` |
| 索引 | slingshot 自动生成 `index.html` | zine 管理路由 |
| rsync 源目录 | 站点根目录 `./` 直接 rsync | 自动从 `public/` rsync |
| CSS 优化 | `site rsync` 自动优化 | 无需优化（zine 管理） |
| 适用场景 | 简单站点、Org mode 写作 | 多语言、模板、文档站点 |

## page 类型工作流

### 准备页面文件

支持两种源格式：

```bash
# HTML 文件 — 直接使用
slingshot page add mysite my-article.html

# Org mode 文件 — 自动转换为 HTML（需 Emacs 26+）
slingshot page add mysite my-article.org
```

> 页面名称从文件名推导，不含扩展名。只允许 `[a-zA-Z0-9_-.]`。

### 页面管理

```bash
# 列出站点所有页面
slingshot page list mysite

# 添加/更新页面
slingshot page add mysite my-article.html
slingshot page update mysite my-article.html

# 删除页面（同时删除页面目录及其所有资产）
slingshot page remove mysite my-article
```

### 图片资产处理

添加/更新页面时自动处理 `<img src="...">` 引用：

- **绝对 URL**（`http://`、`https://`、`data:`、`//`）：保持原样
- **相对路径**（如 `images/photo.png`）：自动复制到页面目录
- **缺失图片**：报警告但不中断

### 索引自动生成

添加/更新/移除页面后自动重新生成站点根目录的 `index.html`，以页面 `<title>` 作为链接文本。

> ⚠️ **索引覆盖**：自动生成的 `index.html` 会覆盖自定义首页。如需自定义首页，请使用 zine 类型。

### 部署

```bash
slingshot site rsync mysite
```

page 类型 rsync 流程：
1. 自动优化站点 CSS（检查响应式 sentinel，未优化则升级）
2. 从站点目录 `./` 执行 rsync

## zine 类型工作流

Zine 站点不通过 `slingshot page` 管理页面。内容编辑全在 Zine 源文件中完成。

### 快速参考

```bash
# 本地开发预览（仅人类使用）
cd ~/mysite && zine serve

# 生产构建
cd ~/mysite && zine release --force

# slingshot 一键构建+部署
slingshot site rsync mysite
```

`slingshot site rsync` 对 zine 类型的流程：
1. 运行 `zine release --force`（构建到 `public/`）
2. 从 `public/` 目录执行 rsync

> 详细的 Zine 操作（目录结构、config.ziggy、.smd 格式、模板、i18n、版本管理、CSS 样式）→ 参阅 **[site-zine skill](./site-zine/SKILL.md)**。

### 与 site-zine skill 的分工

| 主题 | 在哪里 |
|------|--------|
| slingshot CLI 命令 | ✅ 本文档 |
| 目录结构 + .gitignore 陷阱 | site-zine |
| config.ziggy 配置（host_url、output_prefix_override） | site-zine |
| .smd 内容格式 + layout 选择 | site-zine |
| 模板系统（base.shtml、导航、子页面） | site-zine |
| i18n 国际化（zine.ziggy、locale 切换） | site-zine |
| 版本管理（assets/version.ziggy） | site-zine |
| 开发迭代工作流 | site-zine |
| 常见陷阱表 | site-zine |

## 通用操作

### CSS 优化

```bash
# 手动优化站点 CSS（page 类型适用）
slingshot site optimize mysite

# 强制重新写入（即使已优化）
slingshot site optimize mysite --force
```

自动写入响应式 CSS sentinel。page 类型 `site rsync` 时自动触发，通常无需手动执行。

### 部署远程

```bash
slingshot site rsync mysite
```

slingshot 根据站点类型自动选择流程：
- **page**：CSS 优化 → 从 `./` rsync
- **zine**：`zine release --force` 构建 → 从 `public/` rsync

## 端到端示例

### page 类型：从 Org 文件到线上

```bash
# 1. 配置站点
slingshot site add my-blog dir ~/blog \
  rsync 'rsync -avz --delete ./ user@server:/var/www/blog'

# 2. 添加页面
slingshot page add my-blog posts/hello-world.org  # Org→HTML→页面目录→索引更新
slingshot page add my-blog posts/about-me.org
slingshot page add my-blog posts/project-update.html

# 3. 查看页面
slingshot page list my-blog

# 4. 部署（自动 CSS 优化 + rsync）
slingshot site rsync my-blog

# 5. 删除页面
slingshot page remove my-blog old-page
```

### zine 类型：文档站点

```bash
# 1. 配置 zine 类型站点
slingshot site add docs dir ~/docs type zine \
  rsync 'rsync -avz --delete ./ user@server:/var/www/docs'

# 2. 编辑内容（直接操作 Zine 源文件）
#   content/en-US/index.smd  — 首页
#   layouts/templates/base.shtml — 导航模板

# 3. 部署（自动构建 + public/ 下 rsync）
slingshot site rsync docs
```

## 注意事项

- **站点目录**：`site add` 自动创建；`site remove` 不会删除本地目录
- **路径可移植**：`dir` 路径自动 `$HOME` → `~`，方便跨机器共享
- **页面名称**：只允许 `[a-zA-Z0-9_-.]`，防止路径穿越
- **图片复制**：仅在 add/update 时执行，不跟踪后续更改
- **Emacs 依赖**：`.org` → `.html` 需 GNU Emacs ≥ 26.1。未安装时先用 Emacs GUI 导出 `.html` 再执行 `page add`
- **索引覆盖**：自动 `index.html` 会覆盖自定义首页
- **zine 依赖**：系统需安装 [Zine](https://zine-ssg.io/)（静态二进制，下载解压即可）
- **zine 站点**：不适用 `page add/update/remove`，内容管理见 site-zine skill
