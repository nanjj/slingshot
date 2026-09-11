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
├── tikz_engine.go                  # tikz 引擎选择 (xe/tectonic/auto) + 兼容 profile
├── tikz_proc_unix.go               # 进程组超时清理 (unix)
├── tikz_proc_other.go              # 进程组超时清理 (windows 退化实现)
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
├── tikz <in-file> <out-file> [--engine xe|tectonic|auto]
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

`slingshot tikz <in-file> <out-file> [--engine xe|tectonic|auto]` 把 TikZ 片段渲染成
png/jpg/svg/pdf。输出格式由输出文件扩展名决定（PDF 直接产出；png/svg 走 mutool，
jpg 走 ghostscript 栅格化）。

`--engine` 的值命名的是 **TeX 引擎**而不是驱动：`xe`（默认）经 `latexmk -xelatex`
走 TeX Live 2026 新版语法；`tectonic` 用内置的 2021 年旧版 tkz-euclide 4.051b /
circuitikz 1.4.x bundle；`auto` 优先 xe、不可用时回退 tectonic；`pdf` / `lua` 是保留
取值，`selectTikzEngine` 直接报「尚未实现」。显式指定引擎时探测失败直接报错，编译失败
不自动换后端；只有 `auto` 才在 xe 不可用时回退 tectonic。

外部命令（latexmk / tectonic / mutool / gs）每条都有超时上限：`TIKZ_TIMEOUT`
（Go duration，默认 `1m`）。超时由 `runCmd` 的 `context.WithTimeout` 触发，杀掉**整个
进程组**（`tikz_proc_unix.go` 的 `Setpgid` + `kill(-pid, SIGKILL)`）——latexmk → xelatex
→ xdvipdfmx 是进程树，只杀直接子进程会留下孤儿。Windows 上退化为杀直接子进程。
这条兜底是必需的：旧语法 `\tkzDrawCircle[circum](A,B,C)` 会让 pgfkeys 无限自展开
（100% CPU、无任何输出），没有超时就会挂死调用方。

兼容 shim 按后端 profile 门控（`tikz_engine.go` 的 `tikzProfile`）：
tectonic 上做 tkz-euclide 5.x → 4.051b 语法翻译、2021-bundle 兼容 shim（buzzer/converter/
apollonius/IEC）；xe/pdf/lua profile 全 false，不启用这些翻译与 shim（否则在新语法上「反向出错」）。
IEC 风格在 xe 上注入 \usetikzlibrary{circuits.ee.IEC}（真库），tectonic 上用 circuitikz shim。
motor shim 两个后端都保留——上游 circuitikz 从来没有 motor 元件（圆圈 + M），只能定制补齐。
tikzlings 的 pic 语法在 xe 上注入 `\usetikzlibrary{tikzlings}`（真库，v2.x 才有）；tectonic
bundle（v0.8）没有该库文件，改用动物子宏包 + `tikzlingsPicShim`（复刻库的 `<name>/.pic`
与 `thing/.search also`；缺 `thing/.search also` 时 `pic[thing/hat=..]` 报 "I do not know
the key '/tikz/thing/hat'"）。

内容探测（`tikzExtraPackages` / `tikzlingsCommands`）：命中特征即加载对应宏包。tikzlings
的动物命令映射到各自子宏包（`tikzlings-marmots` 等，表取自 tikzlings-list.sty），`\tikzling`
走基础宏包、`\thing` 走 tikzlings-addons；命令匹配带词边界，`\bearwear` 不会命中 `\bear`。
`\bearwear`（bearwear 宏包，独立 CTAN 包，给 tikzlings-bears 的熊提供服装）由 tikzExtraPackages
的 `\bearwear` 子串命中（一并覆盖 `\bearwearsetup` / `\bearwearlogo`），注入 `\usepackage{bearwear}`；
包内自行 `\RequirePackage{tikzlings-bears}`，两后端 bundle 均自带该包。
tikzlings 的 pic 语法（`pic{bear}` / `pic[coati/body=blue, scale=0.5]{coati}` /
`pic[thing/hat=red]{penguin}`）由 `tikzPicRe` / `detectTikzlingsPics` 单独探测：`/tikz/pics/<name>`
键只由 TikZ 库文件（`tikzlibrarytikzlings.code.tex`）定义，动物子宏包不带 pic 定义——漏检时
pgfkeys 报 "I do not know the key '/tikz/pics/bear'"；legacy 后端要加载的动物子宏包由
`tikzlingsPicPackages` 提供。
手册示例常用的 `tcblisting` 盒子由 `tcblistingSetup` 注入 `\tcbuselibrary{listings}` +
`\tcbset{tikz lower}`：tcblisting 的 text 部分默认在 tikzpicture 之外，而 TikZ 只在 picture
内安装 `\path` / `\draw` / `scope`，不注入就报 "Environment scope undefined"；
`tikzSelfContainedEnvs` 同时收录 `tcblisting`，避免再套外层 tikzpicture（套了会被 pgf
包围盒裁切）。两个后端都支持（2021 bundle 自带 tcolorbox + listings）。

pgf 的 `3d` 库（`tikzlibrary3d.code.tex`）定义 `canvas is <xy|yx|xz|zx|yz|zy> plane at
<axis>=` 与裸 `canvas is plane` 坐标系/选项，由 `tikzExtraLibraries` 的内容特征正则自动加载
（命中即注入 `\usetikzlibrary{3d}`，库本身只定义坐标系、对现有片段无副作用）；tikzlings
手册的 z-order/rhino 分层切片示例依赖它。

文档局部颜色兜底（`tikzDocColors` / `tikzDocColorShims`，与 `ensureNewStyle` 同类）：手册
片段若引用了源文档 preamble 自定义的颜色（如 tikzlings 的 `themecolor`，其值 samviolet =
RGB(136,46,114)），工具会在内容引用该名称且未自行定义时注入 `\providecolor{themecolor}
{RGB}{136,46,114}`；`\providecolor` 语义保证片段里自己的 `\definecolor` / `\colorlet` 优先
（显式定义同名颜色即覆盖兜底值）。

CJK：内容含 CJK 时两个后端都注入 fontspec + xeCJK 前导（tectonic bundle 自带 xeCJK，
无需探测）；仅 xe 在探测阶段用 kpsewhich 检查 xeCJK.sty。字体可用 `TIKZ_CJK_FONT`
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
