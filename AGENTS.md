# Slingshot — AI 智能体 CLI 工具

**模块**: `github.com/nanjj/slingshot` | **语言**: Go 1.27.1 | **许可证**: Apache 2.0

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
├── tikz.go                         # tikz 子命令 (TikZ → png/jpg/svg/pdf)
├── tikz_engine.go                  # tikz 引擎选择 (latexmk/tectonic) + 兼容 profile
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
├── mathrender/mathrender.go        # 公式渲染 (MathJax SVG + latexmk/tectonic PNG 兜底)
└── usage/                          # 声明式 Atom 参数解析器
```

## 子命令

```
slingshot
├── draft list|add|update|remove|show|convert <file>
├── config list|show|get|set|unset
├── meterial add|list|remove|show
├── skill list|install
├── tikz <in-file> <out-file> [--engine auto|latexmk|tectonic]
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

## TikZ 渲染管线

`slingshot tikz <in-file> <out-file> [--engine auto|latexmk|tectonic]` 把 TikZ 片段渲染成
png/jpg/svg/pdf。输出格式由输出文件扩展名决定（PDF 直接产出；png/svg 走 mutool，
jpg 走 ghostscript 栅格化）。

编译后端：latexmk -xelatex（TL2026 新版语法）为主 → tectonic（内置旧版 tkz-euclide 4.051b /
circuitikz 1.4.x bundle）后备 → 两者都不可用时明确报错。`--engine` 显式指定时探测失败直接报错，
编译失败不自动换后端；只有 `auto` 才在 latexmk 缺失时回退 tectonic。

兼容 shim 按后端 profile 门控（`tikz_engine.go` 的 `tikzProfile`）：
tectonic 上做 tkz-euclide 5.x → 4.051b 语法翻译、2021-bundle 兼容 shim（buzzer/converter/
apollonius/IEC）；latexmk profile 全 false，不启用这些翻译与 shim（否则在新语法上「反向出错」）。
IEC 风格在 latexmk 上注入 \usetikzlibrary{circuits.ee.IEC}（真库），tectonic 上用 circuitikz shim。
motor shim 两个后端都保留——上游 circuitikz 从来没有 motor 元件（圆圈 + M），只能定制补齐。

CJK：内容含 CJK 时两个后端都注入 fontspec + xeCJK 前导（tectonic bundle 自带 xeCJK，
无需探测）；仅 latexmk 在探测阶段用 kpsewhich 检查 xeCJK.sty。字体可用 `TIKZ_CJK_FONT`
环境变量覆盖（默认 Noto Sans CJK SC）；该值必须是有效字体族名，不要包含 `{` `}` `%` `\`
等 TeX 特殊字符（会直接拼进 `\setCJKmainfont{...}`）。字体缺失时 fontspec 会报错退出。

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
- **i18n**: 翻译必须用 `slingshot i18n translate`（`--id` 优先），禁止直接编辑 `.po` 或写脚本；改代码后先 `i18n sync` 再 `i18n translate`，最后 `i18n stats` + `i18n check --exit-code` 验证
- **格式化**: `make fmt`
