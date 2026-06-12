---
name: site-page
description: 静态站点部署全流程 — site 管理（add/update/remove/list/rsync）、page 管理（add/update/list/remove）、Org→HTML 转换
keywords: site, page, static site, deployment, rsync, org, html, index, 静态站点, 站点部署, 页面管理
author: JUN JIE NAN <nanjunjie@gmail.com>
---

# site-page — 静态站点部署

静态站点部署工作流。从 HTML / Org mode 到站点目录，再到远程服务器。

## 核心概念

每个 **site** 是一个部署目标，包含：
- `dir`：本地站点目录（必需）
- `rsync`：远程部署命令（可选）

每个 **page** 是站点目录下一个子目录，包含 `index.html` 及其资产（图片等）。页面名称需符合 `[a-zA-Z0-9_-.]`。

## 前置条件

### 0. 配置站点

```bash
# 查看已有站点
slingshot site list

# 添加站点（必需：dir；可选：rsync）
slingshot site add mysite dir ~/mysite
slingshot site add mysite dir ~/mysite rsync 'rsync -avz --delete ./ user@host:/path'

# 查看站点的目录和 rsync 命令
slingshot site list

# 更新站点配置
slingshot site update mysite rsync 'rsync -avz --delete ./ user@host:/new-path'
```

> **AI 智能体注意**：执行任何 page 操作前，确认目标 site 存在且 `dir` 字段非空。
> 如果站点未配置，先执行 `slingshot site add <name> dir <path>`。

## 工作流

### 1. 准备页面文件

支持两种源格式：

**方式一：HTML 文件**
```bash
# 直接使用 HTML
slingshot page add mysite my-article.html
```

**方式二（推荐）：Org mode 文件**
```bash
# 自动调用 Emacs 转换为 HTML（需安装 Emacs 26+）
slingshot page add mysite my-article.org
```

Org→HTML 转换依赖 GNU Emacs，系统需安装 `emacs`。若不可用，先手动导出为 HTML 再使用。

> 页面名称从文件名推导（不含扩展名）。例如 `my-article.org` → 页面名 `my-article`。
> 页面名称只允许字母、数字、连字符、下划线和点。

### 2. 页面管理

```bash
# 列出站点所有页面
slingshot page list mysite

# 添加页面（从 HTML 或 Org 文件）
slingshot page add mysite my-article.html
slingshot page add mysite my-article.org   # 自动 Org→HTML 转换

# 更新已有页面
slingshot page update mysite my-article.html

# 删除页面（同时删除页面目录及其所有资产）
slingshot page remove mysite my-article
```

### 3. 图片资产处理

添加/更新页面时自动处理 `<img src="...">` 引用：

- **绝对 URL**（`http://`、`https://`、`data:`、`//`）：保持原样，不复制
- **相对路径**（如 `images/photo.png`）：自动从源位置复制到页面目录
- **缺失图片**：报警告但不中断流程

图片按原相对路径结构复制到页面目录下。

### 4. 索引自动生成

添加/更新/移除页面后自动重新生成站点根目录的 `index.html`，包含所有页面的链接列表。页面 `<title>` 作为链接文本。

```bash
# 手动重建索引（如果有需要）
# 目前无独立命令，通过 page add/update/remove 自动触发
```

### 5. 部署远程

```bash
# 执行站点的 rsync 命令部署到远程服务器
slingshot site rsync mysite
```

rsync 命令在站点目录下执行，输出实时显示。

### 6. 管理站点

```bash
# 删除站点（不会删除本地目录）
slingshot site remove mysite
```

## 端到端示例

### 场景：从 Org 文件到线上站点

```bash
# 1. 配置站点（首次）
slingshot site add my-blog dir ~/blog \
  rsync 'rsync -avz --delete ./ user@server:/var/www/blog'

# 2. 添加页面（Org→HTML→页面目录→复制图片→更新索引）
slingshot page add my-blog posts/hello-world.org
# → Org 转换为 HTML
# → 页面目录 ~/blog/hello-world/ 创建
# → 图片从 posts/ 复制到 ~/blog/hello-world/
# → ~/blog/index.html 自动更新

# 3. 添加更多页面
slingshot page add my-blog posts/about-me.org
slingshot page add my-blog posts/project-update.html

# 4. 查看页面列表
slingshot page list my-blog

# 5. 更新页面内容
slingshot page update my-blog posts/hello-world.html
# 或直接用 Org 更新
slingshot page update my-blog posts/hello-world.org

# 6. 部署到远程服务器
slingshot site rsync my-blog

# 7. 删除不需要的页面
slingshot page remove my-blog old-page
```

### 场景：纯 HTML 站点（无 Org）

```bash
# 1. 配置站点
slingshot site add landing dir ~/landing

# 2. 添加页面
slingshot page add landing index.html
slingshot page add landing features.html
slingshot page add landing contact.html

# 3. 添加 rsync 配置并部署
slingshot site update landing rsync 'rsync -avz --delete ./ user@server:/www'
slingshot site rsync landing
```

## 注意事项

- **站点目录**：`site add` 时自动创建目录；`site remove` 不会删除本地目录
- **路径可移植**：`dir` 路径中的 `$HOME` 自动替换为 `~`，方便跨机器共享配置
- **页面名称安全**：只允许 `[a-zA-Z0-9_-.]`，防止路径穿越和特殊字符问题
- **图片复制**：仅在添加/更新页面时执行，不会跟踪后续的图片更改
- **Emacs 依赖**：`.org` → `.html` 转换需要 GNU Emacs（≥ 26.1）。如果未安装，先用 Emacs GUI 导出 `.html`，再执行 `page add <site> <file.html>`
- **索引覆盖**：`regenerateSiteIndex` 会覆盖站点根目录的 `index.html`，自定义首页会被覆盖
