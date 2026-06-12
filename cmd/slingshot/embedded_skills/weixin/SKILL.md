---
name: weixin
description: 微信公众号文章草稿全流程 — Markdown 转 HTML、上传图片、管理素材、创建/更新草稿、发布/群发/预览
keywords: weixin, wechat, draft, meterial, article, 公众号, 草稿, 素材, publish, preview, mass, 发布, 群发
author: JUN JIE NAN <nanjunjie@gmail.com>
---

# weixin

微信公众号文章发布工作流。从 Markdown 到公众号草稿、再到发布/群发的完整链路。

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

### 1. 准备 Markdown 文件

编写文章内容，支持 GFM 语法（表格、删除线、代码块等）。

支持两种方式设置元数据（标题、作者、摘要、封面图）：

**方式一：YAML front matter**（嵌入在 Markdown 文件头部）

```yaml
---
title: 文章标题
author: 作者名
digest: 摘要内容（选填）
thumb_media_id: cover.png
---
```

**方式二（推荐）：sidecar YAML 文件**（与 Markdown 同目录，命名为 `<filename>.yaml`）

```yaml
# article.md 的同目录 sidecar 文件: article.yaml
title: 文章标题
author: 作者名
digest: 摘要内容（选填）
thumb_media_id: cover.png
```

Sidecar YAML 优先级高于 front matter，适合不修改原始 Markdown 文件添加元数据。
尤其适合从 org-mode 等外部工具导出的场景——Markdown 本身保持干净，元数据放在旁边。

> `thumb_media_id` 可以是本地图片路径（如 `cover.png`），
> 配合 `--upload` 会自动上传到微信素材库并替换为 media_id。

### 2. 转换为微信 HTML

```bash
# 基本转换（不处理图片）
slingshot draft convert article.md

# 完整转换：上传图片 + 缩略图
slingshot draft convert article.md --upload
```

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

### 6. 发布/群发/预览

草稿创建完成后，根据需要选择发布方式：

```bash
# ① 发布到时间线（FreePublish），不群发推送（默认）
slingshot draft publish <media_id>

# ② 发布到时间线 + 群发给所有订阅者
slingshot draft publish <media_id> --mass

# ③ 发布到时间线 + 群发给指定标签组
slingshot draft publish <media_id> --mass --to-all=false --tag-id 2

# ④ 发布 + 群发（启用转载忽略）
slingshot draft publish <media_id> --mass --send-ignore-reprint

# ⑤ 发送预览给指定用户（发布前检查排版效果）
# 按 OpenID 发送
slingshot draft preview <media_id> --touser <openid>

# 按微信号发送（每天限 100 次）
slingshot draft preview <media_id> --towxname <wxname>
```

**draft publish 的两种模式**：

| 模式 | API | 时间线展示 | 订阅者推送 | 适用场景 |
|------|-----|-----------|-----------|---------|
| 不加 `--mass`（默认） | FreePublish | ✅ | ❌ | 仅发布到时间线，不打扰订阅者 |
| 加 `--mass` | FreePublish + SendAll | ✅ | ✅ | 正常发布 + 群发推送 |

**draft preview 与 draft publish 的关系**：

- `draft preview` 只是发送预览给特定用户检查排版，不会产生发布记录
- 预览确认无误后，再用 `draft publish` 正式发布

> **AI 智能体注意**：
> - `draft publish` 不加 `--mass` 返回 `publish_id`（通过 FreePublish API 追踪发布状态）
> - `draft publish --mass` 额外返回 `msg_id` 和 `msg_data_id`（通过 MassSend API 追踪群发状态）
> - `draft preview` 可选择按 OpenID（推荐）或微信号发送

## 端到端示例

```bash
# 1. 配置凭据（仅首次）
slingshot config set wechat.appid wx1234567890abcdef
slingshot config set wechat.secret abcdefghijklmnopqrstuvwxyz123456

# 2. 编写 Markdown → 转换 → 创建草稿
slingshot draft convert my-article.md --upload
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

# 7a. 预览排版效果
slingshot draft preview <media_id> --touser <openid>

# 7b. 确认无误后发布到时间线（不群发）
slingshot draft publish <media_id>

# 7c. 或发布到时间线并群发给所有订阅者
slingshot draft publish <media_id> --mass
```

## 注意事项

- **配置**：必须配置 `wechat.appid` 和 `wechat.secret`，否则所有 API 操作会失败
- **封面图**：微信草稿要求必须提供 `thumb_media_id`。可以通过 sidecar YAML
  （`<file>.yaml`）或 HTML `<meta name="thumb_media_id">` 指定
- **摘要**：可通过 sidecar YAML 的 `digest` 字段或 HTML `<meta name="digest">` 设置
- **图片缓存**：首次上传的图片会缓存到 `images.yaml`（当前目录），下次跳过重复上传
- **缩略图缓存**：封面图片的 media_id 也会缓存到 `images.yaml`，避免重复上传
- **群发限制**：微信对群发有频率限制（取决于公众号类型），`--mass` 超额会返回错误
- **预览限制**：`--towxname` 方式每天限 100 次，`--touser` 方式不限
