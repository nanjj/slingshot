---
name: site-zine
description: Zine 站点实战 — init/i18n/版面布局/rsync联动/版本管理/CSS响应式
author: JUN JIE NAN <nanjunjie@gmail.com>
keywords:
  - zine
  - static site
  - i18n
  - multilingual
  - smd
  - shtml
  - ziggy
  - rsync
  - slingshot
  - responsive
  - css
  - version
  - host_url
  - locale
---

# site-zine — Zine 站点实战

## 0. 📦 安装

```bash
# 示意：Linux x86_64，其他架构从 releases 选
curl -LO https://github.com/kristoff-it/zine/releases/download/v0.11.2/x86_64-linux-musl.tar.xz
tar xf x86_64-linux-musl.tar.xz
install zine ~/bin/
```

- 官方文档：<https://zine-ssg.io/>
- 源码/发布：<https://github.com/kristoff-it/zine/releases>

## 1. 🏗️ 项目创建

```bash
zine init mysite          # 创建项目
cd mysite
echo 'public/' >> .gitignore   # ← 必须，否则 git 提交构建产物
```

### 目录结构（以生产站点 slingshot.dscli.io 为例）

```
site-dir/
├── zine.ziggy             # 多语言配置、host_url、layouts/assets 路径
├── i18n/
│   ├── en-US.ziggy        # 英文翻译键值对
│   └── zh-CN.ziggy        # 中文翻译键值对
├── layouts/
│   ├── templates/
│   │   └── base.shtml     # 基础骨架（nav + 语言切换 + footer + 版本号）
│   ├── index.shtml        # 首页布局
│   ├── page.shtml         # 通用内容页布局
│   ├── docs-index.shtml   # 文档索引（自动遍历 subpages）
│   ├── blog.shtml
│   └── post.shtml
├── content/
│   ├── en-US/             # 英文内容
│   │   ├── index.smd      # 首页
│   │   ├── getting-started.smd
│   │   └── docs/
│   │       ├── index.smd
│   │       └── releases.smd
│   └── zh-CN/             # 中文内容（与 en-US 平行）
│       ├── index.smd
│       └── ...
├── assets/
│   ├── style.css          # 站点样式
│   ├── version.ziggy      # 版本号
│   ├── install.sh         # 安装脚本
│   └── slingshot.png      # logo
├── public/                # 构建输出（gitignore）
└── .gitignore
```

## 2. 🌐 多语言（i18n）配置

### zine.ziggy

```ziggy
Multilingual {
    .host_url = "https://slingshot.dscli.io",
    .i18n_dir_path = "i18n",
    .layouts_dir_path = "layouts",
    .assets_dir_path = "assets",
    .static_assets = ["install.sh", "slingshot.png"],
    .locales = [
        {
            .code = "en-US",
            .name = "English",
            .site_title = "Slingshot",
            .content_dir_path = "content/en-US",
            .output_prefix_override = "",          # 英文输出到根目录
        },
        {
            .code = "zh-CN",
            .name = "简体中文",
            .site_title = "Slingshot",
            .content_dir_path = "content/zh-CN",
            # output_prefix_override 省略 → 输出到 /zh-CN/
        },
    ],
}
```

关键注意：
- **`.host_url` 必须正确** — 否则语言切换链接可能出错
- **`.static_assets`** 列出所有需要随站点发布的文件（不在 `content/` 或 `assets/` 中的也会被复制）
- **英文 `.output_prefix_override = ""`** — 英文是默认语言，输出到根路径
- 中文省略 `output_prefix_override` → 输出到 `/zh-CN/` 子路径

### i18n/ 文件

`i18n/en-US.ziggy`:
```ziggy
{
    "nav.getting-started": "Getting Started",
    "nav.docs": "Docs",
    "nav.releases": "Release Notes",
    "switch-locale": "中文",
    "switch-locale-code": "zh-CN",
    "switch-locale-label": "Switch to Chinese",
    "footer.copyright": "Copyright © Slingshot Project",
    "version-label": "Version",
    "readmore": "Read more →"
}
```

`i18n/zh-CN.ziggy` — 对应翻译：
```ziggy
{
    "nav.getting-started": "快速开始",
    "nav.docs": "文档",
    "nav.releases": "发布说明",
    "switch-locale": "English",
    "switch-locale-code": "en-US",
    "switch-locale-label": "切换到英文",
    "footer.copyright": "版权所有 © Slingshot 项目",
    "version-label": "版本",
    "readmore": "阅读更多 →"
}
```

**教训**：修改导航或添加页面时，**必须在两个语言的 i18n 文件中都添加对应键**，否则会报错或显示空白。

