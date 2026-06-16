---
name: site-page
description: 静态站点部署全流程 — site 管理（add/update/remove/list/rsync）、page 管理（add/update/list/remove）、Org→HTML 转换、Zine 站点内容管理
keywords: site, page, static site, deployment, rsync, org, html, index, zine, i18n, smd, ziggy, 静态站点, 站点部署, 页面管理, zine站点
author: JUN JIE NAN <nanjunjie@gmail.com>
---

# site-page — 静态站点部署

静态站点部署工作流。支持两种站点类型：

- **page**（默认）— 直接管理 HTML 页面，适合简单站点
- **zine** — [Zine](https://zine.tools/) 生成站点，支持 .smd 内容管理、i18n、模板系统

## 核心概念

每个 **site** 是一个部署目标，包含：
- `dir`：本地站点目录（必需）
- `rsync`：远程部署命令（可选）
- `type`：站点类型 — `page` 或 `zine`（可选，默认 `page`）

对于 **page 类型**：每个 **page** 是站点目录下一个子目录，包含 `index.html` 及其资产（图片等）。

对于 **zine 类型**：内容以 Zine 源文件管理（`.smd`、`.shtml`、`.ziggy`），通过 `zine release --force` 构建输出到 `public/`。

## 前置条件

### 0. 配置站点

```bash
# 查看已有站点
slingshot site list

# 添加 page 类型站点（必需：dir；可选：rsync）
slingshot site add mysite dir ~/mysite
slingshot site add mysite dir ~/mysite rsync 'rsync -avz --delete ./ user@host:/path'

# 添加 zine 类型站点
slingshot site add mysite dir ~/mysite type zine \
  rsync 'rsync -avz --delete ./ user@host:/path'

# 查看站点的目录、类型和 rsync 命令
slingshot site list

# 更新站点配置
slingshot site update mysite rsync 'rsync -avz --delete ./ user@host:/new-path'
slingshot site update mysite type zine
```

> **AI 智能体注意**：执行任何 page 操作前，确认目标 site 存在且 `dir` 字段非空。
> 如果站点未配置，先执行 `slingshot site add <name> dir <path>`。
> 对于 zine 类型站点，`page add/update/remove` 命令不适用（见下文 zine 工作流）。

---

## page 类型工作流

### 1. 准备页面文件

**方式一：HTML 文件**
```bash
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

### 5. 部署远程

```bash
slingshot site rsync mysite
```

rsync 命令在站点目录下执行，输出实时显示。

### 6. 管理站点

```bash
# 删除站点（不会删除本地目录）
slingshot site remove mysite
```

---

## zine 类型工作流

Zine 是一个静态站点生成器，使用 `.smd`（Simplified Markdown）作为内容格式，`.shtml` 作为模板。slingshot 的 `slingshot site rsync` 命令会自动识别 zine 类型，先运行 `zine release --force` 构建，再从 `public/` 目录 rsync 部署。

### 目录结构

```
site-dir/
├── config.ziggy          # 站点配置（标题、语言、主题色）
├── i18n/                 # 国际化
│   ├── en-US.ziggy
│   └── zh-CN.ziggy
├── layouts/
│   └── templates/
│       └── base.shtml    # 基础模板（导航栏、页脚）
├── content/
│   ├── en-US/
│   │   ├── index.smd     # 首页
│   │   ├── getting-started.smd
│   │   ├── docs/
│   │   │   └── index.smd
│   │   └── blog/
│   │       └── index.smd
│   └── zh-CN/            # 中文翻译版本
│       ├── index.smd
│       └── ...
├── assets/
│   ├── style.css
│   └── version.ziggy
├── public/               # 构建输出（自动生成，不手动编辑）
└── scripts/
    └── update-version.sh
```

### 0. 本地预览

```bash
cd /path/to/site
zine serve      # 开发服务器，默认 :8080
zine release    # 生产构建，输出到 public/
```

### 1. 管理导航栏

导航栏在 `layouts/templates/base.shtml` 中定义：

```bash
# 查看当前导航
grep '<nav>' /path/to/site/layouts/templates/base.shtml

# 编辑导航栏
# 在 <nav> 内添加/删除 <a href="/xxx/">...</a>
```

导航项一般对应 `content/` 中的页面。添加新导航项后需要在 `i18n/` 中添加对应的翻译键。

### 2. 管理页面内容

Zine 使用 `.smd`（Simplified Markdown）格式：

```markdown
---
title: My Page
---

Content here in Simplified Markdown.
```

**添加新页面**：

```bash
# 1. 创建 .smd 内容文件
cat > content/en-US/my-page.smd << 'EOF'
---
title: My Page
---

This is my new page.
EOF

# 2. 添加中文翻译
cat > content/zh-CN/my-page.smd << 'EOF'
---
title: 我的页面
---

这是我的新页面。
EOF

# 3. 构建验证
slingshot site rsync mysite --explain  # 或用 zine release --force
```

### 3. 管理国际化（i18n）

i18n 字符串存储在 `i18n/<locale>.ziggy` 文件中：

```ziggy
nav:
  home: "Home"
  docs: "Docs"
  getting-started: "Getting Started"
site:
  title: "My Site"
```

**添加新的 i18n 键**：

```bash
# 1. 添加到英文文件
echo 'my-key: "English text"' >> i18n/en-US.ziggy

# 2. 添加到中文文件
echo 'my-key: "中文文本"' >> i18n/zh-CN.ziggy
```

### 4. 管理发布说明/版本信息

版本信息通常在 `assets/version.ziggy` 或 `i18n/` 中管理。更新版本号后需重新构建。

```bash
# 查看当前版本
grep version assets/version.ziggy

# 更新版本
sed -i 's/version: ".*"/version: "v0.3.0"/' assets/version.ziggy
```

### 5. 构建与部署

```bash
# 构建并部署（zine 类型会自动构建然后 rsync）
slingshot site rsync mysite

# 仅构建预览
cd /path/to/site && zine release --force

# 本地预览
cd /path/to/site && zine serve
```

`slingshot site rsync` 对于 zine 类型的流程：
1. 切换到站点目录
2. 运行 `zine release --force`（输出到 `public/`）
3. 从 `public/` 目录执行 rsync

### 6. 优化 CSS

```bash
# 优化站点 CSS 以适应响应式显示
slingshot site optimize mysite
```

---

## 端到端示例

### 场景 A：从 Org 文件到 page 类型站点

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

### 场景 B：纯 HTML 站点（无 Org）

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

### 场景 C：Zine 文档站点（如 slingshot.dscli.io）

```bash
# 1. 配置 zine 类型站点
slingshot site add docs dir ~/docs type zine \
  rsync 'rsync -avz --delete ./ user@server:/var/www/docs'

# 2. 编辑内容
#   content/en-US/getting-started.smd  — 快速开始
#   content/en-US/docs/index.smd       — 文档首页
#   layouts/templates/base.shtml       — 导航模板

# 3. 添加 i18n 条目
#   i18n/en-US.ziggy
#   i18n/zh-CN.ziggy

# 4. 本地预览
cd ~/docs && zine serve

# 5. 构建并部署（自动 zine release + rsync public/）
slingshot site rsync docs
```

---

## 注意事项

- **站点目录**：`site add` 时自动创建目录；`site remove` 不会删除本地目录
- **路径可移植**：`dir` 路径中的 `$HOME` 自动替换为 `~`，方便跨机器共享配置
- **页面名称安全**：只允许 `[a-zA-Z0-9_-.]`，防止路径穿越和特殊字符问题
- **图片复制**：仅在添加/更新页面时执行，不会跟踪后续的图片更改
- **Emacs 依赖**：`.org` → `.html` 转换需要 GNU Emacs（≥ 26.1）。如果未安装，先用 Emacs GUI 导出 `.html`，再执行 `page add <site> <file.html>`
- **索引覆盖**：`regenerateSiteIndex` 会覆盖站点根目录的 `index.html`，自定义首页会被覆盖
- **zine 构建**：`zine release --force` 需要系统安装 [Zine](https://zine-ssg.io/)（从 [GitHub Releases](https://github.com/kristoff-it/zine/releases) 下载静态二进制）
- **本地预览**：在 zine 站点目录运行 `zine serve` 启动开发服务器，默认端口 8080
