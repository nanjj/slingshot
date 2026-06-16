---
name: site-zine
description: Zine 静态站点 — 安装/git管理/rsync联动/CLI注意
author: JUN JIE NAN <nanjunjie@gmail.com>
keywords:
  - zine
  - static site
  - git
  - public
  - gitignore
  - rsync
  - slingshot
  - browser test
  - AI agent
  - dscli
  - i18n
  - smd
  - ziggy
  - multilingual
  - kristoff-it
---

# site-zine — Zine 站点实战经验

## 0. 📦 安装

下载二进制，放到 `~/bin/` 即可

```bash
# 示意：Linux x86_64，其他架构从 releases 选
curl -LO https://github.com/kristoff-it/zine/releases/download/v0.11.2/x86_64-linux-musl.tar.xz
tar xf x86_64-linux-musl.tar.xz
install zine ~/bin/
```

- 官方文档：<https://zine-ssg.io/>
---

## 1. 🗂️ public/ 必须在 .gitignore 中

```gitignore
public/
```

**为什么？**
- `public/` 是 `zine release` 的输出目录，提交它会导致合并冲突
- 源文件（`content/`、`layouts/`、`i18n/`、`zine.ziggy`）才是源码
- `slingshot site rsync` 从 `public/` 同步到远程，git 不需要关心它

> **AI 注意**：接手 zine 站点，第一件事检查 `.gitignore` 是否包含 `public/`。

---

## 2. 🔁 slingshot rsync 自动执行 zine release

`slingshot site rsync <name>` 对 `type: zine` 自动执行：

```
1. zine release --force    → 构建到 public/
2. rsync -avz --delete public/ user@host:/path
```

```bash
# ❌ 不要手动组合：
cd ~/site && zine release --force && slingshot site rsync mysite

# ✅ 只需：
slingshot site rsync mysite
```

### 什么时候需要单独运行 zine release？

| 场景 | 命令 |
|------|------|
| **本地预览** | `zine release --force`，打开 `public/index.html` |
| **开发迭代**（热重载） | `zine serve` → `http://localhost:8080` |
| **验证构建** | `zine release --force`，检查 `public/` |
| **部署** | `slingshot site rsync <name>`（自动构建） |

> **AI 注意**：改内容后先用 `zine release` 本地检查，部署时用 `slingshot site rsync`。

---

## 3. 🌐 浏览器测试清单

```bash
cd /path/to/site && zine serve
```

- [ ] 首页加载正常
- [ ] 导航栏链接可用
- [ ] 中/英文切换正确
- [ ] 每个页面内容渲染完整
- [ ] 样式（CSS）加载正常
- [ ] 图片等静态资源可访问

---

## 4. ⚠️ CLI 注意事项

- **不支持 `--explain`/`--dry-run`** → AI 无法安全地探索命令。对策：用 `slingshot site rsync` 部署（自动构建），本地用 `zine release --force` 后检查 `public/`
- **`zine serve` 不适合 AI** → 热重载服务器是给人用的。AI 直接用 `zine release --force` 构建，打开 `public/index.html` 检查
- **错误信息不明确** → 报错时先检查 `zine.ziggy`（Multilingual、pages 配置），然后检查 `.smd` 文件格式
- **i18n 手动维护** → 修改 `i18n/` 下的翻译键值对后重新构建
- **无内置页面管理** → 用 slingshot `page add/update/remove` 替代

### zine.ziggy 参考

```ziggy
Multilingual {
    .host_url = "https://example.com",
    .locales = [
        { .code = "en-US", .name = "English", .site_title = "My Site", ... },
        { .code = "zh-CN", .name = "简体中文", ... },
    ],
}
```

- `.smd` 文件是结构化内容单元
- `i18n/` 存储所有翻译键
- `layouts/` 定义页面骨架
- `static_assets` 列出随站点发布的文件

---
## 快速参考

```bash
# 安装（Linux x86_64 示意）
curl -LO https://github.com/kristoff-it/zine/releases/download/v0.11.2/x86_64-linux-musl.tar.xz
tar xf x86_64-linux-musl.tar.xz && install zine ~/bin/

# 本地开发
zine serve                    # 热重载开发服务器 :8080

# 生产构建
zine release --force          # 构建到 public/

# 通过 slingshot 部署
slingshot site add mysite dir ~/mysite type zine \
  rsync 'rsync -avz --delete ./ user@host:/path'
slingshot site rsync mysite   # 自动构建 + 从 public/ rsync

# 检查站点配置
slingshot site list
```