### 模板中的 i18n 用法

```shtml
<a :text="$i18n.get('nav.getting-started')"></a>
```

## 3. 📄 内容格式（.smd）

每个页面是一个 `.smd`（Simplified Markdown）文件，顶部有 YAML 前置元数据：

```markdown
---
.title = "Getting Started",
.description = "Install Slingshot and equip your AI agent with skills",
.date = @date("2025-01-01T00:00:00"),
.author = "Slingshot Team",
.layout = "page.shtml",
.draft = false,
---

Content here in Simplified Markdown (subset of Markdown).
```

关键字段：
- `.layout` — 指定用哪个 shtml 布局文件（`page.shtml`、`index.shtml`、`docs-index.shtml` 等）
- `.title` / `.description` — 页面标题和描述，可在模板中引用
- `.draft = true` — 草稿不会出现在构建中

**文化差异注意**：中文 `.smd` 文件中 `.title` 等字段用英文内容没关系，但 `.description` 建议翻译。

**教训**：`.smd` 是 Zine 自定义格式，不是标准 Markdown。语法有差异（如用 `---` 作为 frontmatter 分隔符，但 frontmatter 用 Ziggy 语法而非 YAML）。

## 4. 🎨 布局系统（.shtml）

Zine 使用 `.shtml`（Simplified HTML）模板。`<super>` 标记子模板插入点。

### base.shtml — 站点骨架

所有页面继承 `base.shtml`：

```shtml
<!DOCTYPE html>
<html lang="$site.localeCode()">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="initial-scale=1">
    <title :text="$page.title.suffix(' · ', $site.title)"></title>
    <link type="text/css" rel="stylesheet" href="$site.asset('style.css').link()">
    <super>
</head>
<body>
    <header>
        <div class="header-inner">
            <a href="$site.link()" class="logo">Slingshot</a>
            <nav>
                <a href="$site.page('getting-started').link()"
                   :class="$site.page('getting-started').isCurrent().then('active')"
                   :text="$i18n.get('nav.getting-started')"></a>
                <!-- 更多导航项... -->
            </nav>
            <div class="locale-switcher">
                <ctx :loop="$page.locales()">
                    <ctx :if="$loop.it.link().eql($page.link()).not()">
                        <a href="$loop.it.link()" class="locale-link"
                           :text="$i18n.get('switch-locale')"
                           title="$i18n.get('switch-locale-label')"></a>
                    </ctx>
                </ctx>
            </div>
        </div>
    </header>
    <main><super></main>
    <footer>
        <p :text="$i18n.get('footer.copyright')"></p>
        <p class="version-info">
            <span :text="$i18n.get('version-label')"></span>
            <ctx :if="$site.asset('version.ziggy').ziggy().get?('version')">
                <code :text="$if"></code>
            </ctx>
        </p>
    </footer>
</body>
```

关键模式：
- **语言切换**：`$page.locales()` 遍历所有语言，`$loop.it.link().eql($page.link()).not()` 过滤掉当前语言
- **版本号**：从 `assets/version.ziggy` 读取并显示在 footer
- **导航高亮**：`$site.page('name').isCurrent().then('active')` 给当前页加 active 类

### 子页面索引（docs-index.shtml）

```shtml
<nav class="docs-list">
    <ctx :loop="$page.subpages()">
        <a href="$loop.it.link()" class="doc-card">
            <h3 :text="$loop.it.title"></h3>
            <p :text="$loop.it.description"></p>
        </a>
    </ctx>
</nav>
```

`.subpages()` 自动遍历当前页面目录下的所有 `.smd` 文件。页面结构：
```
content/en-US/docs/
├── index.smd      ← 用 docs-index.shtml，自动列出子页面
├── commands.smd
└── releases.smd   ← 普通 page.shtml
```

## 5. 🔄 开发迭代工作流

```
┌──────────────────────────────────────────────┐
│  1. 编辑 .smd / .shtml / i18n / style.css    │
│      （中英文内容同步修改）                     │
├──────────────────────────────────────────────┤
│  2. zine release --force                     │
│      → 构建输出到 public/                     │
├──────────────────────────────────────────────┤
│  3. 浏览器打开 public/index.html 检查渲染      │
│      （或部署后用浏览器工具验证）               │
├──────────────────────────────────────────────┤
│  4. 发现问题 → 回到步骤 1                      │
│     没问题 → git commit + git push            │
└──────────────────────────────────────────────┘
```

