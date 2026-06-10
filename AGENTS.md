# Slingshot — AI 智能体 CLI 工具

**模块**: `github.com/nanjj/slingshot` | **语言**: Go 1.26.4 | **许可证**: Apache 2.0

微信公众号 Markdown 转换 + 草稿管理 CLI，基于 Cobra + 声明式参数解析器。

## 目录结构

```
cmd/slingshot/                     # 主入口 + 子命令
├── main.go                        # 根命令 + 全局标志 + 子命令注册
├── config.go                      # config 子命令 (list/show/get/set/unset)
├── draft.go                       # draft 子命令 (list/add/update/remove/show) + sidecar YAML
├── draft_convert.go               # draft convert 子命令 (Markdown→WeChat HTML) + --upload
├── meterial.go                    # meterial 子命令 (add/list/remove/show)
├── skill.go                       # skill 子命令 (list/install) + embedded skills
└── embedded_skills/
    └── weixin/SKILL.md            # 内置 weixin skill (SKILL.md 嵌入 binary)

internal/
├── cmd/shared.go                  # 共享 CLI 工具 (FormatSection 等)
├── config/config.go               # YAML 配置管理 (~/.config/slingshot/config.yml)
├── draft/draft.go                 # 微信草稿 API (add/list/update/remove/show)
├── getaccesstoken/getaccesstoken.go  # 微信 Access Token 管理 (config 缓存 + 自动刷新)
├── i18n/                          # 国际化引擎 (embed .po + locales/)
│   ├── locales.go
│   └── locales/zh_CN/
├── material/material.go           # 微信永久素材 API (add/list/remove/show)
├── mdtowx/mdtowx.go              # Markdown → 微信 HTML 转换 (goldmark + inline styles)
│                                  #   + YAML front matter 解析
│                                  #   + 图片路径提取与 URL 替换
├── uploadcache/uploadcache.go     # 图片上传缓存 (images.yaml, 避免重复上传)
├── uploadimage/                   # 微信图片上传
│   ├── uploadimage.go             #   POST /cgi-bin/media/uploadimg (文章内图片)
│   └── uploadthumb.go             #   POST /cgi-bin/material/add_material?type=image (封面)
└── usage/                         # 声明式 Atom 参数解析器 (从 incus CLI 精炼)
    ├── doc.go
    ├── usage.go
    ├── parsed.go
    ├── errors.go
    ├── diagnose.go
    ├── placeholder.go             # u.File, u.ID, u.Key, u.Value 等占位符
    ├── flag.go / list.go / optional.go / compound.go / verbatim.go / hide.go
    ├── alternative.go / deprecated.go
    └── *_test.go

Makefile                           # 构建/测试/发布
```

## 子命令树

```
slingshot
├── draft                          # 微信草稿管理
│   ├── list                       #   列出草稿
│   ├── add    <file>              #   从 HTML 创建草稿
│   ├── update <id> <file>         #   更新草稿
│   ├── remove <id>                #   删除草稿
│   ├── show   <id>                #   查看草稿详情
│   └── convert <file> [--upload]  #   Markdown → 微信 HTML 转换
├── config                         # 配置管理 (~/.config/slingshot/config.yml)
│   ├── list                       #   列出所有配置键
│   ├── show                       #   显示完整配置
│   ├── get    <key>               #   获取配置值
│   ├── set    <key> <value>       #   设置配置值
│   └── unset <key>                #   删除配置键
├── meterial                       # 微信永久素材管理
│   ├── add    <file>              #   上传素材 (图片/视频/语音)
│   ├── list                       #   列表素材 (支持 --type/--offset/--count)
│   ├── remove <id>                #   删除素材
│   └── show   <id>                #   查看/下载素材
└── skill                          # AI Agent 技能管理
    ├── list                       #   列出内置技能
    └── install <name>             #   安装内置技能到项目
```

## 架构要点

1. **双层 CLI**: Cobra 注册子命令 + `internal/usage` 声明式 Atom 解析（支持 `--explain` 诊断）
2. **子命令模板**: `cmdConfigSub` / `cmdDraftSub` / `cmdSkillSub` 结构体模板减少样板代码
3. **i18n**: `i18n.G("msg")` 包裹字符串 → embed .po → 环境变量选择语言（回退原文）
4. **全局状态**: `cmdGlobal` 持有 Help/Version/Quiet/Explain 标志，通过 `Parse()` 包装注入配置
5. **配置**: `~/.config/slingshot/config.yml` — YAML，`config.Load/Save/Get/Set/Del/AllKeys`
6. **微信 Token**: `getaccesstoken.GetToken()` 先查 config 缓存，过期后自动请求 + 缓存
7. **图片上传**: `uploadimage.Upload()` 上传文章内图片，`UploadThumb()` 上传封面素材
8. **上传缓存**: `uploadcache` 维护 `images.yaml`，按 md5 避免重复上传（文章图片 + 封面）
9. **元数据解析优先级**: sidecar YAML > 内嵌 front matter > HTML <meta> > 默认值
10. **内置 Skill**: `//go:embed embedded_skills` 将 SKILL.md 嵌入二进制，`skill install` 提取到项目

## draft convert 转换流程

`slingshot draft convert [--upload|-u] <file.md>`

### 基础流程 (无 --upload)

1. **YAML front matter 解析**: 提取 title/author/thumb_media_id/digest
2. **Markdown → HTML**: goldmark 解析 + 内联样式注入 → `<filename>.html`
3. **元数据写入 HTML**: `<title>`, `<meta name="author">`, `<meta name="thumb_media_id">`, `<meta name="digest">`
4. 若 `thumb_media_id` 是本地路径（如 `cover.png`），给出提示建议使用 `--upload`

### 完整流程 (--upload)

1-3. 同上
4. **图片提取**: 解析 `<img src="...">`，收集本地文件路径
5. **图片上传**: 上传到微信素材库，获取 `mmbiz.qpic.cn` URL（含 `images.yaml` 缓存）
6. **URL 替换**: 更新 HTML 中的 `src` 为微信 URL
7. **缩略图上传**: 若 front matter 含本地图片路径，自动上传为永久素材
8. 保存最终 HTML

### draft add/update 缩略图自动上传

`slingshot draft add <file>` 和 `draft update <id> <file>`

- 解析 `--thumb` 值：若为本地图片路径（如 `cover.png`），自动调用 `uploadimage.UploadThumb`
- 缓存到 `images.yaml`（基于 md5），避免重复上传
- 支持侧边 YAML 文件 (`<file>.yaml`) 存放 title/author/thumb_media_id/digest

## 构建与测试

| 命令 | 说明 |
|------|------|
| `make build` | `go build ./cmd/slingshot` |
| `make test` | `go test -v -count=1 ./...` |
| `make install` | `go install ./cmd/slingshot` |
| `make release` | 交叉编译 linux/macos/windows × amd64/arm64 (+ UPX) |
| `make fmt` | `go fmt ./...` |
| `make clean` | 清理 build 产物 |

Release: `CGO_ENABLED=0` + `-ldflags="-s -w"`，产物输出到 `_release/`。

## 贡献约定

- **DCO**: 每个 commit 需 `Signed-off-by: JUN JIE NAN <nanjunjie@gmail.com>`
- **Commit 前缀**: `feat:` / `fix:` / `docs:` / `refactor:` / `i18n:`
- **i18n**: 修改 CLI 字符串后同步更新 `.pot` 和对应 `.po`
- **格式化**: `make fmt` 保持风格
