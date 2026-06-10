# slingshot

![img](./slingshot.png)

**Slingshot** — 微信公众号文章发布工作流 CLI。从 Markdown 到公众号草稿的完整链路。

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue)](LICENSE)

---

## 功能

- **Markdown → 微信 HTML**：基于 goldmark 的 GFM 转换，自动注入内联样式
- **一键上传图片**：`--upload` 自动上传本地图片到微信素材库，避免手动操作
- **草稿管理**：创建、查看、更新、删除公众号草稿
- **素材管理**：上传/查看/下载/删除永久素材（图片、视频、语音）
- **YAML 元数据**：支持 YAML front matter 和 sidecar YAML 文件设置标题/作者/封面/摘要
- **缩略图自动上传**：封面图片路径自动转换为微信 media_id
- **--explain 诊断**：在真正执行前查看参数如何被解析
- **国际化 (i18n)**：支持中/英文 CLI 输出

## 安装

```bash
# Go 安装
go install github.com/nanjj/slingshot@latest

# 或从源码编译
git clone https://github.com/nanjj/slingshot.git
cd slingshot
make build
```

## 快速开始

### 1. 配置微信凭据

```bash
# 在公众号后台 → 开发 → 基本配置 中获取 AppID 和 AppSecret
slingshot config set wechat.appid wx1234567890abcdef
slingshot config set wechat.secret abcdefghijklmnopqrstuvwxyz123456
```

### 2. 写一篇 Markdown 文章

```markdown
---
title: 我的第一篇公众号文章
author: 作者名
thumb_media_id: cover.png
---

# 你好，世界！

这是一篇通过 **Slingshot** 发布的文章。

![配图](./photo.jpg)
```

### 3. 转换并发布

```bash
# 转换 Markdown → 微信 HTML（自动上传图片和封面）
slingshot draft convert article.md --upload

# 创建草稿
slingshot draft add article.html

# 查看草稿列表
slingshot draft list
```

## 使用示例

### 转换 Markdown

```bash
# 基本转换
slingshot draft convert article.md

# 带图片上传的完整转换
slingshot draft convert article.md --upload
```

### 管理草稿

```bash
# 创建草稿（从转换后的 HTML）
slingshot draft add article.html --title "我的标题"

# 列出草稿
slingshot draft list

# 查看草稿详情
slingshot draft show <media_id>

# 更新草稿
slingshot draft update <media_id> article.html

# 删除草稿
slingshot draft remove <media_id>
```

### 管理素材

```bash
# 上传图片素材
slingshot meterial add image.png

# 上传视频素材（需指定标题）
slingshot meterial add video.mp4 --type video --title "视频标题"

# 列出素材
slingshot meterial list --type image

# 查看素材详情
slingshot meterial show <media_id>

# 下载素材
slingshot meterial show <media_id> --output downloaded.jpg

# 删除素材
slingshot meterial remove <media_id>
```

### 配置管理

```bash
# 查看完整配置
slingshot config show

# 获取某个配置值
slingshot config get wechat.appid

# 设置配置
slingshot config set wechat.appid wx1234567890abcdef

# 删除配置项
slingshot config unset wechat.appid
```

### 诊断模式

```bash
# 查看参数解析结果（实际不执行）
slingshot draft convert article.md --explain
```

## sidecar YAML

不想修改 Markdown 文件本身？在同目录放一个同名 YAML 文件即可：

```yaml
# article.md 的 sidecar 文件: article.yaml
title: 文章标题
author: 作者名
thumb_media_id: cover.png
digest: 文章摘要...
```

优先级：**sidecar YAML > YAML front matter > HTML <meta>**

## 图片缓存

首次上传的图片会缓存到 `images.yaml`（当前目录），下次上传相同文件（基于 md5 校验和）会跳过并复用已有 URL。

## 子命令参考

```
slingshot
├── draft                        # 微信草稿管理
│   ├── list                     #   列出草稿
│   ├── add    <file>            #   创建草稿
│   ├── update <id> <file>       #   更新草稿
│   ├── remove <id>              #   删除草稿
│   ├── show   <id>              #   查看草稿
│   └── convert <file> [--upload] #  Markdown → HTML
├── config                       # 配置管理
│   ├── list                     #   列出配置键
│   ├── show                     #   显示配置
│   ├── get    <key>             #   获取配置
│   ├── set    <key> <value>     #   设置配置
│   └── unset <key>              #   删除配置
├── meterial                     # 素材管理
│   ├── add    <file>            #   上传素材
│   ├── list                     #   列出素材
│   ├── remove <id>              #   删除素材
│   └── show   <id>              #   查看素材
└── skill                        # AI 技能管理
    ├── list                     #   列出内置技能
    └── install <name>           #   安装技能
```

## 许可证

Apache 2.0 © 2025 JUN JIE NAN
