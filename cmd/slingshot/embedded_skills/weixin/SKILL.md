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

# 更新草稿（自动检测 media_id：sidecar YAML 优先 → 兜底第一篇草稿）
slingshot draft update article.html

# 更新草稿（多图文第 N 篇，0-based）
slingshot draft update article.html --index 1

# 更新草稿并换封面
slingshot draft update article.html --thumb <media_id>

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

## 更新已发布文章（实战 SOP）

> 场景：文章同时发布到个人站点 + 微信公众号，改完源文件（`.org`/`.md`）后两处同步更新；
> 按本节顺序执行。

### 关键陷阱：`<name>.html` 被两条管线共用

源文件旁生成的 `<name>.html` 有两个产生者，内容不同、先后互相覆盖：

- `slingshot page update <site> <name>.org` → 生成**站点版**（Emacs 导出）
- `slingshot draft convert <name>.org --upload` → 生成**微信版**（内联样式、SVG 公式、CDN 图片）

另注意：`slingshot draft update <name>.org`（直接传 .org）在「`.html` 不比 `.org` 旧」时
**直接复用现有 `.html`**、不重新转换——page 更新后 `.html` 恰是站点版，会原样发进草稿。

**所以：顺序固定「先站点、后微信」；微信侧始终显式 `draft convert` 重建。**

> ⚠️ 站点部署不要用裸 `slingshot site rsync`（2026-10-09 实测：重复高亮会把已高亮的代码块
> 二次转义，一次损坏 26 页）；用 `page update --rsync`——详见 site-page skill「部署」节警告。

### 标准步骤

```bash
# 0. 进入文章目录（例：tikzlings/jiheti01），确认源文件已改好

# 1. 站点：重建页面并部署（--rsync 一步完成；顺带把 <name>.html 覆盖为站点版，无妨——第 3 步会重建微信版）
slingshot page update --rsync nanjj.dscli.io <name>.org

# 2. 站点：复核 + 提交 + 推送（部署已在第 1 步完成）
cd ~/.local/src/gitlab.com/dscli/nanjj.dscli.io
git diff <name>/index.html        # 只应有正文变化 + 导出噪声（时间戳/锚点）
git add <name>/ index.html
git commit -m "Update <name> page"
HTTPS_PROXY=socks5h://localhost:8777 git push    # gitlab 需 socks5 代理
curl -s "https://nanjj.dscli.io/<name>/" | grep -c "<新文关键句>"    # ≥1 即上线

# 3. 微信：回到文章目录，重建微信版 HTML（图片/封面全走 images.yaml 缓存，零重传）
cd -
slingshot draft convert <name>.org --upload

# 4. 微信：更新草稿（media_id 自动读 sidecar <name>.yaml）
slingshot draft update <name>.html
```

### 更新后核验（必做）

```bash
# 元数据：标题/作者/摘要/来源 URL/封面/评论设置
slingshot draft show <name>.yaml

# 正文全文：直接调微信 API（密钥只进变量、不回显）
AID=$(slingshot config get wechat.appid | tr -d '[:space:]"')
SEC=$(slingshot config get wechat.secret | tr -d '[:space:]"')
TOK=$(curl -s "https://api.weixin.qq.com/cgi-bin/token?grant_type=client_credential&appid=$AID&secret=$SEC" | jq -r '.access_token')
MID=$(sed -n 's/^media_id: *//p' <name>.yaml)
curl -s "https://api.weixin.qq.com/cgi-bin/draft/get?access_token=$TOK" \
  -H 'Content-Type: application/json' -d "{\"media_id\":\"$MID\"}" \
  | jq -r '.news_item[0].content' > /tmp/draft-check.html

# 文本：新句在、旧句不在
grep -c "<新句>" /tmp/draft-check.html            # 期望 ≥1
grep -c "<被替换的旧句>" /tmp/draft-check.html    # 期望 0

# 图片：数量、域名、缓存归属
grep -o 'src="[^"]*"' /tmp/draft-check.html | wc -l           # = 文章图片数
grep -o 'src="[^"]*"' /tmp/draft-check.html | grep -v mmbiz   # 期望空（全部 CDN）
```

核验要点：

- 草稿保存后微信把图片 URL 从 `/0` 改写为 `/640`（实际服务 ≤1080px rendition）——正常
  现象。比较图片时先去掉尾部 `/\d+`；精确对比用「回载 + 像素比」而非 md5。
