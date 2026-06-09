# Slingshot — AI 智能体 CLI 工具

**模块**: `github.com/nanjj/slingshot` | **语言**: Go 1.26.4 | **许可证**: Apache 2.0

微信公众号 Markdown 转换 + 草稿管理 CLI，基于 Cobra + 声明式参数解析器。

## 目录结构

```
cmd/slingshot/       # 主入口 + 子命令 (main.go, config.go, wxdraft.go, wxdraft_convert.go)
internal/
  cmd/shared.go      # 共享 CLI 工具
  config/config.go   # YAML 配置管理
  i18n/              # 国际化引擎 (embed .po + locales/)
  usage/usage.go     # 声明式 Atom 参数解析器 (从 incus CLI 精炼)
  mdtowx/            # Markdown → 微信 HTML 转换 (goldmark + inline styles)
                     #   + 图片路径提取与 URL 替换
  getaccesstoken/    # 微信 Access Token 管理 (config 缓存 + 自动刷新)
  uploadimage/       # 微信素材上传 (POST /cgi-bin/media/uploadimg)
Makefile             # 构建/测试/发布
```

## 架构要点

1. **双层 CLI**: Cobra 注册子命令 + `internal/usage` 声明式 Atom 解析（支持 `--explain` 诊断）
2. **子命令模板**: `cmdConfigSub` / `cmdWxdraftSub` 结构体模板减少样板代码
3. **i18n**: `i18n.G("msg")` 包裹字符串 → embed .po → 环境变量选择语言（回退原文）
4. **全局状态**: `cmdGlobal` 持有 Help/Version/Quiet/Explain 标志，通过 `Parse()` 包装注入配置
5. **配置**: `~/.config/slingshot/config.yml` — YAML，`config.Load/Save/Get/Set/Del/AllKeys`
6. **微信 Token**: `getaccesstoken.GetToken()` 先查 config 缓存，过期后自动请求 + 缓存
7. **图片上传**: `uploadimage.Upload()` 上传本地图片，返回微信图片 URL

## mdtowx 转换流程

`slingshot wxdraft convert [--upload|-u] <file.md>`

1. **Markdown → HTML**: goldmark 解析 + 内联样式注入 → `<filename>.html`
2. **图片提取** (`--upload`): 解析 `<img src="...">`，收集本地文件路径
3. **图片上传** (`--upload`): 上传到微信素材库，获取 `mmbiz.qpic.cn` URL
4. **URL 替换** (`--upload`): 更新 HTML 中的 `src` 为微信 URL，重新保存

## 构建与测试

| 命令 | 说明 |
|------|------|
| `make build` | `go build ./cmd/slingshot` |
| `make test` | `go test -v -count=1 ./...` |
| `make install` | `go install ./cmd/slingshot` |
| `make release` | 交叉编译 linux/macos/windows × amd64/arm64 (+ UPX) |
| `make fmt` | `go fmt ./...` |

Release: `CGO_ENABLED=0` + `-ldflags="-s -w"`，产物输出到 `_release/`。

## 贡献约定

- **DCO**: 每个 commit 需 `Signed-off-by: JUN JIE NAN <nanjunjie@gmail.com>`
- **Commit 前缀**: `feat:` / `fix:` / `docs:` / `refactor:` / `i18n:`
- **i18n**: 修改 CLI 字符串后同步更新 `.pot` 和对应 `.po`
- **格式化**: `make fmt` 保持风格
