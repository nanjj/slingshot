# slingshot

![img](./slingshot.png)

微信公众号文章发布工作流 CLI。Markdown → 公众号草稿一条龙。

## 安装

```bash
go install github.com/nanjj/slingshot/cmd/slingshot@latest
# 或 make build
```

## 用法

```bash
# 1. 配置凭据
slingshot config set wechat.appid APPID
slingshot config set wechat.secret SECRET

# 2. 转换 Markdown → 微信 HTML（可选 --upload 自动上传图片）
slingshot draft convert article.md --upload

# 3. 创建草稿
slingshot draft add article.html
```

### 子命令

```
slingshot
├── draft    list|add|update|remove|show|convert <file>
├── config   list|show|get|set|unset
├── meterial add|list|remove|show
└── skill    list|install
```

### sidecar YAML

同名 YAML 文件覆盖/补充 front matter（`article.md` → `article.yaml`）：

```yaml
title: 标题
author: 作者
thumb_media_id: cover.png  # 本地路径自动上传
digest: 摘要...
```

优先级: sidecar YAML > front matter > HTML `<meta>` > 默认值

### 诊断模式

加 `--explain` 查看参数解析结果，不实际执行。

## 构建

| 命令 | 说明 |
|------|------|
| `make build` | 编译 |
| `make test` | 测试 |
| `make install` | 安装 |
| `make release` | 交叉编译 |

## 许可证

Apache 2.0 © 2025 JUN JIE NAN