- 图片 URL 路径集（去尾 `/\d+` 后）应 ⊆ `images.yaml` 缓存路径集。
- sidecar 的 `need_open_comment` / `only_fans_can_comment` 留空（`null`）时默认 `1/1`（开启、仅粉丝）。
- 图片与封面缓存按内容 md5 存在 `images.yaml`：源图未变时 `convert --upload` 全部命中缓存（输出「使用缓存」），不重传。

### 网络与凭据

- 微信 API 走**直连**（勿设 `HTTPS_PROXY`）；出口 IP 须在公众号「IP 白名单」内
  （`curl -s https://ipinfo.io/ip` 查看，变更后到公众号后台更新白名单）。
- 本机 gitlab 推送需 `HTTPS_PROXY=socks5h://localhost:8777`（端口见 `slingshot config get proxy.port`）；
  gitcode 走 SSH 直连。

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

# 5. 修改后更新（自动用 sidecar YAML 中的 media_id）
slingshot draft update my-article.html

# 6. 管理封面图片
slingshot meterial list --type image
slingshot meterial add cover.jpg
```

## 注意事项

- **配置**：必须配置 `wechat.appid` 和 `wechat.secret`，否则所有 API 操作会失败
- **封面图**：微信草稿要求必须提供 `thumb_media_id`。可以通过 sidecar YAML
  （`<file>.yaml`）或 HTML `<meta name="thumb_media_id">` 指定
- **摘要**：可通过 sidecar YAML 的 `digest` 字段或 HTML `<meta name="digest">` 设置
- **图片缓存**：首次上传的图片会缓存到 `images.yaml`（当前目录）。下次 `--upload` 跳过重复上传；`draft add/update` 也会自动从缓存中查找 CDN URL 替换本地路径，无需重新上传
- **缩略图缓存**：封面图片的 media_id 也会缓存到 `images.yaml`，避免重复上传
- **Emacs 依赖**：转换 `.org` 文件需要系统安装 `emacs`（Emacs 26+）。
  大多数 Linux/macOS 环境默认可用；若缺失，降级使用 `.md` 文件即可。
- **微信 API 网络**：直连（勿设 `HTTPS_PROXY`）；出口 IP 须在公众号「IP 白名单」内（`curl -s https://ipinfo.io/ip` 查看）
- **git 推送网络**：本机 gitlab 需 `HTTPS_PROXY=socks5h://localhost:8777`；gitcode 走 SSH 直连
- **同名文件 `<name>.html` 双管线**：更新顺序「先站点、后微信」，详见「更新已发布文章（实战 SOP）」
- **站点部署**：用 `page update --rsync nanjj.dscli.io <name>.org`；避免裸 `slingshot site rsync`（重复高亮会破坏代码块，2026-10-09 实测）

## SVG 图片处理

微信公众平台**不支持 SVG 格式图片**。`--upload` 流程会自动将 SVG 转换为 PNG。

### 系统依赖

需要安装 `rsvg-convert`（来自 `librsvg`）：

```bash
# Debian/Ubuntu
apt install librsvg2-bin

# macOS
brew install librsvg

# Arch Linux
pacman -S librsvg
```

若 `rsvg-convert` 未安装，SVG 文件会原样传给微信（上传将失败）。

### 实现机制

- **检测**：按文件后缀（`.svg`，大小写不敏感）识别 SVG
- **转换**：调用 `rsvg-convert` 生成临时 PNG 文件
- **缓存**：按原始 SVG 文件内容（md5）缓存上传后的 URL，SVG 未变时不重复转换/上传
- **适用范围**：文章内图片 + 封面缩略图均自动转换

### 覆盖的上传路径

| 路径 | 说明 |
|------|------|
| `convert --upload` 内容图片 | 自动 SVG→PNG 后上传 |
| `convert --upload` 缩略图 | 自动转换 |
| `draft add --thumb` | 封面 SVG 自动转换 |
| `draft update --thumb` | 封面 SVG 自动转换 |

> 维护：本 skill 上游 = slingshot 仓库 `cmd/slingshot/embedded_skills/weixin/SKILL.md`；
> 修改后 `make install` 重建二进制，再到项目里 `slingshot skill install weixin` 刷新安装副本。
