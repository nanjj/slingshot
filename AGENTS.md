# Slingshot — AI 智能体 CLI 工具

**模块**: `github.com/nanjj/slingshot` | **语言**: Go 1.26.4 | **许可证**: Apache 2.0

微信公众号 Markdown 转换 + 草稿管理 CLI，基于 Cobra + 声明式参数解析器。

## 目录结构

```
cmd/slingshot/                      # 主入口 + 子命令
├── main.go                         # 根命令 + 全局标志 + 子命令注册
├── config.go                       # config 子命令
├── draft.go                        # draft 子命令 (含 sidecar YAML)
├── draft_convert.go                # draft convert + --upload
├── meterial.go                     # meterial 子命令
├── skill.go                        # skill 子命令 (含 embedded skills)
├── amap.go                         # amap 子命令 (高德地图 MCP)
└── embedded_skills/weixin/SKILL.md  # 内置 skill (嵌入 binary)
internal/
├── cmd/shared.go                   # 共享 CLI 工具
├── config/config.go                # YAML 配置管理
├── draft/draft.go                  # 微信草稿 API
├── getaccesstoken/getaccesstoken.go# Access Token 管理 (缓存 + 自动刷新)
├── amap/amap.go                    # 高德地图 MCP 客户端 (JSON-RPC, 无状态)
├── i18n/                           # 国际化 (.po + locales/)
├── material/material.go            # 永久素材 API
├── mdtowx/mdtowx.go               # Markdown → 微信 HTML (goldmark + inline styles)
├── uploadcache/uploadcache.go      # 图片上传缓存 (images.yaml, 按 md5 去重)
├── uploadimage/                    # 微信图片上传 (文章内/封面)
└── usage/                          # 声明式 Atom 参数解析器
```

## 子命令

```
slingshot
├── draft list|add|update|remove|show|convert <file>
├── config list|show|get|set|unset
├── meterial add|list|remove|show
├── skill list|install
└── amap search|around|detail|geo|regeo|driving|walking|bicycling|transit|distance|ip
```

## 架构要点

1. **双层 CLI**: Cobra 注册子命令 + `internal/usage` 声明式 Atom 解析（支持 `--explain` 诊断）
2. **i18n**: `i18n.G("msg")` 包裹 → embed .po → 环境变量选语言
3. **Token 缓存**: 先查 config 缓存，过期自动请求 + 缓存
4. **元数据优先级**: sidecar YAML > front matter > HTML `<meta>` > 默认值
5. **图片缓存**: `images.yaml`，按 md5 避免重复上传
6. **内置 Skill**: `//go:embed` 嵌入 SKILL.md，`skill install` 提取到项目

## draft convert 流程

基础 (`slingshot draft convert file.md`):
1. 解析 YAML front matter → 提取 title/author/thumb_media_id/digest
2. Markdown → HTML（goldmark + 内联样式）
3. 元数据写入 HTML `<title>` / `<meta>`

加 `--upload` 额外做：
4. 提取 `<img src="...">` → 上传到微信获取 `mmbiz.qpic.cn` URL（含 `images.yaml` 缓存）
5. 替换 HTML 中 `src`
6. front matter 中封面路径自动上传为永久素材

## 构建与测试

| 命令 | 说明 |
|------|------|
| `make build` | `go build ./cmd/slingshot` |
| `make test` | `go test -v -count=1 ./...` |
| `make install` | `go install ./cmd/slingshot` |
| `make release` | 交叉编译 (linux/darwin/windows × amd64/arm64 + UPX) |
| `make fmt` | `go fmt ./...` |

Release: `CGO_ENABLED=0` + `-ldflags="-s -w"`

## 贡献约定

- **DCO**: commit 需 `Signed-off-by: JUN JIE NAN <nanjunjie@gmail.com>`
- **前缀**: `feat:` / `fix:` / `docs:` / `refactor:` / `i18n:`
- **i18n**: 修改 CLI 字符串后同步更新 `.pot` / `.po`
- **格式化**: `make fmt`