### AI 注意
- **`zine serve` 不适合 AI** — 热重载服务器是给人用的。AI 直接用 `zine release --force` 构建后检查 `public/`
- **修改内容后 AI 流程**：编辑 .smd → `zine release --force` → 浏览器查看 → 发现问题 → 再编辑

## 6. 🚀 部署（slingshot 集成）

```bash
# 添加 zine 类型站点（必需：dir、type、rsync）
slingshot site add mysite dir ~/mysite type zine \
  rsync 'rsync -avz --delete ./public/ user@host:/path'

# 一键构建 + 部署
slingshot site rsync mysite
# → 自动执行：zine release --force + rsync public/ → 远程
```

slingshot 的 `type: zine` 自动处理：
1. 切换到站点目录
2. 运行 `zine release --force`（输出到 `public/`）
3. 从 `public/` 目录执行 rsync

**教训**：rsync 路径必须是 `./public/`（不是 `./`），因为构建输出在 `public/` 里。

## 7. 📋 版本管理

### assets/version.ziggy

```ziggy
{
    "version": "v0.2.0",
}
```

更新版本：
```bash
sed -i 's/"version": ".*"/"version": "v0.3.0"/' assets/version.ziggy
zine release --force
slingshot site rsync mysite
```

### 在模板中显示版本

```shtml
<ctx :if="$site.asset('version.ziggy').ziggy().get?('version')">
    <code :text="$if"></code>
</ctx>
```

## 8. 📱 响应式 CSS 实战模式

从生产站点提取的核心模式（完整参考 `assets/style.css`）：

### 布局容器

```css
.header-inner, .footer-inner {
    max-width: 960px;
    margin: 0 auto;
    padding: 0 20px;
}

.content-page {
    max-width: 720px;
    margin: 0 auto;
    padding: 40px 20px 80px;
}
```

### 移动端适配

```css
@media (max-width: 640px) {
    .hero { padding: 40px 16px 32px; }
    .hero-logo { width: 100px; }
    .hero h1 { font-size: 2em; }
    nav { gap: 0; }
    nav a { font-size: 0.8em; padding: 4px 8px; }
    .footer-inner { flex-direction: column; gap: 8px; text-align: center; }
}
```

**教训**：
- 所有内容容器用 `max-width` + `margin: 0 auto` 而不是固定宽度
- `header-inner` 用 `flex` + `align-items: center` 保证导航和语言切换垂直居中
- 代码块在手机上要 `word-break: break-all` 防止溢出
- 表格单元格在手机上恢复 `white-space: normal` 防止滚动

## 9. ⚠️ 常见陷阱

| 陷阱 | 症状 | 解决 |
|------|------|------|
| `public/` 没加 `.gitignore` | git 提交了构建产物，合并冲突 | `echo 'public/' >> .gitignore` |
| `.host_url` 错误 | 语言切换链接不对、静态资源 404 | 检查 `zine.ziggy` 中 `.host_url` |
| 只加了一个语言的 i18n 键 | 另一个语言构建报错 | 两个 `i18n/*.ziggy` 都加 |
| `assets/` 中的文件没列在 `static_assets` | 构建后找不到该文件 | 在 `zine.ziggy` 的 `.static_assets` 中添加 |
| `.smd` 文件格式错误 | zine release 报 cryptic 错误 | 检查 frontmatter 语法（Ziggy 格式，不是 YAML） |
| 页面名拼写错误 | 模板中 `$site.page('xxx')` 返回空 | 检查文件名和路径是否正确 |
| Chinese locale 内容的 `.title` 不是中文 | 语言切换后标题未翻译 | 中英文 `.smd` 的 `.title` 应分别对应 |
| `zine serve` 在 CI 中运行 | 进程挂起，永不退出 | 用 `zine release --force` 替代 |

## 快速参考

```bash
# 新建站点
zine init mysite
echo 'public/' >> mysite/.gitignore

# 本地开发（人类用）
cd mysite && zine serve

# AI 构建检查
cd mysite && zine release --force
# 然后打开 public/index.html 查看

# 部署
slingshot site add mysite dir ~/mysite type zine \
  rsync 'rsync -avz --delete ./public/ user@host:/path'
slingshot site rsync mysite    # 构建 + 部署一步完成

# 更新版本
sed -i 's/"version": ".*"/"version": "v0.3.0"/' assets/version.ziggy

# 添加页面（中英文）
cat > content/en-US/my-page.smd << 'EOF'
---
.title = "My Page",
.layout = "page.shtml",
---
Content here.
EOF
cp content/en-US/my-page.smd content/zh-CN/my-page.smd
# 编辑中文内容...
# 记得加 i18n 键和导航项
zine release --force
```
