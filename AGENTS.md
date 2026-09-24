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
`\duck` / `\randuck`（tikzducks 宏包，独立 CTAN 包，橡皮鸭）在 `tikzExtraLibraries` 正则表命中，
注入手册推荐的加载形式 `\usetikzlibrary{ducks}`（库文件内部 `\usepackage{tikzducks}` 并给鸭子
定义 `duck/.pic`，是宏包的超集）；TL2026（v2.2）与 2021 bundle（v1.5）均自带库文件，两后端同
路径；`\b` 词边界避免误中 `\ducksay` 等其他宏包的命令。裸 `\duck` 片段由 `normalizeTikz` 自动补
`tikzpicture` 外壳——`\duck` 内部的 `\begin{scope}` 依赖 picture。
`businessman` 等 tikzpeople 人形是 node 选项键（`\node[businessman,minimum size=1.5cm]`），由
`tikzExtraPackageRes` 的键位正则命中（`[` 或 `,` 之后 + 词边界；名单取自包内 29 个
`\tikzpeople@declareshape` 调用；裸词会误中散文与节点文本故不取子串），注入 `\usepackage{tikzpeople}`；
TL2026 与 2021 bundle 均自带 v0.4，其 `duck` shape 与 tikzducks 的鸭子无冲突（`tikzpeople_tikzducks_combo` 双引擎样例固化）。demo 专属
命令（`\alltikzpeople` / `\tikzpeoplecolors`，需 `[demo]` 选项）非生产用途，暂不探测。
`\fcBell` / `\fcStar[scale=0.5]` 等 figchild 图形命令由 `figchildRe` 命中（`\fc`+大写 556 个，另 5 个小写例外 `\fcfrog` / `\fchamburger` / `\fcpink` / `\fcsheetA` / `\fcsheetB`；`[A-Z]` 起点避开内核 `\fcolorbox`），注入 `\usepackage{figchild}`；命令宏各自自带完整 tikzpicture，故 `tikzSelfContainedCmdRes`（`selfContainedCmd`）把它们列为自包含命令——`normalizeTikz` 不包外壳，`stripOuterTikzShells` 也会剥掉误包外壳（嵌套 picture 会字体钩子递归/包围盒异常）。自包含命令判定前先经 `stripTikzComments` 剥离注释（未转义 `%` 至行尾，`\%` 视为转义），因此注释里的命令不算数、仍补外壳；混合了自包含命令与裸 tikz 的内容无法自动修好，保持原样（包一层会嵌套 picture）——这是固有取舍，`tikzSelfContainedEnvs` 的环境检查不做注释剥离。TL2026 v3.1.1 直接可用；tectonic bundle 的 v1.1.1 是 3 必填参数老 API（裸 `\fcBell` 报 Runaway argument），由`legacyFigchild` 在编译前把 vendored `figchild.sty`（`tikzassets/`，`//go:embed tikzassets/*.sty`）写入工作目录覆盖之。
`\tkztriminos[keys]<tikz options>{a § b § c}`（tikz-triminos 三连块拼图）由 `tikzTriminosRe`（`\b` 排除内部变量 `\tkztriminosize`）命中，注入 `\usepackage{tikz-triminos}`；命令自带 tikzpicture，同属自包含命令。tkz-euclide 探测已从子串 `\tkz` 收紧为 `tkzEuclideRe`（`\tkz`+大写；用户命令族全是 `\tkzDefPoint` / `\tkzDrawPoints` 这类 CamelCase，小写 `\tkzfrom` / `\tkzto` 只是内部临时宏），否则 `\tkztriminos` 会误加载 tkz-euclide——该前置检查放在两张探测表之前，保持 tkz-euclide 仍在包列表最前；`fixTkzPercentJoins` 的布防同步收紧为 `\tkz`+大写，`\tkztriminos` 参数里的 `%` 续行不再被改写。tectonic bundle 无该包，由 `legacyTriminos` 写 vendored `tikz-triminos.sty`，并注入 `triminosFpevalShim`——bundle 的 LaTeX 2021 缺 `\fpeval`（l3kernel 2021-05 才加入），shim 用 `\ifcsname` 守卫后把 `\fpeval` 别名到 expl3 既有的 `\fp_eval:n`。
`\scsnowman[keys]` 及 `\scsnowmandefault` / `\scsnowmannumeral` / `\makeitemsnowman` / `\makeqedsnowman` / `\makedocumentsnowman` / `\usescsnowmanlibrary` / `\enumsnowman` / `\makeqedother` / `\makeitemother` 由 `scsnowmanRe` 命中（`[^@A-Za-z0-9]` 边界排除内部命名空间 `\scsnowman@...`；`\enumsnowman` 等玩笑命令写成带反斜杠的命令，`\pagenumbering{enumsnowman}` 另行显式匹配），注入 `\usepackage{scsnowman}`；命令生成 inline 图形盒，同属自包含命令。命令名是 `\scsnowman` 而非 `\snowman`，与 tikzlings-snowmen 的 `\snowman` 无同名冲突。TL2026 v1.3c 与 bundle v1.2d 键集完全一致，两后端同一路径，无需 vendoring 或 shim。
tikzlings 的 pic 语法（`pic{bear}` / `pic[coati/body=blue, scale=0.5]{coati}` /
`pic[thing/hat=red]{penguin}`）由 `tikzPicRe` / `detectTikzlingsPics` 单独探测：`/tikz/pics/<name>`
键只由 TikZ 库文件（`tikzlibrarytikzlings.code.tex`）定义，动物子宏包不带 pic 定义——漏检时
pgfkeys 报 "I do not know the key '/tikz/pics/bear'"；legacy 后端要加载的动物子宏包由
`tikzlingsPicPackages` 提供。
手册示例常用的 `tcblisting` 盒子由 `tcblistingSetup` 注入高亮引擎库 + `\tcbset{tikz lower,
sidebyside, center lower, righthand width=5.7cm, sidebyside gap=10pt, lower separated=false,
listing engine=listings}`：库列表按"后端能力 **且** 内容命中"拼接——只有 latexmk 后端
（`tikzProfile.supportsMinted`）且片段提到 minted 时才拼 `\tcbuselibrary{listings,minted}`，
其余组合都只拼 `\tcbuselibrary{listings}`。两个条件缺一不可：能力决定"能不能加载"，
内容决定"要不要加载"——字节里不含 minted 字样的文档因此保持与改动前一致，不会平白依赖
minted/latexminted。minted 与 listings 都**不需要** `-shell-escape`，但途径不同：listings 是
纯 TeX 引擎；TL2026 的 minted v3 把高亮交给 `latexminted` 助手，而 `latexminted` 已在 texmf.cnf
的受限白名单里（`shell_escape = p` + `shell_escape_commands`，`latexmkCompileArgs` 绝不加
`-shell-escape` 的规则不受影响）。tectonic 的 shell escape 被完全禁用，加载 minted 会在
**导言区**就报 "You must invoke LaTeX with the -shell-escape flag"，且拖垮文档里每一个
tcblisting，故 tectonic profile 不加载 minted（此时 `listing engine=minted` 退化为 pgfkeys
"Choice 'minted' unknown in choice key '/tcb/listing engine'"，属已知限制；
`TestRenderTikzTcblistingMintedNotBlank` 只设 xe 腿，分档契约由
`TestTcblistingSetupMintedGatedByProfile` 单测钉住）。`\tcbset` 末项
`listing engine=listings` 钉住默认引擎（minted 库一加载，tcolorbox 默认引擎可能跟着变），
片段的盒子实例选项晚于导言区执行、总是优先。tcblisting 的 text 部分默认在 tikzpicture 之外，而 TikZ 只在 picture
内安装 `\path` / `\draw` / `scope`，不注入就报 "Environment scope undefined"；sidebyside
系列选项复刻手册盒内的左右布局（代码在左、编译结果在右并居中、无虚线分隔，取值来自
tikzducks-doc-settings.sty / tikzlings-doc-settings.sty 的 \tcbset）；注入的只是默认值，
片段自己的 `\tcbset` / 盒子实例选项（如 `righthand width=4cm`）在其后的正文中执行，总是
优先。`tikzSelfContainedEnvs` 同时收录 `tcblisting`，避免
再套外层 tikzpicture（套了会被 pgf 包围盒裁切）。盒子的**内侧**还有第二层包裹需要处理：
`tikz lower` 会给盒子正文再套一个 tikzpicture（tcolorbox.sty: `tikz lower/.style={before
lower*={\centering\tcb@shield@externalize\begin{tikzpicture}[{#1}]},after lower*=
\end{tikzpicture}}`），正文本身自包含时嵌套 picture 会**静默丢失**内容（编译 exit 0、
盒子右侧空白）——`rewriteSelfContainedTcblistings`（normalizeTikz 入口处调用）用
`selfContainedCmd` / `selfContainedStart` 判定正文（先经 `stripTikzComments`），命中时把
`tcblistingNoWrapStyle`（"slingshot nowrap"，与 `tcblistingSetup` 共用常量，该样式覆盖
`before lower*` / `after lower*`、只留 `\centering`）插到盒子选项**最前面**：pgfkeys 后写者胜，
用户后写的 `tikz lower` / `before lower*` 仍优先。只改选项参数、正文一字不动（要在代码侧原样
显示）；选项参数用 `matchBalancedBrace` 做平衡扫描（支持嵌套与 `\{` `\}` 转义）；已含样式名
则跳过（幂等），无选项参数 / 找不到 `\end{tcblisting}` 时原样跳过不报错；定位 begin/end 标记与
扫描选项花括号时都跳过注释区（未转义 `%` 至行尾），注释里的伪环境标记与花括号不参与判定；幂等
守卫按 pgfkeys 逗号分隔逐项全等比较（`hasNoWrapStyle`），值里出现同名文字不算引用。自包含命令
与裸 TikZ **混用**时按 contains 语义命中、同样摘包裹，其中的裸 TikZ 会因缺 picture 编译报错——
与 standalone 片段同一取舍，`README.md` 的 tikz 小节同步说明。**注释族布局**是 nowrap 的第二个
触发条件，名单 `tcblistingCommentFamilyStyles` 是**防御性超集**，按实测分三类：
（1）**承载修复**：`listing and comment`（含别名 `listing side comment` =
`sidebyside, listing and comment`，tcblistingscore.code.tex:224）把散文注释
（`comment={...}`）放进盒子 lower 槽，tcolorbox 为 sbs 排版 lower 时用
`\sbox{\tcb@lowerbox}{...\tcb@insert@before@lower <注释> \tcb@insert@after@lower}`
（tcolorbox.sty:1301），于是注释被 `before lower*` 的 `\centering` + `\begin{tikzpicture}`
包住；注释里的 `\\`（用户合法用法 = 换行）被 `\centering` 重定义为 `\@centercr`
（latex.ltx:15402）→ `\@xcentercr` → `\addvspace{ -\parskip}`（latex.ltx:15404），而
`\sbox` 内是 restricted horizontal mode，`\par` 无法结束段落 → `\addvspace` 的
`\ifhmode\ifinner` 守卫触发 `\@LRmoderr`（latex.ltx:9321），报 "! LaTeX Error: Not allowed
in LR mode." 于 `\end{tcblisting}`（base = 用户片段 + 全默认注入、注释含 `\\`、无 nowrap
时复现；加 nowrap 后 OK）。
（2）**防御性**：`comment and listing` / `comment side listing` / `comment only` 在默认值下
本就能编译（无 nowrap 亦 OK），picture 包裹对它们无意义，统一摘除以防将来重排/默认值变化漏判。
（3）**不在本修复范围**：`comment above* listing` / `listing above* comment` 走
`listing@process@outside` 路径，默认注入下另有独立失败（`Missing number` /
`Missing \endgroup`），加 nowrap 后**仍失败**；收录只为与其它注释族布局保持一致，不声称覆盖。
判定复用同一套 pgfkeys 扫描：对选项的**顶层逗号分隔条目**（splitPgfKeysOptions 已跳过未转义
`%` 至行尾的行内注释）剥掉等价外层花括号后与名单**全等**比较（isCommentFamilyTcblisting /
tcblistingCommentFamilyStyles），因此 `title={listing side comment}` 这类值内文字与注释里的
同名文字都不算引用；名单含父样式与 sidebyside 别名，**不含** `listing side text` /
`text side listing` / `text only` / `listing only` / 裸 `comment={...}`（默认布局 =
`listing and text`）——它们的 tikz lower 行为不变。插入位置、幂等（hasNoWrapStyle）、无选项
参数 / 找不到 `\end{tcblisting}` 时原样跳过等行为与自包含分支完全一致，两个触发条件共用同一次
注入。注释族盒子摘除 picture 后，`comment={...}`
里若写可执行 TikZ（如 `\draw`）将没有 picture 可跑——注释定位为散文，不支持是预期行为。
两个后端都支持（2021 bundle 自带
tcolorbox + listings）。

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

AMS 符号：导言区固定加载 `amsmath` + `amssymb`（`tikzWrapper` 模板固定部分，与四个 %s 注入槽无关）。
`\ulcorner` / `\urcorner` / `\llcorner` / `\lrcorner` / `\varnothing` / `\checkmark` 等
AMS 符号由 amsfonts 提供、amssymb 依赖并加载它，片段里可直接使用。典型场景是 tikz-cd 的
pullback corner 写法：`\begin{tikzcd}` 里 `\arrow[dr, phantom, "\ulcorner"]`——tikz-cd 标签
默认数学模式（`\iftikzcd@mathmode`），缺 amssymb 会在 `\end{tikzcd}` 报 "Undefined control
sequence"。这里**不做内容探测**：符号族太大，且子串会误中 `\node` 文本里的同名文字，故与
amsmath 同级常驻。TL2026 与 tectonic 2021 bundle 均自带 amssymb，两后端同一路径。片段里
显式写 `\usepackage{amssymb}` 会被提升到导言区，此时重复加载是 no-op（LaTeX 的 `\ver@` 去重）。

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
