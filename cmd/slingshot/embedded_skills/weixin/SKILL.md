---
name: weixin
description: 微信公众号文章草稿全流程 — Markdown / Org mode 转 HTML、上传图片、管理素材、创建/更新草稿
keywords: weixin, wechat, draft, meterial, article, 公众号, 草稿, 素材
author: JUN JIE NAN <nanjunjie@gmail.com>
---

# weixin

微信公众号文章发布工作流。从 Markdown / Org mode 到公众号草稿的完整链路。

## 前置条件

### 0. 配置微信凭据

所有操作依赖微信公众平台的 API 凭据。首次使用前必须配置：

```bash
# 在公众号后台 → 开发 → 基本配置 中获取 AppID 和 AppSecret
slingshot config set wechat.appid wx1234567890abcdef
slingshot config set wechat.secret abcdefghijklmnopqrstuvwxyz123456
```

查看配置状态：

```bash
slingshot config show          # 查看完整配置
slingshot config list          # 列出所有配置键
```

> **AI 智能体注意**：执行任何 draft/meterial 操作前，确认 `wechat.appid` 和
> `wechat.secret` 已配置。若缺失，需先引导用户提供凭据。

## 工作流

### 1. 准备文章（Markdown / Org mode）

编写文章内容，支持 GFM 语法（表格、删除线、代码块等）。

支持两种方式设置元数据（标题、作者、摘要、封面图）：

**方式一：YAML front matter**（嵌入在 Markdown 文件头部）

```yaml
---
title: 文章标题
author: 作者名
digest: 摘要内容（选填）
thumb_media_id: cover.png
content_source_url: https://example.com/article  # "阅读原文"跳转链接（选填）
---
```

**方式二（推荐）：sidecar YAML 文件**（与 Markdown 同目录，命名为 `<filename>.yaml`）

```yaml
# article.md 的同目录 sidecar 文件: article.yaml
title: 文章标题
author: 作者名
digest: 摘要内容（选填）
thumb_media_id: cover.png
content_source_url: https://example.com/article  # "阅读原文"跳转链接（选填）
```

Sidecar YAML 优先级高于 front matter，适合不修改原始 Markdown 文件添加元数据。
尤其适合从 org-mode 等外部工具导出的场景——Markdown 本身保持干净，元数据放在旁边。
> **`content_source_url`**：设置"阅读原文"跳转链接。sidecar YAML 或 HTML `<meta name="content_source_url">` 指定。
> 典型场景：`slingshot page add --rsync` 部署站点后，把站点 URL 填进来，形成公众号→个人站导流。

**直接支持 Org mode**：`.org` 文件可直接转换，无需事先导出为 Markdown：
  slingshot draft convert article.org --upload
系统调用 Emacs 完成 Org→Markdown 转换（要求系统已安装 Emacs）。

> `thumb_media_id` 可以是本地图片路径（如 `cover.png`），
> 配合 `--upload` 会自动上传到微信素材库并替换为 media_id。

### 2. 转换为微信 HTML

```bash
# 基本转换（不处理图片）
slingshot draft convert article.md
slingshot draft convert article.org    # 直接转换 Org mode

# 完整转换：上传图片 + 缩略图
slingshot draft convert article.md --upload
slingshot draft convert article.org --upload

输出：`article.html`（与输入文件同目录）。

### 3. 创建草稿

```bash
# 从 HTML 创建草稿
slingshot draft add article.html

# 指定标题（覆盖 HTML 中的 <title>）
slingshot draft add article.html --title "我的文章标题"

# 指定封面 media_id（覆盖 HTML 中的 <meta>）
slingshot draft add article.html --thumb <media_id>
```

### 4. 管理草稿

```bash
# 列出所有草稿
slingshot draft list

# 查看草稿详情（查看文章内容等）
slingshot draft show <media_id>

# 更新草稿
slingshot draft update <media_id> article.html

# 删除草稿
slingshot draft remove <media_id>
```

### 5. 管理素材

```bash
# 列出素材
slingshot meterial list
slingshot meterial list --type image
slingshot meterial list --type video
slingshot meterial list --type voice
slingshot meterial list --type news

# 上传素材
slingshot meterial add image.png
slingshot meterial add video.mp4 --type video --title "视频标题"
slingshot meterial add voice.amr --type voice

# 查看素材详情
slingshot meterial show <media_id>
slingshot meterial show <media_id> --output image.jpg

# 删除素材
slingshot meterial remove <media_id>
```

## 端到端示例

```bash
# 1. 配置凭据（仅首次）
slingshot config set wechat.appid wx1234567890abcdef
slingshot config set wechat.secret abcdefghijklmnopqrstuvwxyz123456

# 2a. Markdown → 转换 → 创建草稿
slingshot draft convert my-article.md --upload
slingshot draft add my-article.html

# 2b. Org mode 同样支持（需安装 Emacs）
slingshot draft convert my-article.org --upload
slingshot draft add my-article.html

# 3. 查看草稿列表
slingshot draft list

# 4. 查看创建结果
slingshot draft show <media_id>

# 5. 修改后更新
slingshot draft update <media_id> my-article.html

# 6. 管理封面图片
slingshot meterial list --type image
slingshot meterial add cover.jpg

## 注意事项

- **配置**：必须配置 `wechat.appid` 和 `wechat.secret`，否则所有 API 操作会失败
- **封面图**：微信草稿要求必须提供 `thumb_media_id`。可以通过 sidecar YAML
  （`<file>.yaml`）或 HTML `<meta name="thumb_media_id">` 指定
- **摘要**：可通过 sidecar YAML 的 `digest` 字段或 HTML `<meta name="digest">` 设置
- **图片缓存**：首次上传的图片会缓存到 `images.yaml`（当前目录），下次跳过重复上传
- **缩略图缓存**：封面图片的 media_id 也会缓存到 `images.yaml`，避免重复上传
- **Emacs 依赖**：转换 `.org` 文件需要系统安装 `emacs`（Emacs 26+）。
  大多数 Linux/macOS 环境默认可用；若缺失，降级使用 `.md` 文件即可。
