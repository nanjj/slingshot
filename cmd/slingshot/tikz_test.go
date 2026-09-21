package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestNormalizeTikz(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "full environment passed through",
			input: "\\begin{tikzpicture}\n\\draw (0,0);\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\draw (0,0);\n\\end{tikzpicture}\n",
		},
		{
			name:  "bare content wrapped",
			input: "\\draw (0,0) -- (1,1);",
			want:  "\\begin{tikzpicture}\n\\draw (0,0) -- (1,1);\n\\end{tikzpicture}\n",
		},
		{
			name:  "usetikzlibrary hoisted out of wrapper",
			input: "\\usetikzlibrary{arrows.meta}\n\\draw[->>] (0,0) -- (1,1);",
			want:  "\\usetikzlibrary{arrows.meta}\n\\begin{tikzpicture}\n\\draw[->>] (0,0) -- (1,1);\n\\end{tikzpicture}\n",
		},
		{
			name:  "multiple usetikzlibrary lines hoisted",
			input: "\\usetikzlibrary{arrows.meta}\n\\usetikzlibrary{calc}\n\\node at (0,0) {a};",
			want:  "\\usetikzlibrary{arrows.meta}\n\\usetikzlibrary{calc}\n\\begin{tikzpicture}\n\\node at (0,0) {a};\n\\end{tikzpicture}\n",
		},
		{
			name:  "circuitikz environment passed through, not nested",
			input: "\\begin{circuitikz}\n\\draw (0,0) to[V=1V] (0,2);\n\\end{circuitikz}\n",
			want:  "\\begin{circuitikz}\n\\draw (0,0) to[V=1V] (0,2);\n\\end{circuitikz}\n",
		},
		{
			name:  "tikzcd environment passed through, not nested",
			input: "\\begin{tikzcd}\nA \\arrow[r] & B\n\\end{tikzcd}\n",
			want:  "\\begin{tikzcd}\nA \\arrow[r] & B\n\\end{tikzcd}\n",
		},
		{
			name:  "forest environment passed through, not nested",
			input: "\\begin{forest}\n[A [B]]\n\\end{forest}\n",
			want:  "\\begin{forest}\n[A [B]]\n\\end{forest}\n",
		},
		{
			name:  "tcblisting passed through, not wrapped in tikzpicture",
			input: "\\begin{tcblisting}{title={Basic}}\n\\marmot\n\\end{tcblisting}\n",
			want:  "\\begin{tcblisting}{title={Basic}}\n\\marmot\n\\end{tcblisting}\n",
		},
		{
			name:  "snippet tcbset before tcblisting kept, not wrapped",
			input: "\\tcbset{righthand width=3cm}\n\\begin{tcblisting}{title={Basic}}\n\\duck\n\\end{tcblisting}\n",
			want:  "\\tcbset{righthand width=3cm}\n\\begin{tcblisting}{title={Basic}}\n\\duck\n\\end{tcblisting}\n",
		},
		{
			name:  "axis still wrapped (needs tikzpicture)",
			input: "\\begin{axis}\\addplot {x};\n\\end{axis}",
			want:  "\\begin{tikzpicture}\n\\begin{axis}\\addplot {x};\n\\end{axis}\n\\end{tikzpicture}\n",
		},
		{
			name:  "tkzpicture normalized to tikzpicture",
			input: "\\begin{tkzpicture}\n\\tkzDefPoint(0,0){A}\n\\end{tkzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\tkzDefPoint(0,0){A}\n\\end{tikzpicture}\n",
		},
		{
			name:  "tkzpicture with options normalized",
			input: "\\begin{tkzpicture}[scale=2]\n\\tkzDefPoint(0,0){A}\n\\end{tkzpicture}\n",
			want:  "\\begin{tikzpicture}[scale=2]\n\\tkzDefPoint(0,0){A}\n\\end{tikzpicture}\n",
		},
		{
			name:  "outer tikzpicture shell around tkzpicture stripped",
			input: "\\begin{tikzpicture}\n\\begin{tkzpicture}\n\\tkzDefPoint(0,0){A}\n\\end{tkzpicture}\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\tkzDefPoint(0,0){A}\n\\end{tikzpicture}",
		},
		{
			name:  "veclen scope option normalized to xfp",
			input: "\\begin{tikzpicture}\n\\begin{scope}[veclen]\n\\draw (0,0);\n\\end{scope}\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\begin{scope}[xfp]\n\\draw (0,0);\n\\end{scope}\n\\end{tikzpicture}\n",
		},
		{
			name:  "veclen first among other options",
			input: "\\begin{tikzpicture}\n\\begin{scope}[veclen, x=1]\n\\end{scope}\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\begin{scope}[xfp, x=1]\n\\end{scope}\n\\end{tikzpicture}\n",
		},
		{
			name:  "veclen later option with space",
			input: "\\begin{tikzpicture}\n\\begin{scope}[a, veclen]\n\\end{scope}\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\begin{scope}[a, xfp]\n\\end{scope}\n\\end{tikzpicture}\n",
		},
		{
			name:  "veclen with value and node",
			input: "\\begin{tikzpicture}\n\\node[veclen=true]{x};\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\node[xfp=true]{x};\n\\end{tikzpicture}\n",
		},
		{
			name:  "veclen pgfmath function call untouched",
			input: "\\begin{tikzpicture}\n\\node at ($ (0,0) ! veclen(1,1) ! (1,0) $) {};\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\node at ($ (0,0) ! veclen(1,1) ! (1,0) $) {};\n\\end{tikzpicture}\n",
		},
		{
			name:  "tkz-euclide doc example with scope veclen",
			input: "\\begin{tikzpicture}[scale=1]\n\\tkzDefPoint(0,0){O}\n\\tkzDefPoint(2.5,0){N}\n\\begin{scope}[veclen]\n\\tkzMarkAngle[mkpos=.2, size=1.2](C,A,M)\n\\end{scope}\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}[scale=1]\n\\tkzDefPoint(0,0){O}\n\\tkzDefPoint(2.5,0){N}\n\\begin{scope}[xfp]\n\\tkzMarkAngle[mkpos=.2, size=1.2](C,A,M)\n\\end{scope}\n\\end{tikzpicture}\n",
		},
		{
			name:  "new style injected for tkz-euclide doc example",
			input: "\\begin{tikzpicture}\n\\tkzDrawSegment[new](I,C)\n\\end{tikzpicture}\n",
			want:  "\\tikzset{new/.style={color=orange,line width=.2pt}}\n\\begin{tikzpicture}\n\\tkzDrawSegment[new](I,C)\n\\end{tikzpicture}\n",
		},
		{
			name:  "new later among other options",
			input: "\\begin{tikzpicture}\n\\tkzDrawCircle[draw=red, new](I,J)\n\\end{tikzpicture}\n",
			want:  "\\tikzset{new/.style={color=orange,line width=.2pt}}\n\\begin{tikzpicture}\n\\tkzDrawCircle[draw=red, new](I,J)\n\\end{tikzpicture}\n",
		},
		{
			name:  "new not injected when user defines it via tikzset",
			input: "\\begin{tikzpicture}\n\\tikzset{new/.style={color=blue}}\n\\tkzDrawSegment[new](I,C)\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\tikzset{new/.style={color=blue}}\n\\tkzDrawSegment[new](I,C)\n\\end{tikzpicture}\n",
		},
		{
			name:  "new not injected when user defines it via tkzSetUpStyle",
			input: "\\tkzSetUpStyle[color=blue]{new}\n\\tkzDrawPoints[new](A,B)\n",
			want:  "\\begin{tikzpicture}\n\\tkzSetUpStyle[color=blue]{new}\n\\tkzDrawPoints[new](A,B)\n\n\\end{tikzpicture}\n",
		},
		{
			name:  "new style injected for bare content",
			input: "\\tkzDrawSegment[new](I,C)",
			want:  "\\begin{tikzpicture}\n\\tikzset{new/.style={color=orange,line width=.2pt}}\n\\tkzDrawSegment[new](I,C)\n\\end{tikzpicture}\n",
		},
		{
			name:  "new not injected for longer key names",
			input: "\\begin{tikzpicture}\n\\node[newwidth]{x};\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\node[newwidth]{x};\n\\end{tikzpicture}\n",
		},
		{
			name:  "veclen translated and new injected independently",
			input: "\\begin{tikzpicture}\n\\begin{scope}[veclen, new]\n\\draw (0,0);\n\\end{scope}\n\\end{tikzpicture}\n",
			want:  "\\tikzset{new/.style={color=orange,line width=.2pt}}\n\\begin{tikzpicture}\n\\begin{scope}[xfp, new]\n\\draw (0,0);\n\\end{scope}\n\\end{tikzpicture}\n",
		},
		{
			name:  "percent line-join col-0 continuation fixed",
			input: "\\begin{tikzpicture}\n\\tkzDrawSegments[new](P,R M,P M,Q M,R N,P'%\nN,Q' N,R' P',R' I,K)\n\\end{tikzpicture}\n",
			want:  "\\tikzset{new/.style={color=orange,line width=.2pt}}\n\\begin{tikzpicture}\n\\tkzDrawSegments[new](P,R M,P M,Q M,R N,P'\nN,Q' N,R' P',R' I,K)\n\\end{tikzpicture}\n",
		},
		{
			name:  "percent line-join indented continuation also fixed",
			input: "\\begin{tikzpicture}\n\\tkzDrawSegments[new](P,R N,P'%\n   N,Q')\n\\end{tikzpicture}\n",
			want:  "\\tikzset{new/.style={color=orange,line width=.2pt}}\n\\begin{tikzpicture}\n\\tkzDrawSegments[new](P,R N,P'\n   N,Q')\n\\end{tikzpicture}\n",
		},
		{
			name:  "percent line-join in tkz brace list fixed",
			input: "\\begin{tikzpicture}\n\\tkzDefPointsBy[reflection=over A--B](M,N){P,P'%\nQ,Q'}\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\tkzDefPointsBy[reflection=over A--B](M,N){P,P'\nQ,Q'}\n\\end{tikzpicture}\n",
		},
		{
			name:  "percent line-join in manual MarkRightAngles example fixed",
			input: "\\begin{tikzpicture}\n\\tkzMarkRightAngles(A,F,I B,D,I J_c,X_c,A%\n     J_c,Y_c,B)\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\tkzMarkRightAngles(A,F,I B,D,I J_c,X_c,A\n     J_c,Y_c,B)\n\\end{tikzpicture}\n",
		},
		{
			name:  "node text percent join untouched",
			input: "\\begin{tikzpicture}\n\\node {foo%\nbar};\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\node {foo%\nbar};\n\\end{tikzpicture}\n",
		},
		{
			name:  "comma before percent untouched",
			input: "\\begin{tikzpicture}\n\\tkzDrawPoints(A,B,%\nC,D)\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\tkzDrawPoints(A,B,%\nC,D)\n\\end{tikzpicture}\n",
		},
		{
			name:  "number before percent with comma continuation untouched",
			input: "\\begin{tikzpicture}\n\\tkzFillAngle[fill=teal,opacity=.2%\n,fill=blue](A,B,C)\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\tkzFillAngle[fill=teal,opacity=.2%\n,fill=blue](A,B,C)\n\\end{tikzpicture}\n",
		},
		{
			name:  "comment containing tkz command untouched",
			input: "\\begin{tikzpicture}\n% \\tkzDrawPoints(A,B%\nC,D)\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n% \\tkzDrawPoints(A,B%\nC,D)\n\\end{tikzpicture}\n",
		},
		{
			name:  "mid-line comment with indented continuation untouched",
			input: "\\begin{tikzpicture}\n\\tkzDrawSegments(A,B% note\n   C,D)\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\tkzDrawSegments(A,B% note\n   C,D)\n\\end{tikzpicture}\n",
		},
		{
			name:  "double outer shells stripped iteratively",
			input: "\\begin{tikzpicture}\n\\begin{tikzpicture}\n\\begin{circuitikz}\n\\draw (0,0) to[R] (2,0);\n\\end{circuitikz}\n\\end{tikzpicture}\n\\end{tikzpicture}\n",
			want:  "\\begin{circuitikz}\n\\draw (0,0) to[R] (2,0);\n\\end{circuitikz}",
		},
		{
			name:  "node embedded picture not stripped",
			input: "\\begin{tikzpicture}\n\\node {\\begin{tikzpicture}\\draw (0,0);\\end{tikzpicture}};\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\node {\\begin{tikzpicture}\\draw (0,0);\\end{tikzpicture}};\n\\end{tikzpicture}\n",
		},
		{
			name:  "scope not mistaken for self-contained env",
			input: "\\begin{tikzpicture}\n\\begin{scope}\n\\draw (0,0);\n\\end{scope}\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\begin{scope}\n\\draw (0,0);\n\\end{scope}\n\\end{tikzpicture}\n",
		},
		{
			name:  "tkzInterLC next to first endpoint translated to near",
			input: "\\begin{tikzpicture}\n\\tkzInterLC[next to=Ja](Ja,Q)(Q,Cb) \\tkzGetFirstPoint{F'a}\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\tkzInterLC[near](Ja,Q)(Q,Cb) \\tkzGetFirstPoint{F'a}\n\\end{tikzpicture}\n",
		},
		{
			name:  "tkzInterLC next to second endpoint swaps line and uses near",
			input: "\\begin{tikzpicture}\n\\tkzInterLC[next to=Q](Ja,Q)(Q,Cb) \\tkzGetFirstPoint{X}\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\tkzInterLC[near](Q,Ja)(Q,Cb) \\tkzGetFirstPoint{X}\n\\end{tikzpicture}\n",
		},
		{
			name:  "tkzInterCC next to untouched (no near in 4.051b circlecircle)",
			input: "\\begin{tikzpicture}\n\\tkzInterCC[next to=C](A,B)(C,D) \\tkzGetFirstPoint{X}\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\tkzInterCC[next to=C](A,B)(C,D) \\tkzGetFirstPoint{X}\n\\end{tikzpicture}\n",
		},
		{
			name:  "next to and common coexist, each kept semantically",
			input: "\\begin{tikzpicture}\n\\tkzInterLC[next to=Ja](Ja,Q)(Q,Cb) \\tkzGetFirstPoint{F'a}\n\\tkzInterLC[common=F'a](Sp,F'a)(Ja,F'a) \\tkzGetFirstPoint{Fa}\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\tkzInterLC[near](Ja,Q)(Q,Cb) \\tkzGetFirstPoint{F'a}\n\\tkzInterLC[common=F'a](Sp,F'a)(Ja,F'a) \\tkzGetFirstPoint{Fa}\n\\end{tikzpicture}\n",
		},
		{
			name:  "next to with spaces around key and value",
			input: "\\begin{tikzpicture}\n\\tkzInterLC [next to = Ja](Ja,Q)(Q,Cb)\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\tkzInterLC[near](Ja,Q)(Q,Cb)\n\\end{tikzpicture}\n",
		},
		{
			name:  "next to not a line endpoint untouched",
			input: "\\begin{tikzpicture}\n\\tkzInterLC[next to=O](Ja,Q)(Q,Cb) \\tkzGetFirstPoint{X}\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\tkzInterLC[next to=O](Ja,Q)(Q,Cb) \\tkzGetFirstPoint{X}\n\\end{tikzpicture}\n",
		},
		{
			name:  "next to in node text untouched",
			input: "\\begin{tikzpicture}\n\\node {next to the point};\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\node {next to the point};\n\\end{tikzpicture}\n",
		},
		{
			name:  "def point on circle through reordered (apollonius doc example)",
			input: "\\begin{tikzpicture}\n\\tkzDefCircle[apollonius,K=2](A,B) \\tkzGetPoints{K1}{k}\n\\tkzDefPointOnCircle[through= center K1 angle 30 point k] \\tkzGetPoint{I}\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\tkzDefCircle[apollonius,K=2](A,B) \\tkzGetPoints{K1}{k}\n\\tkzDefPointOnCircle[through= angle 30 center K1 point k] \\tkzGetPoint{I}\n\\end{tikzpicture}\n",
		},
		{
			name:  "def point on circle 4.051b order kept",
			input: "\\begin{tikzpicture}\n\\tkzDefPointOnCircle[through= angle 30 center K1 point k] \\tkzGetPoint{I}\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\tkzDefPointOnCircle[through= angle 30 center K1 point k] \\tkzGetPoint{I}\n\\end{tikzpicture}\n",
		},
		{
			// figchild 的 \fc* 命令自带 tikzpicture, 不能再包外壳。
			name:  "figchild command is self-contained, not wrapped",
			input: "\\fcBell",
			want:  "\\fcBell",
		},
		{
			// tikz-triminos 的 \tkztriminos 自带 tikzpicture, 不能再包外壳。
			name:  "tkztriminos command is self-contained, not wrapped",
			input: "\\tkztriminos{value 1 § value 2 § value 3}",
			want:  "\\tkztriminos{value 1 § value 2 § value 3}",
		},
		{
			// \scsnowman 家族生成 inline 图形盒, 不能再包外壳。
			name:  "scsnowman command is self-contained, not wrapped",
			input: "\\scsnowman[scale=2,hat=red]",
			want:  "\\scsnowman[scale=2,hat=red]",
		},
		{
			// 误包在 tikzpicture 里的 figchild 命令: 内层是自包含命令, 剥壳。
			name:  "outer tikzpicture shell around figchild stripped",
			input: "\\begin{tikzpicture}\n\\fcBell\n\\end{tikzpicture}\n",
			want:  "\\fcBell",
		},
		{
			// 收紧 fixTkzPercentJoins 布防 (\tkz+大写) 后, tikz-triminos 参数内
			// 的 % 续行必须原样保留 (旧代码会把 % 删掉)。
			name:  "tkztriminos percent continuation untouched",
			input: "\\tkztriminos{a § b%\n c}",
			want:  "\\tkztriminos{a § b%\n c}",
		},
		{
			// 注释里的自包含命令不算数: 剥离注释后仍应补外壳。
			name:  "commented-out figchild command still wrapped",
			input: "% \\fcBell renders nicely\n\\draw (0,0) -- (1,1);",
			want:  "\\begin{tikzpicture}\n% \\fcBell renders nicely\n\\draw (0,0) -- (1,1);\n\\end{tikzpicture}\n",
		},
		{
			// 固有取舍: 混合了自包含命令与裸 tikz 的内容无法自动修好
			// (包一层会嵌套 picture), 保持原样。
			name:  "mixed self-contained command and raw tikz stays unwrapped",
			input: "\\fcBell\n\\draw (0,0) -- (1,1);",
			want:  "\\fcBell\n\\draw (0,0) -- (1,1);",
		},
		{
			// 外层壳里内层以注释行开头, 随后才是真实 figchild 命令:
			// selfContainedCmdStart 先剥注释再判定, 因此仍能识别并剥壳。
			name:  "shell around commented+real figchild stripped",
			input: "\\begin{tikzpicture}\n% \\fcBell\n\\fcBell\n\\end{tikzpicture}\n",
			want:  "% \\fcBell\n\\fcBell",
		},
		{
			// 外层壳里内层以注释行开头, 随后才是真实 circuitikz 环境:
			// selfContainedStart 同样先剥注释再判定, 因此仍能识别并剥壳。
			name:  "shell around commented+real circuitikz env stripped",
			input: "\\begin{tikzpicture}\n% my circuit\n\\begin{circuitikz}\n\\draw (0,0) to[R] (2,0);\n\\end{circuitikz}\n\\end{tikzpicture}\n",
			want:  "% my circuit\n\\begin{circuitikz}\n\\draw (0,0) to[R] (2,0);\n\\end{circuitikz}",
		},
		{
			// picture 是 LaTeX 内核环境, 不是 TikZ 环境; 套进裸 tikzpicture 会被 pgf
			// 当空盒子, 内容全丢 (issue #2)。前缀识别后不再补外壳。
			name:  "picture env passed through, not wrapped",
			input: "\\begin{picture}(42,44)\n  \\picduck\n\\end{picture}\n",
			want:  "\\begin{picture}(42,44)\n  \\picduck\n\\end{picture}\n",
		},
		{
			// 显式 \usetikzlibrary 行即使位于自包含片段里也要提到环境外, 否则库文件
			// 内部的 \usepackage 留在正文 (tikzducks 库文件首行即 \usepackage)。
			name:  "usetikzlibrary hoisted with self-contained env",
			input: "\\usetikzlibrary{ducks}\n\\begin{tikzpicture}\n\\draw (0,0) pic{duck};\n\\end{tikzpicture}\n",
			want:  "\\usetikzlibrary{ducks}\n\\begin{tikzpicture}\n\\draw (0,0) pic{duck};\n\\end{tikzpicture}\n",
		},
		{
			// tikzducks 手册的 picture 示例带 \setlength{\unitlength}{...} 前导
			// 语句, \begin{picture} 不在首位; 同样不能补外壳。
			name:  "picture with setlength prologue passed through",
			input: "\\setlength{\\unitlength}{1mm}\n\\begin{picture}(42,44)\n  \\picduck\n\\end{picture}\n",
			want:  "\\setlength{\\unitlength}{1mm}\n\\begin{picture}(42,44)\n  \\picduck\n\\end{picture}\n",
		},
		{
			// picture 嵌在 \node 里是合法结构 (issue #2 的 workaround): 片段仍需要外层
			// tikzpicture, 不能因为"提到 picture"就跳过包裹。
			name:  "picture inside node still wrapped",
			input: "\\node[inner sep=0] {\\begin{picture}(42,44)\\picduck\\end{picture}};",
			want:  "\\begin{tikzpicture}\n\\node[inner sep=0] {\\begin{picture}(42,44)\\picduck\\end{picture}};\n\\end{tikzpicture}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeTikz(tt.input, tectonicProfile()); got != tt.want {
				t.Errorf("normalizeTikz() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestNormalizeTikzLatexmkProfile 是 latexmk profile 的对照用例:
// legacy 翻译 (tkz-euclide 5.x → 4.051b) 在 TL2026 下必须原样保留,
// 否则会"反向出错"。与 TestNormalizeTikz 的 tectonic profile 行为一一对照。
func TestNormalizeTikzLatexmkProfile(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "veclen key preserved for TL2026",
			input: "\\begin{tikzpicture}\n\\begin{scope}[veclen]\n\\draw (0,0);\n\\end{scope}\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\begin{scope}[veclen]\n\\draw (0,0);\n\\end{scope}\n\\end{tikzpicture}\n",
		},
		{
			name:  "through center..angle..point order preserved",
			input: "\\begin{tikzpicture}\n\\tkzDefPointOnCircle[through= center K1 angle 30 point k] \\tkzGetPoint{I}\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\tkzDefPointOnCircle[through= center K1 angle 30 point k] \\tkzGetPoint{I}\n\\end{tikzpicture}\n",
		},
		{
			name:  "def circle R preserved",
			input: "\\begin{tikzpicture}\n\\tkzDefCircle[R](A,1) \\tkzGetPoint{a}\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\tkzDefCircle[R](A,1) \\tkzGetPoint{a}\n\\end{tikzpicture}\n",
		},
		{
			name:  "next to preserved",
			input: "\\begin{tikzpicture}\n\\tkzInterLC[next to=A](A,B)(C,D) \\tkzGetFirstPoint{X}\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\tkzInterLC[next to=A](A,B)(C,D) \\tkzGetFirstPoint{X}\n\\end{tikzpicture}\n",
		},
		{
			name:  "tangent at preserved",
			input: "\\begin{tikzpicture}\n\\tkzDefLine[tangent at=T](B) \\tkzGetPoint{h}\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\tkzDefLine[tangent at=T](B) \\tkzGetPoint{h}\n\\end{tikzpicture}\n",
		},
		{
			name:  "apollonius preserved",
			input: "\\begin{tikzpicture}\n\\tkzDefCircle[apollonius,K=2](A,B) \\tkzGetPoints{K1}{k}\n\\end{tikzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\tkzDefCircle[apollonius,K=2](A,B) \\tkzGetPoints{K1}{k}\n\\end{tikzpicture}\n",
		},
		{
			name:  "tkzpicture still normalized (both backends)",
			input: "\\begin{tkzpicture}\n\\tkzDefPoint(0,0){A}\n\\end{tkzpicture}\n",
			want:  "\\begin{tikzpicture}\n\\tkzDefPoint(0,0){A}\n\\end{tikzpicture}\n",
		},
		{
			name:  "new style injected (both backends)",
			input: "\\begin{tikzpicture}\n\\tkzDrawSegment[new](I,C)\n\\end{tikzpicture}\n",
			want:  "\\tikzset{new/.style={color=orange,line width=.2pt}}\n\\begin{tikzpicture}\n\\tkzDrawSegment[new](I,C)\n\\end{tikzpicture}\n",
		},
		{
			// picture 的自包含判定与后端无关: 两个 profile 都不补外壳。
			name:  "picture env passed through, not wrapped",
			input: "\\begin{picture}(42,44)\n  \\picduck\n\\end{picture}\n",
			want:  "\\begin{picture}(42,44)\n  \\picduck\n\\end{picture}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeTikz(tt.input, latexmkProfile()); got != tt.want {
				t.Errorf("normalizeTikz(latexmk) = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFixNextToKeys(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "next to first endpoint becomes near",
			input: `\tkzInterLC[next to=Ja](Ja,Q)(Q,Cb)`,
			want:  `\tkzInterLC[near](Ja,Q)(Q,Cb)`,
		},
		{
			name:  "next to second endpoint swaps line",
			input: `\tkzInterLC[next to=Q](Ja,Q)(Q,Cb)`,
			want:  `\tkzInterLC[near](Q,Ja)(Q,Cb)`,
		},
		{
			name:  "spaces around key and equals",
			input: `\tkzInterLC [next to = Ja](Ja,Q)(Q,Cb)`,
			want:  `\tkzInterLC[near](Ja,Q)(Q,Cb)`,
		},
		{
			name:  "primed point names",
			input: `\tkzInterLC[next to=F'a](F'a,Jc)(Ja,F'a)`,
			want:  `\tkzInterLC[near](F'a,Jc)(Ja,F'a)`,
		},
		{
			name:  "inter CC untouched (no near in 4.051b circlecircle)",
			input: `\tkzInterCC[next to=C](A,B)(C,D)`,
			want:  `\tkzInterCC[next to=C](A,B)(C,D)`,
		},
		{
			name:  "next to other point untouched",
			input: `\tkzInterLC[next to=O](Ja,Q)(Q,Cb)`,
			want:  `\tkzInterLC[next to=O](Ja,Q)(Q,Cb)`,
		},
		{
			name:  "already near untouched",
			input: `\tkzInterLC[near](A,B)(C,D)`,
			want:  `\tkzInterLC[near](A,B)(C,D)`,
		},
		{
			name:  "already common untouched",
			input: `\tkzInterLC[common=F'a](Sp,F'a)(Ja,F'a)`,
			want:  `\tkzInterLC[common=F'a](Sp,F'a)(Ja,F'a)`,
		},
		{
			name:  "other options untouched",
			input: `\tkzInterLC[R](A,B)(C,2)`,
			want:  `\tkzInterLC[R](A,B)(C,2)`,
		},
		{
			name:  "internal LCR variant not matched by prefix",
			input: `\tkzInterLCR(A,B)(C,D){X}{Y}`,
			want:  `\tkzInterLCR(A,B)(C,D){X}{Y}`,
		},
		{
			name:  "no option bracket, next to stays",
			input: `\tkzInterLC (A,B)(C,D) next to=Ja`,
			want:  `\tkzInterLC (A,B)(C,D) next to=Ja`,
		},
		{
			name:  "plain text next to untouched",
			input: `\node {go next to the point};`,
			want:  `\node {go next to the point};`,
		},
		{
			name:  "inter LL untouched",
			input: `\tkzInterLL(Za,Xc)(C,B) \tkzGetPoint{C'}`,
			want:  `\tkzInterLL(Za,Xc)(C,B) \tkzGetPoint{C'}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fixNextToKeys(tt.input); got != tt.want {
				t.Errorf("fixNextToKeys() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFixDefPointOnCircleThrough(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "2x order reordered to 4.051b",
			input: `\tkzDefPointOnCircle[through= center K1 angle 30 point k]`,
			want:  `\tkzDefPointOnCircle[through= angle 30 center K1 point k]`,
		},
		{
			name:  "different angle value",
			input: `\tkzDefPointOnCircle[through= center K1 angle 280 point k]`,
			want:  `\tkzDefPointOnCircle[through= angle 280 center K1 point k]`,
		},
		{
			name:  "spaces around equals",
			input: `\tkzDefPointOnCircle [through = center K1 angle 30 point k]`,
			want:  `\tkzDefPointOnCircle [through= angle 30 center K1 point k]`,
		},
		{
			name:  "primed point name",
			input: `\tkzDefPointOnCircle[through= center O angle 90 point P']`,
			want:  `\tkzDefPointOnCircle[through= angle 90 center O point P']`,
		},
		{
			name:  "negative and decimal angles",
			input: `\tkzDefPointOnCircle[through= center O angle -30.5 point A]`,
			want:  `\tkzDefPointOnCircle[through= angle -30.5 center O point A]`,
		},
		{
			name:  "other options before through kept",
			input: `\tkzDefPointOnCircle[color=red, through= center O angle 30 point A]`,
			want:  `\tkzDefPointOnCircle[color=red, through= angle 30 center O point A]`,
		},
		{
			name:  "4.051b order untouched",
			input: `\tkzDefPointOnCircle[through= angle 30 center K1 point k]`,
			want:  `\tkzDefPointOnCircle[through= angle 30 center K1 point k]`,
		},
		{
			name:  "R variant untouched",
			input: `\tkzDefPointOnCircle[R= angle 30 center O radius 2]`,
			want:  `\tkzDefPointOnCircle[R= angle 30 center O radius 2]`,
		},
		{
			name:  "through in rad untouched (not in 4.051b)",
			input: `\tkzDefPointOnCircle[through in rad= center O angle 30 point A]`,
			want:  `\tkzDefPointOnCircle[through in rad= center O angle 30 point A]`,
		},
		{
			name:  "bare through boolean untouched",
			input: `\tkzDefPointOnCircle[through] \tkzGetPoint{I}`,
			want:  `\tkzDefPointOnCircle[through] \tkzGetPoint{I}`,
		},
		{
			name:  "def point on line untouched",
			input: `\tkzDefPointOnLine[through= center O angle 30 point A](X,Y)`,
			want:  `\tkzDefPointOnLine[through= center O angle 30 point A](X,Y)`,
		},
		{
			name:  "text mentioning through untouched",
			input: `\node {through= center O angle 30 point A};`,
			want:  `\node {through= center O angle 30 point A};`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fixDefPointOnCircleThrough(tt.input); got != tt.want {
				t.Errorf("fixDefPointOnCircleThrough() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFixDefCircleR(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "basic R circle def gets 5.x point-on-circle semantics",
			input: `\tkzDefCircle[R](A,1) \tkzGetPoint{a}`,
			want: `\tkzDefCircleR(A,1)
\path (A)--++(\tkzLengthResult,0) coordinate (tkzSecondPointResult);
\tkzRenamePoint(tkzSecondPointResult){tkzPointResult} \tkzGetPoint{a}`,
		},
		{
			name:  "spaces around command, key and args",
			input: `\tkzDefCircle [ R ](B, 3)\tkzGetPoint{b}`,
			want: `\tkzDefCircleR(B,3)
\path (B)--++(\tkzLengthResult,0) coordinate (tkzSecondPointResult);
\tkzRenamePoint(tkzSecondPointResult){tkzPointResult}\tkzGetPoint{b}`,
		},
		{
			name:  "decimal radius",
			input: `\tkzDefCircle[R](O,2.5) \tkzGetPoint{p}`,
			want: `\tkzDefCircleR(O,2.5)
\path (O)--++(\tkzLengthResult,0) coordinate (tkzSecondPointResult);
\tkzRenamePoint(tkzSecondPointResult){tkzPointResult} \tkzGetPoint{p}`,
		},
		{
			name:  "through option untouched",
			input: `\tkzDefCircle[through](A,B) \tkzGetPoint{a}`,
			want:  `\tkzDefCircle[through](A,B) \tkzGetPoint{a}`,
		},
		{
			name:  "radius option untouched (4.051b through alias)",
			input: `\tkzDefCircle[radius](A,B) \tkzGetPoint{a}`,
			want:  `\tkzDefCircle[radius](A,B) \tkzGetPoint{a}`,
		},
		{
			name:  "R with other keys untouched (no silent misinterpretation)",
			input: `\tkzDefCircle[R,K=2](A,B)`,
			want:  `\tkzDefCircle[R,K=2](A,B)`,
		},
		{
			name:  "inter CC R untouched (native in 4.051b)",
			input: `\tkzInterCC[R](A,1)(K,3) \tkzGetPoints{a}{a'}`,
			want:  `\tkzInterCC[R](A,1)(K,3) \tkzGetPoints{a}{a'}`,
		},
		{
			name:  "inter LC R untouched (native in 4.051b)",
			input: `\tkzInterLC[R](A,B)(B,3) \tkzGetPoints{b1}{E}`,
			want:  `\tkzInterLC[R](A,B)(B,3) \tkzGetPoints{b1}{E}`,
		},
		{
			name:  "draw circle R untouched (native in 4.051b)",
			input: `\tkzDrawCircle[R](O,2)`,
			want:  `\tkzDrawCircle[R](O,2)`,
		},
		{
			name:  "plain text mentioning R untouched",
			input: `% \tkzDefCircle[R](A,1) example in comment`,
			want:  `% \tkzDefCircle[R](A,1) example in comment`,
		},
		{
			name:  "inline comment usage untouched (translation would leak code)",
			input: `\node{x}; % \tkzDefCircle[R](A,1)`,
			want:  `\node{x}; % \tkzDefCircle[R](A,1)`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fixDefCircleR(tt.input); got != tt.want {
				t.Errorf("fixDefCircleR() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFixDefLineTangent(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "tangent at becomes direct 4.051b call with center-first order",
			input: `\tkzDefLine[tangent at=T](B) \tkzGetPoint{h}`,
			want:  `\tkzTgtAt(B)(T) \tkzGetPoint{h}`,
		},
		{
			name:  "prime point name",
			input: `\tkzDefLine[tangent at=T'](B) \tkzGetPoint{h}`,
			want:  `\tkzTgtAt(B)(T') \tkzGetPoint{h}`,
		},
		{
			name:  "spaces around command, key and args",
			input: `\tkzDefLine [ tangent at = T' ](B) \tkzGetPoint{h}`,
			want:  `\tkzTgtAt(B)(T') \tkzGetPoint{h}`,
		},
		{
			name:  "combined keys untouched (no silent misinterpretation)",
			input: `\tkzDefLine[tangent at=T,new](B)`,
			want:  `\tkzDefLine[tangent at=T,new](B)`,
		},
		{
			name:  "mediator untouched (out of scope, keeps explicit error)",
			input: `\tkzDefLine[mediator](A,B)`,
			want:  `\tkzDefLine[mediator](A,B)`,
		},
		{
			name:  "plain line def untouched",
			input: `\tkzDefLine(A,B) \tkzGetPoint{O}`,
			want:  `\tkzDefLine(A,B) \tkzGetPoint{O}`,
		},
		{
			name:  "internal macro name untouched (word boundary)",
			input: `\tkzDefLineLL(A,B)`,
			want:  `\tkzDefLineLL(A,B)`,
		},
		{
			name:  "comment-only usage untouched",
			input: `% \tkzDefLine[tangent at=T](B) example`,
			want:  `% \tkzDefLine[tangent at=T](B) example`,
		},
		{
			name:  "inline comment usage untouched",
			input: `\node{x}; % \tkzDefLine[tangent at=T](B)`,
			want:  `\node{x}; % \tkzDefLine[tangent at=T](B)`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fixDefLineTangent(tt.input); got != tt.want {
				t.Errorf("fixDefLineTangent() = %q, want %q", got, tt.want)
			}
		})
	}
}
func TestTkzApolloniusShim(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantShim bool
	}{
		{
			name:     "DefCircle apollonius",
			input:    `\tkzDefCircle[apollonius,K=2](A,B) \tkzGetPoints{K1}{k}`,
			wantShim: true,
		},
		{
			name:     "DrawCircle apollonius",
			input:    `\tkzDrawCircle[apollonius,K=3](A,B)`,
			wantShim: true,
		},
		{
			name:     "DrawCircles apollonius",
			input:    `\tkzDrawCircles[apollonius,K=2](A,B)`,
			wantShim: true,
		},
		{
			name:     "direct internal macro",
			input:    `\tkzDefApolloniusCircle(A,B)`,
			wantShim: true,
		},
		{
			name:     "comment mention does not trigger",
			input:    "% apollonius circle example\n\\tkzDefPoint(0,0){A}",
			wantShim: false,
		},
		{
			name:     "unrelated input",
			input:    `\tkzDefCircle[circum](A,B,C)`,
			wantShim: false,
		},
		{
			name:     "apollonius in node text does not trigger",
			input:    `\node {apollonius};`,
			wantShim: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tkzApolloniusShimFor(tt.input)
			if tt.wantShim && got == "" {
				t.Fatal("tkzApolloniusShimFor() = empty, want shim")
			}
			if !tt.wantShim && got != "" {
				t.Fatalf("tkzApolloniusShimFor() = %q, want empty", got)
			}
		})
	}
}

// TestTkzApolloniusShimSemantics 验证注入定义镜像 5.x 的结果点语义:
// First=圆心、Second=内分点、Third=外分点、Length=半径,
// 且圆心仍在 tkzPointResult (DrawCircle[apollonius] 分支兼容)。
func TestTkzApolloniusShimSemantics(t *testing.T) {
	shim := tkzApolloniusShimFor(`\tkzDefCircle[apollonius,K=2](A,B)`)
	checks := []struct {
		name string
		sub  string
	}{
		{"guarded", `\ifcsname tkzDefApolloniusCircle\endcsname`},
		{"first is center", `\pgfnodealias{tkzFirstPointResult}{tkzPointResult}`},
		{"second is inner point", `\pgfnodealias{tkzSecondPointResult}{tkzPointResult}`},
		{"third is outer point", `\pgfnodealias{tkzThirdPointResult}{apo@ptb}`},
		{"radius from center to inner", `\tkz@@CalcLengthcm(tkzFirstPointResult,apo@pta){tkzLengthResult}`},
		{"inner via VecK k/(1+k)", `\tkz@VecK[\tkz@koeff/(1+\tkz@koeff)](#1,#2)`},
		{"outer via VecK k/(k-1)", `\tkz@VecK[\tkz@koeff/(\tkz@koeff-1)](#1,#2)`},
		{"center stays in tkzPointResult", `\tkzDefMidPoint(apo@pta,apo@ptb)`},
	}
	for _, c := range checks {
		if !strings.Contains(shim, c.sub) {
			t.Errorf("shim missing %s: %q not found", c.name, c.sub)
		}
	}
	// First 的赋值必须在 Second 之后出现 (圆心来自 MidPoint 之后的
	// tkzPointResult, 颠倒会取到 VecK 的中间结果)。
	if strings.Index(shim, `\pgfnodealias{tkzFirstPointResult}`) < strings.Index(shim, `\pgfnodealias{tkzSecondPointResult}`) {
		t.Error("tkzFirstPointResult must be assigned after tkzSecondPointResult (center from MidPoint)")
	}
}

func TestTikzOutputFormat(t *testing.T) {
	tests := []struct {
		name    string
		outFile string
		want    string
		wantErr bool
	}{
		{name: "png", outFile: "fig.png", want: "png"},
		{name: "jpg", outFile: "fig.jpg", want: "jpg"},
		{name: "jpeg normalized", outFile: "fig.jpeg", want: "jpg"},
		{name: "svg", outFile: "fig.svg", want: "svg"},
		{name: "pdf", outFile: "fig.pdf", want: "pdf"},
		{name: "uppercase ext", outFile: "fig.PNG", want: "png"},
		{name: "no extension", outFile: "fig", wantErr: true},
		{name: "unsupported", outFile: "fig.gif", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tikzOutputFormat(tt.outFile)
			if tt.wantErr {
				if err == nil {
					t.Errorf("tikzOutputFormat(%q) = %q, want error", tt.outFile, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("tikzOutputFormat(%q) unexpected error: %v", tt.outFile, err)
			}
			if got != tt.want {
				t.Errorf("tikzOutputFormat(%q) = %q, want %q", tt.outFile, got, tt.want)
			}
		})
	}
}

func TestResolveMutoolOutput(t *testing.T) {
	t.Run("plain name exists", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "out.png")
		if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		if got := resolveMutoolOutput(p); got != p {
			t.Errorf("resolveMutoolOutput() = %q, want %q", got, p)
		}
	})

	t.Run("convert numbered suffix", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "out1.svg")
		if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(dir, "out.svg")
		if got := resolveMutoolOutput(want); got != p {
			t.Errorf("resolveMutoolOutput() = %q, want %q", got, p)
		}
	})

	t.Run("draw dashed suffix", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "out-1.png")
		if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(dir, "out.png")
		if got := resolveMutoolOutput(want); got != p {
			t.Errorf("resolveMutoolOutput() = %q, want %q", got, p)
		}
	})

	t.Run("nothing produced", func(t *testing.T) {
		dir := t.TempDir()
		if got := resolveMutoolOutput(filepath.Join(dir, "missing.png")); got != "" {
			t.Errorf("resolveMutoolOutput() = %q, want empty", got)
		}
	})
}

func TestDetectTikzPackages(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{name: "plain tikz", content: "\\draw (0,0) -- (1,1);"},
		{
			name:    "tkz-euclide",
			content: "\\tkzDefPoint(0,0){A}\n\\tkzDrawPoints(A,B)",
			want:    []string{"tkz-euclide"},
		},
		{
			name:    "tikz-cd",
			content: "\\begin{tikzcd}\nA \\arrow{r} & B\n\\end{tikzcd}",
			want:    []string{"tikz-cd"},
		},
		{
			name:    "pgfplots dedup",
			content: "\\begin{axis}\n\\addplot {x^2};\n\\end{axis}",
			want:    []string{"pgfplots"},
		},
		{
			name:    "pgfplots semilogxaxis",
			content: "\\begin{semilogxaxis}\n\\addplot {x};\n\\end{semilogxaxis}",
			want:    []string{"pgfplots"},
		},
		{
			name:    "multiple packages in table order",
			content: "\\begin{tikzcd}X\\end{tikzcd}\\tkzDefPoint(0,0){A}",
			want:    []string{"tkz-euclide", "tikz-cd"},
		},
		{
			name:    "circuitikz",
			content: "\\begin{circuitikz}\n\\draw (0,0) to[R] (2,0);\n\\end{circuitikz}",
			want:    []string{"circuitikz"},
		},
		{
			name:    "tikz-3dplot",
			content: "\\tdplotsetmaincoords{60}{110}\n\\draw (0,0,0) -- (1,1,1);",
			want:    []string{"tikz-3dplot"},
		},
		{
			name:    "forest",
			content: "\\begin{forest}\n[a [b]]\n\\end{forest}",
			want:    []string{"forest"},
		},
		{
			name:    "smartdiagram",
			content: "\\smartdiagram[bubble diagram]{A, B}",
			want:    []string{"smartdiagram"},
		},
		{
			name:    "circuit ee IEC style",
			content: "\\begin{tikzpicture}[circuit ee IEC]\n\\draw (0,0) to[resistor={name=R}] (0,2);\n\\end{tikzpicture}",
			want:    []string{"circuitikz"},
		},
		{
			name:    "circuitikz compat star component",
			content: "\\draw (0,0) to[*R=$R_1$] (1.5,0) to[*Tnpn] (3,0) to[*D](3,2);",
			want:    []string{"circuitikz"},
		},
		{
			name:    "upgreek upright mu",
			content: "\\node {$\\upmu$C};",
			want:    []string{"upgreek"},
		},
		{
			name:    "siunitx micro farad in circuitikz label",
			content: "\\draw (0,0) to[C, l=10<\\micro\\farad>] (0,2);",
			want:    []string{"siunitx"},
		},
		{
			name:    "siunitx prefix command",
			content: "\\draw (0,0) to[R, l=1<\\kilo\\ohm>] (2,0);",
			want:    []string{"siunitx"},
		},
		{
			name:    "siunitx SI command",
			content: "\\node at (0,0) {\\SI{10}{\\micro\\farad}};",
			want:    []string{"siunitx"},
		},
		{
			name:    "siunitx si command",
			content: "\\node at (0,0) {\\si{\\ohm}};",
			want:    []string{"siunitx"},
		},
		{
			name:    "siunitx setup",
			content: "\\sisetup{detect-all}\n\\draw (0,0);",
			want:    []string{"siunitx"},
		},
		{
			name:    "siunitx num qty unit",
			content: "\\node {\\num{3.14}, \\qty{10}{\\farad}, \\unit{\\ohm}};",
			want:    []string{"siunitx"},
		},
		{
			name:    "sin is kernel not siunitx",
			content: "\\node at (0,0) {$\\sin x$};",
		},
		{
			name:    "sigma and sim are kernel not siunitx",
			content: "\\node at (0,0) {$\\sigma \\sim \\mu$};",
		},
		{
			name:    "number and numexpr are kernel not siunitx",
			content: "\\node at (0,0) {\\number\\value{page}, \\numexpr 1+2\\relax};",
		},
		{
			name:    "unitlength is kernel not siunitx",
			content: "\\setlength{\\unitlength}{1cm}\n\\begin{picture}(2,2)\\end{picture}",
		},
		{
			name:    "plain text micro not siunitx",
			content: "\\node at (0,0) {microcontroller};",
		},
		{
			name:    "upgreek other letters",
			content: "\\node {$\\upalpha, \\upomega$};",
			want:    []string{"upgreek"},
		},
		{
			name:    "uparrow is kernel not upgreek",
			content: "\\node {$\\uparrow$};",
		},
		{
			name:    "mu without up prefix",
			content: "\\node {$\\mu$};",
		},
		{
			name:    "tcblisting loads tcolorbox and tikzlings subpackage",
			content: "\\begin{tcblisting}{title={Basic Ti\\emph{k}Zling}}\n\\marmot\n\\end{tcblisting}",
			want:    []string{"tcolorbox", "tikzlings-marmots"},
		},
		{
			name:    "tikzlings animal loads its subpackage",
			content: "\\penguin[rotate=30,scale=0.5]",
			want:    []string{"tikzlings-penguins"},
		},
		{
			name:    "tikzlings animals in table order",
			content: "\\marmot\n\\bear[hat]",
			want:    []string{"tikzlings-bears", "tikzlings-marmots"},
		},
		{
			name:    "random tikzling loads base package",
			content: "\\tikzling[body=blue]",
			want:    []string{"tikzlings"},
		},
		{
			name:    "thing accessory loads addons",
			content: "\\owl\n\\thing[tophat, scale=1.5]",
			want:    []string{"tikzlings-addons", "tikzlings-owls"},
		},
		{
			name:    "bearwear loads bearwear package, not tikzlings-bears",
			content: "\\bearwear",
			want:    []string{"bearwear"},
		},
		{
			name:    "dressed bear example loads bearwear and bears",
			content: "\\bear\n\\bearwear[long sleeves,\n  shirt=red!80!black]",
			want:    []string{"bearwear", "tikzlings-bears"},
		},
		{
			name:    "bearwearsetup loads bearwear package",
			content: "\\bearwearsetup{shirt=red}",
			want:    []string{"bearwear"},
		},
		{
			name:    "bearwear in prose is not a command",
			content: "the bearwear package provides clothes",
		},
		{
			name:    "marmotx is not marmot",
			content: "\\marmotx",
		},
		{
			name:    "businessman node loads tikzpeople",
			content: "\\node[businessman,minimum size=1.5cm] at (0,0) {};",
			want:    []string{"tikzpeople"},
		},
		{
			name:    "tikzpeople shape after comma",
			content: "\\node[draw, alice] at (0,0) {};",
			want:    []string{"tikzpeople"},
		},
		{
			name:    "tikzpeople shape in multiline options",
			content: "\\node[\n  businessman,\n  minimum size=1.5cm] at (0,0) {};",
			want:    []string{"tikzpeople"},
		},
		{
			name:    "businessman in prose is not a shape",
			content: "the businessman walks home",
		},
		{
			name:    "alice in node text is not a shape",
			content: "\\node {alice};",
		},
		{
			name:    "coordinate alice is not a shape",
			content: "\\draw (alice) -- (bob);",
		},
		{
			name:    "comma coordinate pair accepted (known over-match)",
			content: "\\draw (0,0) -- (1,alice);",
			want:    []string{"tikzpeople"},
		},
		{
			name:    "figchild CamelCase command with options",
			content: "\\fcOwlA[scale=0.5]",
			want:    []string{"figchild"},
		},
		{
			name:    "figchild lowercase exception frog",
			content: "\\fcfrog",
			want:    []string{"figchild"},
		},
		{
			name:    "fcolorbox is kernel, not figchild",
			content: "\\fcolorbox{red}{blue}{x}",
		},
		{
			name:    "figchild in prose is not a command",
			content: "the figchild package provides figures",
		},
		{
			name:    "tikz-triminos command loads only tikz-triminos",
			content: "\\tkztriminos{One § Two § Three}",
			want:    []string{"tikz-triminos"},
		},
		{
			name:    "tkztriminosize is internal, not the command",
			content: "\\tkztriminosize",
		},
		{
			name:    "scsnowman command with keys",
			content: "\\scsnowman[scale=2,hat=red]",
			want:    []string{"scsnowman"},
		},
		{
			name:    "scsnowmannumeral command",
			content: "\\scsnowmannumeral{18882}",
			want:    []string{"scsnowman"},
		},
		{
			name:    "makeitemsnowman command",
			content: "\\makeitemsnowman",
			want:    []string{"scsnowman"},
		},
		{
			name:    "enumsnowman bare word in pagenumbering",
			content: "\\pagenumbering{enumsnowman}",
			want:    []string{"scsnowman"},
		},
		{
			name:    "scsnowman in prose is not a command",
			content: "the scsnowman package draws snowmen",
		},
		{
			name:    "scsnowman internal name with numeral capital N",
			content: "\\scsnowmanNumeral{5}",
		},
		{
			name:    "scsnowman internal namespace with at sign",
			content: "\\scsnowman@internal",
		},
		{
			name:    "usescsnowmanlibrary command",
			content: "\\usescsnowmanlibrary{extras}",
			want:    []string{"scsnowman"},
		},
		{
			name:    "makeqedsnowman command",
			content: "\\makeqedsnowman",
			want:    []string{"scsnowman"},
		},
		{
			name:    "makeqedother joke command",
			content: "\\makeqedother",
			want:    []string{"scsnowman"},
		},
		{
			name:    "makeitemother joke command",
			content: "\\makeitemother",
			want:    []string{"scsnowman"},
		},
		{
			name:    "scsnowmannumeral lowercase is the public command",
			content: "\\scsnowmannumeral{18882}",
			want:    []string{"scsnowman"},
		},
		{
			name:    "enumsnowman command loads scsnowman",
			content: "\\enumsnowman",
			want:    []string{"scsnowman"},
		},
		{
			name:    "the enumsnowman style in prose is not a command",
			content: "the enumsnowman style",
		},
		{
			name:    "enumsnowman internal namespace with at sign",
			content: "\\enumsnowman@internal",
		},
		{
			name:    "enumsnowman followed by underscore is the command",
			content: "\\enumsnowman_foo",
			want:    []string{"scsnowman"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectTikzPackages(tt.content, true)
			if len(got) != len(tt.want) {
				t.Fatalf("detectTikzPackages() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("detectTikzPackages() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

// TestDetectTikzpeopleShapes 逐一验证全部 29 个人形名都能在键位被探测到,
// 防止名单 / 正则 typo 使某个 shape 静默失效 (名单是正则的单一来源)。
func TestDetectTikzpeopleShapes(t *testing.T) {
	if len(tikzpeopleShapes) != 29 {
		t.Errorf("tikzpeopleShapes has %d names, want 29", len(tikzpeopleShapes))
	}
	for _, name := range tikzpeopleShapes {
		content := "\\node[" + name + "] at (0,0) {};"
		if got := detectTikzPackages(content, true); !slices.Contains(got, "tikzpeople") {
			t.Errorf("detectTikzPackages(%q) = %v, want tikzpeople", content, got)
		}
	}
}

// TestDetectTikzPackagesLegacyIEC 验证 circuit ee IEC 内容在非 legacy (latexmk)
// 模式下不再额外引入 circuitikz: TL2026 自带真的 circuits.ee.IEC 库, 不需要
// circuitikz 兜底; 只有 tectonic (2021 bundle 缺该库) 才需要。
func TestDetectTikzPackagesLegacyIEC(t *testing.T) {
	content := "\\usetikzlibrary{circuits.ee.IEC}\n\\begin{tikzpicture}[circuit ee IEC]\n\\draw (0,0) to[resistor={name=R}] (0,2);\n\\end{tikzpicture}"
	if got := detectTikzPackages(content, false); len(got) != 0 {
		t.Errorf("detectTikzPackages(legacyIEC=false) = %v, want none", got)
	}
	if got := detectTikzPackages(content, true); len(got) != 1 || got[0] != "circuitikz" {
		t.Errorf("detectTikzPackages(legacyIEC=true) = %v, want [circuitikz]", got)
	}
}

func TestExtractUserPackages(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantPkgs []string
		wantRest string
	}{
		{
			name:     "no usepackage unchanged",
			input:    "\\draw (0,0);",
			wantRest: "\\draw (0,0);",
		},
		{
			name:     "single package hoisted",
			input:    "\\usepackage{mypkg}\n\\draw (0,0);",
			wantPkgs: []string{"mypkg"},
			wantRest: "\\draw (0,0);",
		},
		{
			name:     "options and indentation and CRLF",
			input:    "  \\usepackage[utf8]{inputenc}\r\n\\node{a};",
			wantPkgs: []string{"inputenc"},
			wantRest: "\\node{a};",
		},
		{
			name:     "comma separated",
			input:    "\\usepackage{aa, bb}\n\\draw (0,0);",
			wantPkgs: []string{"aa", "bb"},
			wantRest: "\\draw (0,0);",
		},
		{
			name:     "inside tikzpicture also hoisted",
			input:    "\\begin{tikzpicture}\n\\usepackage{weird}\n\\draw (0,0);\n\\end{tikzpicture}",
			wantPkgs: []string{"weird"},
			wantRest: "\\begin{tikzpicture}\n\\draw (0,0);\n\\end{tikzpicture}",
		},
		{
			name:     "circuitikzgit normalized to circuitikz",
			input:    "\\usepackage[compatibility]{circuitikzgit}\n\\draw (0,0);",
			wantPkgs: []string{"circuitikz"},
			wantRest: "\\draw (0,0);",
		},
		{
			name:     "commented usepackage untouched",
			input:    "% \\usepackage{ghost}\n\\draw (0,0);",
			wantRest: "% \\usepackage{ghost}\n\\draw (0,0);",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkgs, rest := extractUserPackages(tt.input)
			if len(pkgs) != len(tt.wantPkgs) {
				t.Fatalf("extractUserPackages() pkgs = %v, want %v", pkgs, tt.wantPkgs)
			}
			for i := range pkgs {
				if pkgs[i] != tt.wantPkgs[i] {
					t.Fatalf("extractUserPackages() pkgs = %v, want %v", pkgs, tt.wantPkgs)
				}
			}
			if rest != tt.wantRest {
				t.Errorf("extractUserPackages() rest = %q, want %q", rest, tt.wantRest)
			}
		})
	}
}

func TestMergeTikzPackages(t *testing.T) {
	tests := []struct {
		name     string
		explicit []string
		detected []string
		want     []string
	}{
		{name: "both empty"},
		{name: "explicit only", explicit: []string{"a"}, want: []string{"a"}},
		{name: "detected only", detected: []string{"a"}, want: []string{"a"}},
		{name: "combined", explicit: []string{"a"}, detected: []string{"b"}, want: []string{"a", "b"}},
		{name: "dedup explicit first", explicit: []string{"a"}, detected: []string{"a", "b"}, want: []string{"a", "b"}},
		{name: "explicit order preserved", explicit: []string{"b"}, detected: []string{"a", "b"}, want: []string{"b", "a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeTikzPackages(tt.explicit, tt.detected)
			if len(got) != len(tt.want) {
				t.Fatalf("mergeTikzPackages() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("mergeTikzPackages() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

// TestExtractUserTikzLibraries 验证显式 \usetikzlibrary 行的提取与去行:
// \usetikzlibrary{a,b} 与 pgf 的 \usetikzlibrary[a,b] 两种写法都要收
// (见 tikz.code.tex 的 \usetikzlibrary 定义), 行首整行才提, 注释行不动。
func TestExtractUserTikzLibraries(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantLibs []string
		wantRest string
	}{
		{name: "no library unchanged", input: "\\draw (0,0);", wantRest: "\\draw (0,0);"},
		{
			name:     "single library hoisted",
			input:    "\\usetikzlibrary{ducks}\n\\begin{tikzpicture}\n\\draw (0,0);\n\\end{tikzpicture}",
			wantLibs: []string{"ducks"},
			wantRest: "\\begin{tikzpicture}\n\\draw (0,0);\n\\end{tikzpicture}",
		},
		{
			name:     "brace form comma separated",
			input:    "\\usetikzlibrary{arrows.meta, calc}\n\\draw (0,0);",
			wantLibs: []string{"arrows.meta", "calc"},
			wantRest: "\\draw (0,0);",
		},
		{
			name:     "pgf optional-list form and CRLF",
			input:    "  \\usetikzlibrary[patterns,topaths]\r\n\\draw (0,0);",
			wantLibs: []string{"patterns", "topaths"},
			wantRest: "\\draw (0,0);",
		},
		{
			name:     "inside tikzpicture also hoisted",
			input:    "\\begin{tikzpicture}\n\\usetikzlibrary{calc}\n\\draw (0,0);\n\\end{tikzpicture}",
			wantLibs: []string{"calc"},
			wantRest: "\\begin{tikzpicture}\n\\draw (0,0);\n\\end{tikzpicture}",
		},
		{
			name:     "commented line untouched",
			input:    "% \\usetikzlibrary{ghost}\n\\draw (0,0);",
			wantRest: "% \\usetikzlibrary{ghost}\n\\draw (0,0);",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			libs, rest := extractUserTikzLibraries(tt.input)
			if len(libs) != len(tt.wantLibs) {
				t.Fatalf("extractUserTikzLibraries() libs = %v, want %v", libs, tt.wantLibs)
			}
			for i := range libs {
				if libs[i] != tt.wantLibs[i] {
					t.Fatalf("extractUserTikzLibraries() libs = %v, want %v", libs, tt.wantLibs)
				}
			}
			if rest != tt.wantRest {
				t.Errorf("extractUserTikzLibraries() rest = %q, want %q", rest, tt.wantRest)
			}
		})
	}
}

// TestMergeTikzLibraries 验证显式库与自动探测库的合并 (显式优先, 去重保序)。
func TestMergeTikzLibraries(t *testing.T) {
	got := mergeTikzLibraries([]string{"ducks"}, []string{"fit", "ducks"})
	want := []string{"ducks", "fit"}
	if len(got) != len(want) {
		t.Fatalf("mergeTikzLibraries() = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("mergeTikzLibraries() = %v, want %v", got, want)
		}
	}
}

// TestDetectTikzPackagesTikzducksPicture 验证 picture 模式命令 \picduck 触发
// tikzducks 宏包探测 (issue #2): \duck / \randuck 走 TikZ 库, \picduck 只由
// 宏包本体提供, 不探测会报 "Undefined control sequence"。
func TestDetectTikzPackagesTikzducksPicture(t *testing.T) {
	got := detectTikzPackages(`\picduck`, false)
	if !slices.Contains(got, "tikzducks") {
		t.Fatalf("detectTikzPackages(\\picduck) = %v, want tikzducks", got)
	}
	if got := detectTikzPackages(`\picnic`, false); slices.Contains(got, "tikzducks") {
		t.Fatalf("detectTikzPackages(\\picnic) = %v, must not contain tikzducks", got)
	}
}

func TestDetectTikzLibraries(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{name: "plain tikz no libraries", content: "\\draw (0,0) -- (1,1);"},
		{name: "fit bounding box", content: "\\node[draw, fit=(a)(b)] {};", want: []string{"fit"}},
		{name: "calc coordinate", content: "\\node at ($(a)!0.5!(b)$) {};", want: []string{"calc"}},
		{name: "positioning", content: "\\node[right=of foo] {};", want: []string{"positioning"}},
		{name: "arrows.meta", content: "\\draw[-Stealth] (0,0) -- (1,1);", want: []string{"arrows.meta"}},
		{name: "arrows.meta braced", content: "\\draw[-{Latex[length=3pt]}] (0,0) -- (1,1);", want: []string{"arrows.meta"}},
		{name: "arrows legacy", content: "\\draw[-stealth] (0,0) -- (1,1);", want: []string{"arrows"}},
		{name: "patterns", content: "\\fill[pattern=north east lines] (0,0) rectangle (1,1);", want: []string{"patterns"}},
		{name: "decorations", content: "\\draw[decorate, decoration={brace}] (0,0) -- (1,1);", want: []string{"decorations.pathreplacing"}},
		{name: "quotes label", content: "\\draw (a) to[\"label\"] (b);", want: []string{"quotes"}},
		{name: "shapes.geometric", content: "\\node[ellipse, draw] {};", want: []string{"shapes.geometric"}},
		{name: "combined dedup and order", content: "\\node[fit=(a)(b)] {};\n\\node at ($(a)!0.5!(b)$) {};\n\\node[fit=(c)] {};", want: []string{"fit", "calc"}},
		{name: "to path without dash not arrows", content: "\\draw (0,0) to (1,1);"},
		{name: "node text LaTeX not arrows.meta", content: "\\node {LaTeX};"},
		{name: "plain text fit not option", content: "\\node {a fit b};"},
		{name: "canvas zy plane at", content: "\\begin{scope}[canvas is zy plane at x=\\thelayer*1.8]", want: []string{"3d"}},
		{name: "canvas xy plane at", content: "\\begin{scope}[canvas is xy plane at z=1]", want: []string{"3d"}},
		{name: "canvas bare plane", content: "\\begin{scope}[canvas is plane={O(0,0) x(1,0) y(0,1)}]", want: []string{"3d"}},
		{name: "canvas text only not 3d", content: "\\node {the canvas is plain};"},
		{name: "duck loads ducks library", content: "\\duck", want: []string{"ducks"}},
		{name: "duck with options loads ducks library", content: "\\duck[shift={(2.5,1)}, scale=.3]", want: []string{"ducks"}},
		{name: "randuck loads ducks library", content: "\\randuck[body=blue]", want: []string{"ducks"}},
		{name: "ducksay is not duck", content: "\\ducksay{quack}"},
		{name: "duckling is not duck", content: "\\duckling"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectTikzLibraries(tt.content)
			if len(got) != len(tt.want) {
				t.Fatalf("detectTikzLibraries() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("detectTikzLibraries() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

// TestDetectTikzlingsPics 验证 pic 语法 (pic{bear} / pic[opts]{coati}) 的名字探测:
// 只认表内名字 (含 tikzling, 不含没有 pic 定义的 thing), 词边界排除
// picnic / picture / topic, 大小写敏感, 按表顺序去重。
func TestDetectTikzlingsPics(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{name: "bare pic", content: `\path (1,0) pic{bear};`, want: []string{"bear"}},
		{name: "space before name", content: `\path pic {penguin};`, want: []string{"penguin"}},
		{name: "options", content: `pic[coati/body=blue, scale=0.5]{coati}`, want: []string{"coati"}},
		{name: "options multiline", content: "pic[\n  thing/hat=red\n]{penguin}", want: []string{"penguin"}},
		{name: "backslash pic command", content: `\pic{bear};`, want: []string{"bear"}},
		{name: "brace spacing", content: `pic{ bear }`, want: []string{"bear"}},
		{name: "random tikzling", content: `pic{tikzling}`, want: []string{"tikzling"}},
		{name: "table order and dedup", content: `pic{penguin} pic{bear} pic{bear}`, want: []string{"bear", "penguin"}},
		{name: "thing has no pic", content: `pic{thing}`, want: nil},
		{name: "unknown name", content: `pic{seagull}`, want: nil},
		{name: "picnic is not pic", content: `picnic{penguin}`, want: nil},
		{name: "picture is not pic", content: `picture{penguin}`, want: nil},
		{name: "topic is not pic", content: `topic {penguin}`, want: nil},
		{name: "case sensitive", content: `pic{Bear}`, want: nil},
		{name: "empty name", content: `pic{}`, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectTikzlingsPics(tt.content); !slices.Equal(got, tt.want) {
				t.Errorf("detectTikzlingsPics(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}

// TestTikzlingsPicPackages 验证 legacy 后端 pic 语法要加载的动物子宏包 (表顺序)。
func TestTikzlingsPicPackages(t *testing.T) {
	got := tikzlingsPicPackages(`pic{penguin} pic{bear} pic{seagull}`)
	want := []string{"tikzlings-bears", "tikzlings-penguins"}
	if !slices.Equal(got, want) {
		t.Errorf("tikzlingsPicPackages() = %v, want %v", got, want)
	}
	if got := tikzlingsPicPackages(`\draw (0,0) -- (1,1);`); got != nil {
		t.Errorf("tikzlingsPicPackages(plain) = %v, want nil", got)
	}
}

// TestTikzLibrariesTikzlingsPics 验证 pic 语法只在非 legacy 后端注入 tikzlings
// 库 (legacy bundle 没有该库文件, 走动物子宏包 + tikzlingsPicShim)。
func TestTikzLibrariesTikzlingsPics(t *testing.T) {
	pic := "\\begin{tikzpicture}\n\\path (1,0) pic{bear};\n\\end{tikzpicture}"
	if got := tikzLibraries(latexmkProfile(), pic); !slices.Contains(got, "tikzlings") {
		t.Errorf("tikzLibraries(latexmk, pic{bear}) = %v, want tikzlings", got)
	}
	if got := tikzLibraries(tectonicProfile(), pic); slices.Contains(got, "tikzlings") {
		t.Errorf("tikzLibraries(tectonic, pic{bear}) = %v, must not contain tikzlings (bundle lacks the library)", got)
	}
	if got := tikzLibraries(latexmkProfile(), `\path pic{seagull};`); slices.Contains(got, "tikzlings") {
		t.Errorf("tikzLibraries(latexmk, pic{seagull}) = %v, must not contain tikzlings", got)
	}
}

// TestTikzShimsTikzlingsPic 验证 legacy 的 pic shim: 复刻库文件的 <name>/.pic
// 与 thing/.search also (缺后者时 pic[thing/hat=red] 报未知键); latexmk 不注入。
func TestTikzShimsTikzlingsPic(t *testing.T) {
	picRaw := `\path (1,0) pic{bear};`
	got := tikzShims(tectonicProfile(), picRaw, nil)
	for _, want := range []string{
		`\tikzset{thing/.search also={,/tikz,/pgf}}`,
		`\tikzset{bear/.pic={\bear},bear/.search also={,/tikz,/pgf,/thing}}`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("tectonic shims missing tikzlings pic shim %q in:\n%s", want, got)
		}
	}
	if got := tikzShims(tectonicProfile(), `\draw (0,0) -- (1,1);`, nil); strings.Contains(got, "/.pic=") {
		t.Errorf("tectonic shims must not inject pic shim without pics: %q", got)
	}
	if got := tikzShims(latexmkProfile(), picRaw, nil); strings.Contains(got, "/.pic=") {
		t.Errorf("latexmk shims must not inject tikzlings pic shim (uses the real library): %q", got)
	}
}

func TestIecNeedsLibrary(t *testing.T) {
	// want=true 表示要注入 \usetikzlibrary{circuits.ee.IEC} 加载行, 与内容是否
	// 已自行加载无关: latexmk 上一律注入 (pgf 重复加载幂等), 漏注入才会复现回归。
	bare := "\\begin{tikzpicture}[circuit ee IEC]\n\\draw (0,0) to[resistor={name=R}] (0,2);\n\\end{tikzpicture}"
	loaded := "\\usetikzlibrary{circuits.ee.IEC}\n" + bare
	commaLoaded := "\\usetikzlibrary{calc,circuits.ee.IEC}\n" + bare
	commented := "% \\usetikzlibrary{circuits.ee.IEC}\n" + bare
	plain := "\\draw (0,0) -- (1,1);"
	tests := []struct {
		name    string
		profile tikzProfile
		content string
		want    bool
	}{
		{name: "bare latexmk", profile: latexmkProfile(), content: bare, want: true},
		{name: "bare tectonic", profile: tectonicProfile(), content: bare, want: false},
		{name: "loaded latexmk", profile: latexmkProfile(), content: loaded, want: true},
		{name: "comma loaded latexmk", profile: latexmkProfile(), content: commaLoaded, want: true},
		{name: "commented out load latexmk", profile: latexmkProfile(), content: commented, want: true},
		{name: "no iec latexmk", profile: latexmkProfile(), content: plain, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := iecNeedsLibrary(tt.profile, tt.content); got != tt.want {
				t.Errorf("iecNeedsLibrary() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTikzLibraries(t *testing.T) {
	bare := "\\begin{tikzpicture}[circuit ee IEC]\n\\draw (0,0) to[resistor={name=R}] (0,2);\n\\end{tikzpicture}"
	loaded := "\\usetikzlibrary{circuits.ee.IEC}\n" + bare
	calcAndIEC := "\\begin{tikzpicture}[circuit ee IEC]\n\\draw ($(0,0)$) to[resistor={name=R}] (0,2);\n\\end{tikzpicture}"
	gotBare := tikzLibraries(latexmkProfile(), bare)
	if !slices.Contains(gotBare, "circuits.ee.IEC") {
		t.Errorf("tikzLibraries(latexmk, bare) = %v, want circuits.ee.IEC", gotBare)
	}
	gotTectonic := tikzLibraries(tectonicProfile(), bare)
	if slices.Contains(gotTectonic, "circuits.ee.IEC") {
		t.Errorf("tikzLibraries(tectonic, bare) = %v, must not contain circuits.ee.IEC", gotTectonic)
	}
	// 内容自带加载行时仍注入一次, 但绝不能重复 (pgf 幂等, 重复也无害, 只为整洁)。
	gotLoaded := tikzLibraries(latexmkProfile(), loaded)
	n := 0
	for _, lib := range gotLoaded {
		if lib == "circuits.ee.IEC" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("tikzLibraries(latexmk, loaded) = %v, want exactly one circuits.ee.IEC, got %d", gotLoaded, n)
	}
	gotCalcAndIEC := tikzLibraries(latexmkProfile(), calcAndIEC)
	if !slices.Contains(gotCalcAndIEC, "calc") || !slices.Contains(gotCalcAndIEC, "circuits.ee.IEC") {
		t.Errorf("tikzLibraries(latexmk, calc+IEC) = %v, want calc and circuits.ee.IEC", gotCalcAndIEC)
	}
	canvas := "\\begin{scope}[canvas is zy plane at x=\\thelayer*1.8]"
	gotCanvas := tikzLibraries(latexmkProfile(), canvas)
	if !slices.Contains(gotCanvas, "3d") {
		t.Errorf("tikzLibraries(canvas is zy plane at) = %v, want 3d", gotCanvas)
	}
	// \duck / \randuck 注入 ducks 库: 两个后端都要注入 (2021 bundle v1.5 也自带库文件)。
	duck := "\\duck\n\\randuck[body=blue]"
	if got := tikzLibraries(latexmkProfile(), duck); !slices.Contains(got, "ducks") {
		t.Errorf("tikzLibraries(latexmk, duck) = %v, want ducks", got)
	}
	if got := tikzLibraries(tectonicProfile(), duck); !slices.Contains(got, "ducks") {
		t.Errorf("tikzLibraries(tectonic, duck) = %v, want ducks", got)
	}
}

func TestTikzLibraryLines(t *testing.T) {
	if got := tikzLibraryLines(nil); got != "" {
		t.Errorf("tikzLibraryLines(nil) = %q, want empty", got)
	}
	if got := tikzLibraryLines([]string{"fit"}); got != "\\usetikzlibrary{fit}\n" {
		t.Errorf("tikzLibraryLines single = %q", got)
	}
	if got := tikzLibraryLines([]string{"fit", "calc"}); got != "\\usetikzlibrary{fit,calc}\n" {
		t.Errorf("tikzLibraryLines multiple = %q", got)
	}
}
func TestTikzDocColorShims(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "referenced and undefined injected",
			content: "\\draw[themecolor] (0,0) -- (1,1);",
			want:    "\\providecolor{themecolor}{RGB}{136,46,114}\n",
		},
		{
			name:    "definecolor self defined not injected",
			content: "\\definecolor{themecolor}{RGB}{1,2,3}\n\\draw[themecolor] (0,0) -- (1,1);",
		},
		{
			name:    "colorlet self defined not injected",
			content: "\\colorlet{themecolor}{red}\n\\draw[themecolor] (0,0) -- (1,1);",
		},
		{
			name:    "providecolor self defined not injected",
			content: "\\providecolor{themecolor}{RGB}{1,2,3}\n\\draw[themecolor] (0,0) -- (1,1);",
		},
		{
			name:    "colorlet same prefix name still injected",
			content: "\\colorlet{themecolorfoo}{red}\n\\draw[themecolor] (0,0) -- (1,1);",
			want:    "\\providecolor{themecolor}{RGB}{136,46,114}\n",
		},
		{
			name:    "providecolor optional model not injected",
			content: "\\providecolor[RGB]{themecolor}{1,2,3}\n\\draw[themecolor] (0,0) -- (1,1);",
		},
		{name: "unreferenced empty", content: "\\draw (0,0) -- (1,1);"},
		{name: "empty input", content: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tikzDocColorShims(tt.content)
			if got != tt.want {
				t.Fatalf("tikzDocColorShims() = %q, want %q", got, tt.want)
			}
			// 注入时必须恰好一行 \providecolor, 不重复。
			if n := strings.Count(got, "\\providecolor"); n > 1 {
				t.Fatalf("tikzDocColorShims() returned %d providecolor lines, want at most 1", n)
			}
		})
	}
}

func TestSiunitxAliasShim(t *testing.T) {
	if got := siunitxAliasShim(nil); got != "" {
		t.Errorf("siunitxAliasShim(nil) = %q, want empty", got)
	}
	if got := siunitxAliasShim([]string{"circuitikz"}); got != "" {
		t.Errorf("siunitxAliasShim(no siunitx) = %q, want empty", got)
	}
	got := siunitxAliasShim([]string{"circuitikz", "siunitx"})
	for _, want := range []string{`\newcommand{\tikzsiunitx@alias}[2]`, `\tikzsiunitx@alias{micro}`, `\tikzsiunitx@alias{farad}{\si{\farad}}`, `\tikzsiunitx@alias{ohm}{\si{\ohm}}`} {
		if !strings.Contains(got, want) {
			t.Errorf("siunitxAliasShim() missing %q in:\n%s", want, got)
		}
	}
}

func TestCircuitikzBuzzerShim(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string // "" = no shim
	}{
		{name: "no buzzer", content: "\\begin{circuitikz}\n\\draw (0,0) to[R] (2,0);\n\\end{circuitikz}"},
		{
			name:    "buzzer bipole",
			content: "\\begin{circuitikz}\n\\draw (0,3) to[buzzer, a=Buzzer] (0,5);\n\\end{circuitikz}",
			want:    tikzBuzzerShim + "\n",
		},
		{
			name:    "reversed buzzer",
			content: "\\draw (0,0) to[rbuzzer] (2,0);",
			want:    tikzBuzzerShim + "\n",
		},
		{
			name:    "whitespace before name",
			content: "\\draw (0,0) to[ rbuzzer ] (2,0);",
			want:    tikzBuzzerShim + "\n",
		},
		{
			name:    "buzzer label only not a bipole",
			content: "\\draw (0,0) to[R, a=Buzzer] (2,0);",
		},
		{
			name:    "commented out usage ignored",
			content: "% \\draw (0,0) to[buzzer] (2,0);\n\\draw (0,0) to[R] (2,0);",
		},
		{
			name:    "inline comment after real usage still triggers",
			content: "\\draw (0,0) to[buzzer] (2,0); % buzzer comment",
			want:    tikzBuzzerShim + "\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := circuitikzBuzzerShim(tt.content); got != tt.want {
				t.Errorf("circuitikzBuzzerShim() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCircuitikzMotorShim(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string // "" = no shim
	}{
		{name: "no motor", content: "\\begin{circuitikz}\n\\draw (0,0) to[R] (2,0);\n\\end{circuitikz}"},
		{
			name:    "motor bipole",
			content: "\\begin{circuitikz}\n\\draw (0,3) to[motor, l=电动机] (0,5);\n\\end{circuitikz}",
			want:    tikzMotorShim + "\n",
		},
		{
			name:    "whitespace before name",
			content: "\\draw (0,0) to[ motor ] (2,0);",
			want:    tikzMotorShim + "\n",
		},
		{
			name:    "motorcycle not a bipole",
			content: "\\draw (0,0) to[motorcycle] (2,0);",
		},
		{
			name:    "commented out usage ignored",
			content: "% \\draw (0,0) to[motor] (2,0);\n\\draw (0,0) to[R] (2,0);",
		},
		{
			name:    "inline comment after real usage still triggers",
			content: "\\draw (0,0) to[motor] (2,0); % motor comment",
			want:    tikzMotorShim + "\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := circuitikzMotorShim(tt.content); got != tt.want {
				t.Errorf("circuitikzMotorShim() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTikzPackageLines(t *testing.T) {
	if got := tikzPackageLines(nil, false); got != "" {
		t.Errorf("tikzPackageLines(nil) = %q, want empty", got)
	}
	if got := tikzPackageLines([]string{"tkz-euclide"}, false); got != "\\usepackage{tkz-euclide}\n" {
		t.Errorf("tikzPackageLines single = %q", got)
	}
	want := "\\usepackage{a}\n\\usepackage{b}\n"
	if got := tikzPackageLines([]string{"a", "b"}, false); got != want {
		t.Errorf("tikzPackageLines multiple = %q, want %q", got, want)
	}
	if got := tikzPackageLines([]string{"circuitikz"}, false); got != "\\usepackage{circuitikz}\n" {
		t.Errorf("tikzPackageLines plain circuitikz = %q", got)
	}
	if got := tikzPackageLines([]string{"circuitikz"}, true); got != "\\usepackage[compatibility]{circuitikz}\n" {
		t.Errorf("tikzPackageLines compat circuitikz = %q", got)
	}
	wantCompat := "\\usepackage{a}\n\\usepackage[compatibility]{circuitikz}\n"
	if got := tikzPackageLines([]string{"a", "circuitikz"}, true); got != wantCompat {
		t.Errorf("tikzPackageLines mixed compat = %q, want %q", got, wantCompat)
	}
}

func TestCircuitikzCompatDetected(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{name: "plain circuitikz env", content: "\\begin{circuitikz}\n\\draw (0,0) to[R] (2,0);\n\\end{circuitikz}"},
		{name: "plain tikz", content: "\\draw (0,0) -- (1,1);"},
		{name: "star resistor", content: "\\draw (0,0) to[*R=$R_1$] (1.5,0);", want: true},
		{name: "star transistor", content: "\\draw (0,0) to[*Tnpn] (3,0);", want: true},
		{name: "star with spaces", content: "\\draw (0,0) to [ *D ] (3,2);", want: true},
		{name: "pole dots not star", content: "\\draw (0,0) to[*-*] (2,0);"},
		{name: "pole dot at end", content: "\\draw (0,0) to[R, *-] (2,0);"},
		{name: "explicit compat option", content: "\\usepackage[compatibility]{circuitikzgit}\n\\draw (0,0) to[*R] (2,0);", want: true},
		{name: "explicit compat plain pkg", content: "\\usepackage[compatibility]{circuitikz}\n\\draw (0,0) to[R] (2,0);", want: true},
		{name: "plain usepackage", content: "\\usepackage{circuitikz}\n\\draw (0,0) to[R] (2,0);"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := circuitikzCompatDetected(tt.content); got != tt.want {
				t.Errorf("circuitikzCompatDetected() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCircuitikzIecShim(t *testing.T) {
	if got := circuitikzIecShim("\\draw (0,0) -- (1,1);"); got != "" {
		t.Errorf("circuitikzIecShim(plain) = %q, want empty", got)
	}
	if got := circuitikzIecShim("\\begin{circuitikz}\n\\draw (0,0) to[R] (2,0);\n\\end{circuitikz}"); got != "" {
		t.Errorf("circuitikzIecShim(circuitikz only) = %q, want empty", got)
	}
	got := circuitikzIecShim("\\begin{tikzpicture}[circuit ee IEC]\n\\draw (0,0) to[resistor={name=R}] (0,2);\n\\end{tikzpicture}")
	for _, want := range []string{
		`\pgfkeysifdefined{/tikz/circuit ee IEC}`,
		`circuit ee IEC/.style={}`,
		`resistor/.style={tikzcirciec@resistor={}, #1}`,
		`diode/.style={tikzcirciec@diode={}, #1}`,
		`amperemeter/.style={tikzcirciec@ammeter={}, #1}`,
		`\pgf@circ@emptydiode@path`,
		`tikzcirciec@resistor/.style={\circuitikzbasekey, /tikz/to path=\pgf@circ@bipole@path{generic}, l={#1}}`,
		`tikzcirciec@var resistor/.style={\circuitikzbasekey, /tikz/to path=\pgf@circ@bipole@path{tgeneric}, l={#1}}`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("circuitikzIecShim() missing %q in:\n%s", want, got)
		}
	}
}

func TestTikzShims(t *testing.T) {
	// 同时命中 buzzer / converter / apollonius / IEC / motor 的内容。
	raw := "\\begin{circuitikz}\n\\draw (0,0) to[buzzer] (0,2);\n\\end{circuitikz}\n" +
		"\\begin{tikzpicture}[circuit ee IEC]\n\\node[tacdcshape]{};\n\\end{tikzpicture}\n" +
		"\\tkzDefCircle[apollonius,K=2](A,B)\n\\draw (0,0) to[motor] (2,0);"
	pkgs := []string{"circuitikz", "siunitx", "tkz-euclide"}

	// latexmk (TL2026) profile: 只注入与后端无关的 motor 与 siunitx shim。
	modern := tikzShims(latexmkProfile(), raw, pkgs)
	for _, want := range []string{tikzMotorShim, tikzSiunitxShim} {
		if !strings.Contains(modern, want) {
			t.Errorf("latexmk shims missing backend-independent shim %q", want)
		}
	}
	for _, banned := range []string{tikzBuzzerShim, tikzConverterShim, tikzApolloniusShim, tikzIecShim} {
		if strings.Contains(modern, banned) {
			t.Errorf("latexmk shims must not inject 2021-bundle shim %q", banned)
		}
	}

	// tectonic (2021 bundle) profile: 全部兼容 shim 都要注入。
	legacy := tikzShims(tectonicProfile(), raw, pkgs)
	for _, want := range []string{tikzBuzzerShim, tikzConverterShim, tikzApolloniusShim, tikzIecShim, tikzSiunitxShim} {
		if !strings.Contains(legacy, want) {
			t.Errorf("tectonic shims missing 2021-bundle shim %q", want)
		}
	}
}

// tcblistingWant 拼出 tcblistingSetup 的期望输出: libs 是高亮引擎库列表
// ("listings" 或 "listings,minted"), 其余 \tcbset 条目在 profile 之间不共享,
// 故由本函数统一, 测试只需给出 libs。
func tcblistingWant(libs string) string {
	return `\tcbuselibrary{` + libs + `}
\tcbset{
  slingshot nowrap/.style={before lower*={\centering}, after lower*={}},
  tikz lower,
  sidebyside,
  center lower,
  righthand width=5.7cm,
  sidebyside gap=10pt,
  lower separated=false,
  listing engine=listings,
}
`
}

// TestTcblistingSetup 验证 tcblisting 的 tcolorbox 配置注入条件:
// 必须同时命中 tcblisting 与 tcolorbox 宏包, 否则返回空串; 命中时注入
// 高亮引擎库、tikz lower 与手册同款 sidebyside 左右布局选项。
//
// 库列表按 profile 与内容两个条件拼接: 只有 latexmk (受限 shell escape) 且
// 内容提到 minted 时才追加 minted 库; 其余组合都只加载 listings, 字节里不含
// minted 字样的文档因此与改动前一致。\tcbset 的内容与顺序与两者无关 (含末尾的
// listing engine=listings 默认引擎钉扎)。
func TestTcblistingSetup(t *testing.T) {
	const plainRaw = "\\begin{tcblisting}{title={Basic Ti\\emph{k}Zling}}\n\\marmot\n\\end{tcblisting}"
	const mintedRaw = "\\begin{tcblisting}{listing engine=minted, minted options={linenos}}\n\\duck\n\\end{tcblisting}"
	tests := []struct {
		name    string
		profile tikzProfile
		raw     string
		pkgs    []string
		want    string
	}{
		{
			// 内容不含 minted: 即便 latexmk 支持 minted, 也不得加载, 否则
			// 本来正常的文档会平白依赖 minted/latexminted。
			name:    "tcblisting with tcolorbox (latexmk, no minted)",
			profile: latexmkProfile(),
			raw:     plainRaw,
			pkgs:    []string{"tcolorbox", "tikzlings-marmots"},
			want:    tcblistingWant("listings"),
		},
		{
			// 内容命中 minted 且后端支持: 两个库都加载。
			name:    "tcblisting with tcolorbox (latexmk, minted)",
			profile: latexmkProfile(),
			raw:     mintedRaw,
			pkgs:    []string{"tcolorbox"},
			want:    tcblistingWant("listings,minted"),
		},
		{
			name:    "tcblisting with tcolorbox (tectonic)",
			profile: tectonicProfile(),
			raw:     plainRaw,
			pkgs:    []string{"tcolorbox", "tikzlings-marmots"},
			want:    tcblistingWant("listings"),
		},
		{
			name:    "plain tikz needs nothing",
			profile: latexmkProfile(),
			raw:     "\\draw (0,0) -- (1,1);",
			pkgs:    []string{"tcolorbox"},
		},
		{
			name:    "tcblisting without tcolorbox loaded",
			profile: latexmkProfile(),
			raw:     "\\begin{tcblisting}{}x\\end{tcblisting}",
			pkgs:    []string{"tikz"},
		},
		{
			name:    "snippet tcbset does not suppress defaults",
			profile: latexmkProfile(),
			raw:     "\\tcbset{righthand width=3cm}\n\\begin{tcblisting}{title={Basic}}\n\\duck\n\\end{tcblisting}",
			pkgs:    []string{"tcolorbox"},
			want:    tcblistingWant("listings"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tcblistingSetup(tt.profile, tt.raw, tt.pkgs); got != tt.want {
				t.Errorf("tcblistingSetup() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestTcblistingSetupMintedGatedByProfile 是 minted 加载的门控矩阵:
// 后端能力 (profile.supportsMinted) 与内容命中 (raw 含 "minted") 两个条件
// 缺一不可 —— 唯一加载 `listings,minted` 的组合是 latexmk x 含 minted。
//
// 两个理由共同决定这个矩阵: (1) tectonic 加载 minted 会在导言区直接报
// "You must invoke LaTeX with the -shell-escape flag" 并拖垮所有含 tcblisting
// 的文档; (2) latexmk 上不提 minted 的内容也不该加载 minted 库, 否则本来正常
// 的文档会平白依赖 minted/latexminted (缺环境即导言区失败), 还假设了 TL>=2026。
// listing engine=listings 默认引擎钉扎则四种组合都必须有, 否则既有的 listings
// 盒子会平白要求 pygments。
func TestTcblistingSetupMintedGatedByProfile(t *testing.T) {
	const plainRaw = "\\begin{tcblisting}{}\\duck\\end{tcblisting}"
	const mintedRaw = "\\begin{tcblisting}{listing engine=minted}\\duck\\end{tcblisting}"
	// minted 只出现在 title 里 (无 listing engine): 内容触发刻意"宽进", 只看
	// 子串 —— 这正是当前契约, 用本用例钉住, 防止将来收紧匹配时静默改变行为
	// (误报只是多加载一个库; 漏报会让真正的 minted 片段失败)。
	const titleMintedRaw = "\\begin{tcblisting}{title={About the minted package}}\\duck\\end{tcblisting}"
	pkgs := []string{"tcolorbox"}

	cases := []struct {
		name    string
		profile tikzProfile
		raw     string
		wantLib string
	}{
		{"latexmk/no minted", latexmkProfile(), plainRaw, `\tcbuselibrary{listings}`},
		{"latexmk/minted", latexmkProfile(), mintedRaw, `\tcbuselibrary{listings,minted}`},
		{"latexmk/minted in title only (wide match)", latexmkProfile(), titleMintedRaw, `\tcbuselibrary{listings,minted}`},
		{"tectonic/no minted", tectonicProfile(), plainRaw, `\tcbuselibrary{listings}`},
		{"tectonic/minted", tectonicProfile(), mintedRaw, `\tcbuselibrary{listings}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tcblistingSetup(tc.profile, tc.raw, pkgs)
			if !strings.Contains(got, tc.wantLib) {
				t.Errorf("output must contain %q, got:\n%s", tc.wantLib, got)
			}
			if !strings.Contains(got, "listing engine=listings") {
				t.Errorf("output must pin listing engine=listings, got:\n%s", got)
			}
			// 除了 latexmk x 含 minted, 其余组合**注入的 setup 输出**里不得出现
			// "minted" (片段 raw 里当然可以出现): tectonic 加载 minted 必炸,
			// latexmk 上不含 minted 字样的内容也不该引入 minted/latexminted 依赖。
			wantMinted := tc.profile.supportsMinted && strings.Contains(tc.raw, "minted")
			if gotMinted := strings.Contains(got, ",minted"); gotMinted != wantMinted {
				t.Errorf("output minted library = %v, want %v; got:\n%s", gotMinted, wantMinted, got)
			}
			if !wantMinted && strings.Contains(got, "minted") {
				t.Errorf("non-minted output must not mention minted at all, got:\n%s", got)
			}
		})
	}
}

// TestTcblistingSetupDefinesNoWrapStyle 钉住耦合契约: rewriteSelfContainedTcblistings
// 插入的样式名必须真的在 tcblistingSetup 的输出里被定义, 否则盒子会引用一个
// 不存在的样式 (pgfkeys 报 unknown key / 或静默保留 picture 包裹)。
// 两个 profile 都定义该样式 (与后端无关)。
func TestTcblistingSetupDefinesNoWrapStyle(t *testing.T) {
	want := tcblistingNoWrapStyle + "/.style={before lower*={\\centering}, after lower*={}}"
	for name, profile := range map[string]tikzProfile{
		"latexmk":  latexmkProfile(),
		"tectonic": tectonicProfile(),
	} {
		got := tcblistingSetup(profile, "\\begin{tcblisting}{}\\duck\\end{tcblisting}", []string{"tcolorbox"})
		if !strings.Contains(got, want) {
			t.Fatalf("%s: tcblistingSetup() does not define the style referenced by the rewriter:\n%s", name, got)
		}
	}
}

// TestRewriteSelfContainedTcblistings 验证 tcblisting 盒子正文自包含时,
// 样式名被插到选项参数最前面; 正文一字不动; 不命中/幂等/畸形输入原样放行。
func TestRewriteSelfContainedTcblistings(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			// 用户原始输入 (figchild 的 \fc* 自包含命令)。
			name: "figchild command",
			in:   "\\begin{tcblisting}{title=小鸭台灯}\n  \\fcAbajourA\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{slingshot nowrap, title=小鸭台灯}\n  \\fcAbajourA\n\\end{tcblisting}\n",
		},
		{
			name: "non self-contained duck",
			in:   "\\begin{tcblisting}{title={Basic duck}}\n\\duck\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{title={Basic duck}}\n\\duck\n\\end{tcblisting}\n",
		},
		{
			// 注释里的自包含命令不算数 (stripTikzComments 语义)。
			name: "commented self-contained command",
			in:   "\\begin{tcblisting}{}\n% \\fcBell\n\\draw (0,0) -- (1,1);\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{}\n% \\fcBell\n\\draw (0,0) -- (1,1);\n\\end{tcblisting}\n",
		},
		{
			// contains 语义: 自包含命令 + 裸 tikz 混合仍命中。
			name: "mixed self-contained and raw tikz",
			in:   "\\begin{tcblisting}{}\n\\fcBell\n\\draw (0,0) -- (1,1);\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{slingshot nowrap}\n\\fcBell\n\\draw (0,0) -- (1,1);\n\\end{tcblisting}\n",
		},
		{
			// 双环境各自独立判定: 只改命中者。
			name: "two boxes only one hits",
			in: "\\begin{tcblisting}{title=a}\n\\duck\n\\end{tcblisting}\n" +
				"\\begin{tcblisting}{title=b}\n\\fcBell\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{title=a}\n\\duck\n\\end{tcblisting}\n" +
				"\\begin{tcblisting}{slingshot nowrap, title=b}\n\\fcBell\n\\end{tcblisting}\n",
		},
		{
			// 嵌套花括号 title: 插入位置正确, 其余原样。
			name: "nested braces title",
			in:   "\\begin{tcblisting}{title={Basic Ti\\emph{k}Zing}}\n\\fcBell\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{slingshot nowrap, title={Basic Ti\\emph{k}Zing}}\n\\fcBell\n\\end{tcblisting}\n",
		},
		{
			// 转义花括号不参与配对。
			name: "escaped braces in options",
			in:   "\\begin{tcblisting}{title={a\\{b\\}c}}\n\\fcBell\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{slingshot nowrap, title={a\\{b\\}c}}\n\\fcBell\n\\end{tcblisting}\n",
		},
		{
			// 空选项: 只写样式名, 不留悬空逗号。
			name: "empty options",
			in:   "\\begin{tcblisting}{}\n\\fcBell\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{slingshot nowrap}\n\\fcBell\n\\end{tcblisting}\n",
		},
		{
			// 前导 \tcbset 不是盒子选项, 不动; 盒子正文是裸 tikz 也不命中。
			name: "leading tcbset untouched",
			in:   "\\tcbset{righthand width=3cm}\n\\begin{tcblisting}{title={Basic}}\n\\duck\n\\end{tcblisting}\n",
			want: "\\tcbset{righthand width=3cm}\n\\begin{tcblisting}{title={Basic}}\n\\duck\n\\end{tcblisting}\n",
		},
		{
			// 正文以完整自包含环境开头 (selfContainedStart)。
			name: "body starts with tikzpicture",
			in:   "\\begin{tcblisting}{title=t}\n\\begin{tikzpicture}\n\\draw (0,0) -- (1,1);\n\\end{tikzpicture}\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{slingshot nowrap, title=t}\n\\begin{tikzpicture}\n\\draw (0,0) -- (1,1);\n\\end{tikzpicture}\n\\end{tcblisting}\n",
		},
		{
			name: "body starts with circuitikz",
			in:   "\\begin{tcblisting}{}\n\\begin{circuitikz}\n\\draw (0,0) to[buzzer] (0,2);\n\\end{circuitikz}\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{slingshot nowrap}\n\\begin{circuitikz}\n\\draw (0,0) to[buzzer] (0,2);\n\\end{circuitikz}\n\\end{tcblisting}\n",
		},
		{
			// \node 内嵌 picture 是合法结构, 不能因为"提到 picture"就跳过包裹;
			// 且此处没有自包含命令, 保持包裹。
			name: "picture nested in node keeps wrapper",
			in:   "\\begin{tcblisting}{}\n\\node[inner sep=0]{\\begin{picture}(42,44)\\picduck\\end{picture}};\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{}\n\\node[inner sep=0]{\\begin{picture}(42,44)\\picduck\\end{picture}};\n\\end{tcblisting}\n",
		},
		{
			// 幂等守卫: 选项里已含样式名时原样放行 (不重复插入)。
			name: "already has style",
			in:   "\\begin{tcblisting}{slingshot nowrap, title=t}\n\\fcBell\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{slingshot nowrap, title=t}\n\\fcBell\n\\end{tcblisting}\n",
		},
		{
			name: "no tcblisting at all",
			in:   "\\draw (0,0) -- (1,1);\n",
			want: "\\draw (0,0) -- (1,1);\n",
		},
		{
			// 没有 '{' 选项参数: 原样跳过, 不报错。
			name: "no option argument",
			in:   "\\begin{tcblisting}\n\\fcBell\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}\n\\fcBell\n\\end{tcblisting}\n",
		},
		{
			// 找不到 \end{tcblisting}: 原样跳过, 不报错。
			name: "missing end",
			in:   "\\begin{tcblisting}{title=t}\n\\fcBell\n",
			want: "\\begin{tcblisting}{title=t}\n\\fcBell\n",
		},
		{
			// 未闭合的选项参数: 原样跳过, 不报错。
			name: "unbalanced options",
			in:   "\\begin{tcblisting}{title={a}\n\\fcBell\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{title={a}\n\\fcBell\n\\end{tcblisting}\n",
		},
		{
			// 幂等守卫必须按独立选项比较: 值里出现的同名子串不算已引用。
			name: "style name inside a title value",
			in:   "\\begin{tcblisting}{title={About slingshot nowrap}}\n\\fcBell\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{slingshot nowrap, title={About slingshot nowrap}}\n\\fcBell\n\\end{tcblisting}\n",
		},
		{
			name: "style name first",
			in:   "\\begin{tcblisting}{slingshot nowrap, title=t}\n\\fcBell\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{slingshot nowrap, title=t}\n\\fcBell\n\\end{tcblisting}\n",
		},
		{
			name: "style name middle",
			in:   "\\begin{tcblisting}{title=t, slingshot nowrap, other=x}\n\\fcBell\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{title=t, slingshot nowrap, other=x}\n\\fcBell\n\\end{tcblisting}\n",
		},
		{
			name: "style name last",
			in:   "\\begin{tcblisting}{title=t, slingshot nowrap}\n\\fcBell\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{title=t, slingshot nowrap}\n\\fcBell\n\\end{tcblisting}\n",
		},
		{
			// 选项参数里的注释含不平衡花括号: 注释里的花括号不参与配对,
			// 真实 '}' 才是参数结尾 (旧逻辑会因注释里的 '{' 多计而配不上, 整个环境漏改)。
			name: "comment with unbalanced braces in options",
			in:   "\\begin{tcblisting}{title=x % { { } unbalanced\n}\n\\fcBell\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{slingshot nowrap, title=x % { { } unbalanced\n}\n\\fcBell\n\\end{tcblisting}\n",
		},
		{
			// 注释里的 \begin{tcblisting} 不是环境起点: 只应改写真实环境。
			name: "commented begin marker",
			in:   "% \\begin{tcblisting}{}\n\\begin{tcblisting}{title=t}\n\\fcBell\n\\end{tcblisting}\n",
			want: "% \\begin{tcblisting}{}\n\\begin{tcblisting}{slingshot nowrap, title=t}\n\\fcBell\n\\end{tcblisting}\n",
		},
		{
			// 花括号深度感知: 花括号值里的逗号不切分, 值内的样式名不算引用。
			name: "style name inside a braced value list",
			in:   "\\begin{tcblisting}{title={a,slingshot nowrap,b}}\n\\fcBell\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{slingshot nowrap, title={a,slingshot nowrap,b}}\n\\fcBell\n\\end{tcblisting}\n",
		},
		{
			// pgfkeys 里 {name} 与 name 等价: 整体被花括号包住的样式项也算引用。
			name: "braced style name first",
			in:   "\\begin{tcblisting}{{slingshot nowrap}, title=t}\n\\fcBell\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{{slingshot nowrap}, title=t}\n\\fcBell\n\\end{tcblisting}\n",
		},
		{
			name: "braced style name middle",
			in:   "\\begin{tcblisting}{title=t, {slingshot nowrap}, other=x}\n\\fcBell\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{title=t, {slingshot nowrap}, other=x}\n\\fcBell\n\\end{tcblisting}\n",
		},
		{
			name: "braced style name last",
			in:   "\\begin{tcblisting}{title=t, {slingshot nowrap}}\n\\fcBell\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{title=t, {slingshot nowrap}}\n\\fcBell\n\\end{tcblisting}\n",
		},
		{
			// 注释里的 \end 不是环境结尾: 正文继续到真实 \end, 注释原样保留。
			name: "commented end marker",
			in:   "\\begin{tcblisting}{}\n% \\end{tcblisting}\n\\fcBell\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{slingshot nowrap}\n% \\end{tcblisting}\n\\fcBell\n\\end{tcblisting}\n",
		},
		{
			// 注释里的逗号不切分 (splitPgfKeysOptions 与其它扫描 helper 同一规则):
			// 值不会被切出恰等于样式名的片段, 因此仍应注入, 且注释原样保留。
			name: "comment hides the style name",
			in:   "\\begin{tcblisting}{title=t % ,slingshot nowrap,\n}\n\\fcBell\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{slingshot nowrap, title=t % ,slingshot nowrap,\n}\n\\fcBell\n\\end{tcblisting}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rewriteSelfContainedTcblistings(tt.in); got != tt.want {
				t.Errorf("rewriteSelfContainedTcblistings() = %q, want %q", got, tt.want)
			}
			// 幂等: 二次改写与首次结果一致。
			if got := rewriteSelfContainedTcblistings(tt.want); got != tt.want {
				t.Errorf("rewriteSelfContainedTcblistings() not idempotent: %q, want %q", got, tt.want)
			}
		})
	}
}

// TestIsCommentFamilyTcblisting 验证注释族布局的识别: 顶层条目与样式名全等才命中。
func TestIsCommentFamilyTcblisting(t *testing.T) {
	// 正向: 名单里每个样式名都命中 (含父样式与 sidebyside 别名)。
	for _, name := range []string{
		"comment only",
		"comment and listing",
		"comment side listing",
		"listing and comment",
		"listing side comment",
		"comment above listing",
		"comment above* listing",
		"listing above comment",
		"listing above* comment",
		"comment outside listing",
		"listing outside comment",
	} {
		t.Run("forward/"+name, func(t *testing.T) {
			// 单条、前置、后置与花括号包裹等价形式都应命中。
			for _, opts := range []string{
				name,
				name + ", righthand ratio=0.45",
				"title=t, " + name,
				"{" + name + "}",
			} {
				if !isCommentFamilyTcblisting(opts) {
					t.Errorf("isCommentFamilyTcblisting(%q) = false, want true", opts)
				}
			}
		})
	}

	reverse := []struct {
		name string
		opts string
	}{
		// 不动的 text 家族: 没有把注释塞进 lower 槽, 保持 tikz lower 行为。
		{name: "listing side text", opts: "listing side text, righthand ratio=0.45"},
		{name: "text side listing", opts: "text side listing"},
		{name: "text only", opts: "text only"},
		{name: "listing only", opts: "listing only"},
		// 裸 comment={...}: 默认布局仍是 listing and text, 不属于注释族布局。
		{name: "bare comment option", opts: "comment={a\\b}, listing engine=minted"},
		// 值里出现同样文字: 不是布局引用 (子串匹配会误命中)。
		{name: "style name inside a title value", opts: "title={listing side comment}"},
		// 注释值里出现样式名 (comment={listing side comment}): comment= 是 key,
		// 值是散文注释, 不是布局引用, 不得触发 nowrap。
		{name: "style name inside a comment value", opts: "comment={listing side comment}"},
		{name: "style name inside a braced value list", opts: "title={a,listing side comment,b}"},
		// 注释里的样式名不算数 (splitPgfKeysOptions 跳过未转义 % 至行尾)。
		{name: "style name inside a comment", opts: "title=t % listing side comment\n"},
		// 前缀/后缀不同: 必须全等。
		{name: "prefix only", opts: "listing side comment extra"},
		{name: "case sensitive", opts: "Listing Side Comment"},
		{name: "empty", opts: ""},
		{name: "unrelated options", opts: "tikz lower, sidebyside, listing engine=listings"},
	}
	for _, tt := range reverse {
		t.Run("reverse/"+tt.name, func(t *testing.T) {
			if isCommentFamilyTcblisting(tt.opts) {
				t.Errorf("isCommentFamilyTcblisting(%q) = true, want false", tt.opts)
			}
		})
	}
}

// TestRewriteTcblistingCommentFamily 验证注释族布局的盒子会像自包含正文一样
// 在选项参数最前面插入 tcblistingNoWrapStyle (并且只改选项、正文一字不动)。
func TestRewriteTcblistingCommentFamily(t *testing.T) {
	// 正向: 名单里每个样式名都触发注入, 且样式名插在用户选项之前。
	for _, name := range []string{
		"comment only",
		"comment and listing",
		"comment side listing",
		"listing and comment",
		"listing side comment",
		"comment above listing",
		"comment above* listing",
		"listing above comment",
		"listing above* comment",
		"comment outside listing",
		"listing outside comment",
	} {
		t.Run("forward/"+name, func(t *testing.T) {
			in := "\\begin{tcblisting}{\n  " + name + ",\n  comment={note},\n}\ncode\n\\end{tcblisting}\n"
			want := "\\begin{tcblisting}{" + tcblistingNoWrapStyle + ", \n  " + name + ",\n  comment={note},\n}\ncode\n\\end{tcblisting}\n"
			if got := rewriteSelfContainedTcblistings(in); got != want {
				t.Errorf("rewriteSelfContainedTcblistings() = %q, want %q", got, want)
			}
		})
	}

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			// 不动的 text 家族: 选项原样, 不注入。
			name: "listing side text keeps wrapper",
			in:   "\\begin{tcblisting}{listing side text}\n\\duck\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{listing side text}\n\\duck\n\\end{tcblisting}\n",
		},
		{
			name: "text only keeps wrapper",
			in:   "\\begin{tcblisting}{text only}\ntext\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{text only}\ntext\n\\end{tcblisting}\n",
		},
		{
			// 裸 comment={...} (默认布局 = listing and text) 不动。
			name: "bare comment keeps wrapper",
			in:   "\\begin{tcblisting}{comment={a\\\\b}}\ncode\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{comment={a\\\\b}}\ncode\n\\end{tcblisting}\n",
		},
		{
			// 值里出现样式名: 不是布局引用, 不注入。
			name: "style name inside a title value",
			in:   "\\begin{tcblisting}{title={listing side comment}}\ncode\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{title={listing side comment}}\ncode\n\\end{tcblisting}\n",
		},
		{
			// comment={listing side comment}: 样式名只是注释值, 不注入。
			name: "style name inside a comment value",
			in:   "\\begin{tcblisting}{comment={listing side comment}}\ncode\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{comment={listing side comment}}\ncode\n\\end{tcblisting}\n",
		},
		{
			// 注释里出现样式名: 不算引用, 不注入。
			name: "style name inside a comment",
			in:   "\\begin{tcblisting}{title=t % listing side comment\n}\ncode\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{title=t % listing side comment\n}\ncode\n\\end{tcblisting}\n",
		},
		{
			// 无选项参数: 原样跳过。
			name: "no option argument",
			in:   "\\begin{tcblisting}\ncode\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}\ncode\n\\end{tcblisting}\n",
		},
		{
			// 找不到 \\end{tcblisting}: 原样跳过。
			name: "missing end",
			in:   "\\begin{tcblisting}{listing side comment}\ncode\n",
			want: "\\begin{tcblisting}{listing side comment}\ncode\n",
		},
		{
			// 幂等守卫: 已含样式名时原样放行。
			name: "already has style",
			in:   "\\begin{tcblisting}{slingshot nowrap, listing side comment}\ncode\n\\end{tcblisting}\n",
			want: "\\begin{tcblisting}{slingshot nowrap, listing side comment}\ncode\n\\end{tcblisting}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rewriteSelfContainedTcblistings(tt.in); got != tt.want {
				t.Errorf("rewriteSelfContainedTcblistings() = %q, want %q", got, tt.want)
			}
			// 幂等: 二次改写与首次结果一致。
			if got := rewriteSelfContainedTcblistings(tt.want); got != tt.want {
				t.Errorf("rewriteSelfContainedTcblistings() not idempotent: %q, want %q", got, tt.want)
			}
		})
	}
}

// TestHasNoWrapStyle 验证幂等守卫按 pgfkeys 逗号分隔的独立选项全等比较,
// 而不是子串匹配: 值里出现的同样文字不算引用。
func TestHasNoWrapStyle(t *testing.T) {
	tests := []struct {
		name string
		opts string
		want bool
	}{
		{name: "empty", opts: "", want: false},
		{name: "first", opts: "slingshot nowrap, title=t", want: true},
		{name: "middle", opts: "title=t, slingshot nowrap, other=x", want: true},
		{name: "last", opts: "title=t, slingshot nowrap", want: true},
		{name: "surrounding space", opts: "  slingshot nowrap  ", want: true},
		{name: "only in a value", opts: "title={About slingshot nowrap}", want: false},
		{name: "value with trailing text", opts: "title=t, note=slingshot nowrap", want: false},
		{name: "prefix only", opts: "slingshot nowrap extra", want: false},
		{name: "case sensitive", opts: "Slingshot Nowrap", want: false},
		{name: "other style", opts: "tikz lower, sidebyside", want: false},
		// 花括号深度感知: 值里的逗号不切分, 值里的样式名不算引用 (有害方向)。
		{name: "style name inside braced value list", opts: "title={a,slingshot nowrap,b}", want: false},
		{name: "value with comma and keyword", opts: "title={x, slingshot nowrap, y}, other=z", want: false},
		// pgfkeys 里 {name} 与 name 等价。
		{name: "braced style name first", opts: "{slingshot nowrap}, title=t", want: true},
		{name: "braced style name middle", opts: "title=t, {slingshot nowrap}, other=x", want: true},
		{name: "braced style name last", opts: "title=t, {slingshot nowrap}", want: true},
		{name: "nested braces style name", opts: "title=t, {{slingshot nowrap}}", want: true},
		// 注释里的逗号不切分: 值不会被切出恰等于样式名的片段。
		{name: "style name inside a comment", opts: "title=t % ,slingshot nowrap,\n", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasNoWrapStyle(tt.opts); got != tt.want {
				t.Errorf("hasNoWrapStyle(%q) = %v, want %v", tt.opts, got, tt.want)
			}
		})
	}
}

// TestNextMarkerOutsideComment 验证标记定位跳过 TeX 注释区:
// 注释里的伪标记不算数, 转义 % 不是注释起点, 且反斜杠游程的回看要跨越 from 边界。
func TestNextMarkerOutsideComment(t *testing.T) {
	const begin = `\begin{tcblisting}`
	tests := []struct {
		name string
		s    string
		sub  string
		from int
		want int
	}{
		{name: "plain", s: "x" + begin + "y", sub: begin, from: 0, want: 1},
		{
			// 评审场景: 注释里的 begin 被跳过, 返回真实 begin 的下标。
			name: "marker inside comment skipped",
			s:    "% " + begin + "{}\n" + begin + "{}",
			sub:  begin, from: 0, want: 23,
		},
		{
			// \% 是转义百分号, 不是注释起点: begin 紧跟其后, 下标 2。
			name: "escaped percent is not a comment",
			s:    `\%` + begin,
			sub:  begin, from: 0, want: 2,
		},
		{
			// \\% 是注释起点 (两个反斜杠, 偶数): 整个余下内容都在注释里,
			// 因此找不到标记。反斜杠游程从下标 0 起、from=1, 回看必须跨越
			// from 边界才能数出 2 个 (只看 from 之后会误判为 1 个=转义)。
			name: "even backslashes cross from boundary",
			s:    `\\%` + begin,
			sub:  begin, from: 1, want: -1,
		},
		{name: "not found", s: "nothing here", sub: begin, from: 0, want: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextMarkerOutsideComment(tt.s, tt.sub, tt.from); got != tt.want {
				t.Errorf("nextMarkerOutsideComment(%q, %q, %d) = %d, want %d",
					tt.s, tt.sub, tt.from, got, tt.want)
			}
		})
	}
}

// TestSplitPgfKeysOptions 验证选项串按 pgfkeys 规则切分: 只在花括号深度为 0
// 的逗号处分割; 花括号内的逗号不切。
func TestSplitPgfKeysOptions(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{name: "empty", in: "", want: []string{""}},
		{name: "single", in: "tikz lower", want: []string{"tikz lower"}},
		{name: "two", in: "a, b", want: []string{"a", " b"}},
		{name: "comma in braces", in: "title={a,b}, c", want: []string{"title={a,b}", " c"}},
		{name: "nested braces", in: "title={a,{b,c},d},e", want: []string{"title={a,{b,c},d}", "e"}},
		// \{ 是转义花括号, 不抬升深度: 其中的逗号仍是分隔符。
		{name: "escaped brace", in: `title=\{a,b\}, c`, want: []string{`title=\{a`, `b\}`, " c"}},
		{name: "unbalanced merges", in: "title={a,b, c", want: []string{"title={a,b, c"}},
		// 注释里的逗号不是分隔符: % 至行尾的 ",slingshot nowrap," 整段留在同一选项里。
		{name: "comma inside comment", in: "title=t % ,slingshot nowrap,\n", want: []string{"title=t % ,slingshot nowrap,\n"}},
		// 注释吞掉逗号后不再产生分段 (修复前会切成 ["a","%b","c\nd"]); 段内保留
		// 原注释字节 (只跳过、不删, 位置不变), 因此第二段含注释文本。
		{name: "comment swallows comma", in: "a,%b,c\nd", want: []string{"a", "%b,c\nd"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitPgfKeysOptions(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("splitPgfKeysOptions(%q) = %q, want %q", tt.in, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("splitPgfKeysOptions(%q) = %q, want %q", tt.in, got, tt.want)
				}
			}
		})
	}
}

// TestEscapedPercent 钉住共享谓词 escapedPercent 的契约: % 前的连续反斜杠数为
// 奇数表示被转义 (\%), 偶数 (含 0, 如 \\%) 表示注释起点; 下标越界或 s[i] 不是
// '%' 时返回保守的 false (防御性守卫)。
func TestEscapedPercent(t *testing.T) {
	tests := []struct {
		name string
		s    string
		i    int
		want bool
	}{
		{name: "bare percent", s: "%", i: 0, want: false},
		{name: "escaped once", s: `\%`, i: 1, want: true},
		{name: "escaped twice", s: `\\%`, i: 2, want: false},
		{name: "escaped thrice", s: `\\\%`, i: 3, want: true},
		{name: "out of range", s: "%", i: 1, want: false},
		{name: "not a percent", s: "abc", i: 0, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapedPercent(tt.s, tt.i); got != tt.want {
				t.Errorf("escapedPercent(%q, %d) = %v, want %v", tt.s, tt.i, got, tt.want)
			}
		})
	}
}

// TestTrimOptionBraces 验证剥掉选项段最外层等价的成对花括号。
func TestTrimOptionBraces(t *testing.T) {
	tests := []struct{ in, want string }{
		{in: "slingshot nowrap", want: "slingshot nowrap"},
		{in: "{slingshot nowrap}", want: "slingshot nowrap"},
		{in: "{{slingshot nowrap}}", want: "slingshot nowrap"},
		{in: "{ slingshot nowrap }", want: "slingshot nowrap"},
		{in: "{a} extra", want: "{a} extra"}, // 配对 '}' 不在段尾, 不剥
		{in: "{a", want: "{a"},
		{in: "", want: ""},
	}
	for _, tt := range tests {
		if got := trimOptionBraces(tt.in); got != tt.want {
			t.Errorf("trimOptionBraces(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestMatchBalancedBrace 验证选项参数的花括号配对扫描。
func TestMatchBalancedBrace(t *testing.T) {
	tests := []struct {
		in   string
		want string // 配对 '}' 之后的剩余部分; "!" 表示应失败
	}{
		{in: "{}rest", want: "rest"},
		{in: "{a{b}c}rest", want: "rest"},
		{in: `{a\{b\}c}rest`, want: "rest"},
		{in: "{a", want: "!"},
		{in: "x{}", want: "!"}, // 起点不是 '{'
		{in: "", want: "!"},
		// 注释里的花括号不参与配对: 只跳过、位置不变, 真实 '}' 才是结尾。
		{in: "{a % { { } comment\n}rest", want: "rest"},
		{in: "{a % } comment\n}rest", want: "rest"},
		// \% 是转义百分号, 不是注释起点: 它后面的 '}' 正常关闭花括号。
		{in: `{a \% b}rest`, want: "rest"},
	}
	for _, tt := range tests {
		i, ok := matchBalancedBrace(tt.in, 0)
		if tt.want == "!" {
			if ok {
				t.Errorf("matchBalancedBrace(%q, 0) = %d, true, want failure", tt.in, i)
			}
			continue
		}
		if !ok {
			t.Errorf("matchBalancedBrace(%q, 0) failed, want success", tt.in)
			continue
		}
		if got := tt.in[i+1:]; got != tt.want {
			t.Errorf("matchBalancedBrace(%q, 0) tail = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestContainsCommand 验证命令匹配的词边界: \bear 不能命中 \bearwear,
// 但 \bear[ / \bear} 等分隔符都要命中。
func TestContainsCommand(t *testing.T) {
	tests := []struct {
		content string
		cmd     string
		want    bool
	}{
		{content: "\\bear[hat]", cmd: "\\bear", want: true},
		{content: "\\bear", cmd: "\\bear", want: true},
		{content: "\\bear;", cmd: "\\bear", want: true},
		{content: "\\bearwear", cmd: "\\bear", want: false},
		{content: "\\marmotx \\marmot", cmd: "\\marmot", want: true},
		{content: "\\marmotx", cmd: "\\marmot", want: false},
		{content: "\\bear", cmd: "\\marmot", want: false},
		{content: "\\thing@hat", cmd: "\\thing", want: false},
	}
	for _, tt := range tests {
		if got := containsCommand(tt.content, tt.cmd); got != tt.want {
			t.Errorf("containsCommand(%q, %q) = %v, want %v", tt.content, tt.cmd, got, tt.want)
		}
	}
}

// TestTexLogErrNil 钉住 texLogErr 的契约: nil 错误原样返回 nil,
// 不产生 "%!w(<nil>)" 之类的伪错误。
func TestTexLogErrNil(t *testing.T) {
	if err := texLogErr(nil, t.TempDir()); err != nil {
		t.Errorf("texLogErr(nil) = %v, want nil", err)
	}
}

func TestCircuitikzConverterShim(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string // "" = no shim
	}{
		{name: "no converter", content: "\\begin{circuitikz}\n\\draw (0,0) to[R] (2,0);\n\\end{circuitikz}"},
		{name: "plain tikz", content: "\\draw (0,0) -- (1,1);"},
		{name: "tacdcshape node", content: "\\node[tacdcshape, anchor=ac mid in](acdc){};", want: tikzConverterShim + "\n"},
		{name: "tdcacshape node", content: "\\draw (0,0) node[tdcacshape, anchor=dc up in](dcac){};", want: tikzConverterShim + "\n"},
		{name: "anchor access only", content: "\\draw (0,0) -- (acdc.ac up in);"},
		{name: "commented out usage ignored", content: "% \\node[tacdcshape, anchor=ac mid in](acdc){};\n\\draw (0,0) to[R] (2,0);"},
		{name: "inline comment after real usage still triggers", content: "\\draw (0,0) node[tacdcshape]{}; % converter", want: tikzConverterShim + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := circuitikzConverterShim(tt.content); got != tt.want {
				t.Errorf("circuitikzConverterShim() = %q, want %q", got, tt.want)
			}
		})
	}
	got := circuitikzConverterShim("\\draw (0,0) node[tacdcshape, anchor=ac mid in](acdc){} to[smallR] ++(-2,0);")
	for _, want := range []string{
		`\ifcsname pgf@sh@s@tacdcshape\endcsname`,
		`\expandafter\gdef\csname pgf@anchor@tacdcshape@ac mid in\endcsname{\northeast\pgf@y=0\pgf@y\pgf@x=-\pgf@x}`,
		`\expandafter\gdef\csname pgf@anchor@tacdcshape@ac up in\endcsname{\northeast\pgf@y=.6\pgf@y\pgf@x=-\pgf@x}`,
		`\expandafter\gdef\csname pgf@anchor@tacdcshape@dc up out\endcsname{\northeast\pgf@y=.4\pgf@y}`,
		`\expandafter\gdef\csname pgf@anchor@tdcacshape@dc up in\endcsname{\northeast\pgf@y=.4\pgf@y\pgf@x=-\pgf@x}`,
		`\expandafter\gdef\csname pgf@anchor@tdcacshape@ac mid out\endcsname{\northeast\pgf@y=0\pgf@y}`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("circuitikzConverterShim() missing %q in:\n%s", want, got)
		}
	}
}

// TestVendoredPackageFiles 验证 vendored 资产只在 tectonic profile 且命中
// 对应包时才写入工作目录 (xe 用系统 TeX Live, 永远为空)。
func TestVendoredPackageFiles(t *testing.T) {
	tests := []struct {
		name string
		p    tikzProfile
		pkgs []string
		want []string
	}{
		{
			name: "latexmk needs no vendored files",
			p:    latexmkProfile(),
			pkgs: []string{"figchild", "tikz-triminos"},
		},
		{
			name: "tectonic figchild only",
			p:    tectonicProfile(),
			pkgs: []string{"figchild"},
			want: []string{"figchild.sty"},
		},
		{
			name: "tectonic both, figchild first",
			p:    tectonicProfile(),
			pkgs: []string{"figchild", "tikz-triminos"},
			want: []string{"figchild.sty", "tikz-triminos.sty"},
		},
		{
			name: "tectonic with no matching package",
			p:    tectonicProfile(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := vendoredPackageFiles(tt.p, tt.pkgs)
			if len(got) != len(tt.want) {
				t.Fatalf("vendoredPackageFiles() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("vendoredPackageFiles() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

// TestTriminosFpevalShim 验证 fpeval shim 只在包列表含 tikz-triminos 时产出,
// 且内容用 \ifcsname 守卫 (未来 bundle 自带 \fpeval 时不重复定义)。
func TestTriminosFpevalShim(t *testing.T) {
	if got := triminosFpevalShim([]string{"tikz-triminos"}); got == "" {
		t.Fatal("triminosFpevalShim(tikz-triminos) = empty, want the fpeval shim")
	} else if !strings.Contains(got, `\ifcsname fpeval`) {
		t.Errorf("triminosFpevalShim() = %q, want it to guard on \\ifcsname fpeval", got)
	}
	if got := triminosFpevalShim([]string{"tikz-euclide"}); got != "" {
		t.Errorf("triminosFpevalShim(without tikz-triminos) = %q, want empty", got)
	}
	if got := triminosFpevalShim(nil); got != "" {
		t.Errorf("triminosFpevalShim(nil) = %q, want empty", got)
	}
}

// TestTikzAssetsEmbedded 验证两份 vendored .sty 都嵌进了二进制,
// 且确实是对应宏包 (只扫前若干 KB, figchild.sty 有 1.8MB)。
func TestTikzAssetsEmbedded(t *testing.T) {
	// 大小下限断言防止资产被意外截断 / 替换成占位文件。
	for name, tc := range map[string]struct {
		want     string
		minBytes int
	}{
		"figchild.sty":      {`\ProvidesPackage{figchild}`, 1_000_000},
		"tikz-triminos.sty": {`\ProvidesPackage{tikz-triminos}`, 4096},
	} {
		data, err := tikzAssets.ReadFile("tikzassets/" + name)
		if err != nil {
			t.Fatalf("reading embedded %s: %v", name, err)
		}
		if len(data) < tc.minBytes {
			t.Errorf("embedded %s is %d bytes, want >= %d", name, len(data), tc.minBytes)
		}
		head := data
		if len(head) > 8192 {
			head = head[:8192]
		}
		if !strings.Contains(string(head), tc.want) {
			t.Errorf("embedded %s does not contain %q in its first 8KB", name, tc.want)
		}
	}
}

// TestStripTikzComments 验证注释剥离: 未转义 % 删到行尾 (保留换行),
// 转义 \% 不算注释; % 前连续反斜杠数为奇数才转义。
func TestStripTikzComments(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain comment", input: "a%c\nb", want: "a\nb"},
		{name: "escaped percent untouched", input: "a\\%b", want: "a\\%b"},
		{name: "double backslash is a comment start", input: "a\\\\%b\nc", want: "a\\\\\nc"},
		{name: "comment at end without newline", input: "a%c", want: "a"},
		{name: "no percent", input: "a\\b\nc", want: "a\\b\nc"},
		{name: "comment at line start", input: "%c\nb", want: "\nb"},
		{name: "three backslashes then percent is escaped", input: "a\\\\\\%b", want: "a\\\\\\%b"},
		{name: "crlf comment drops cr too", input: "a%c\r\nb", want: "a\nb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripTikzComments(tt.input); got != tt.want {
				t.Errorf("stripTikzComments(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestWriteVendoredPackages 验证 tectonic profile 会把命中的 vendored 宏包
// 写进工作目录 (内容以 \ProvidesPackage 开头), latexmk profile 不写任何文件。
func TestWriteVendoredPackages(t *testing.T) {
	dir := t.TempDir()
	pkgs := []string{"figchild", "tikz-triminos"}
	if err := writeVendoredPackages(dir, tectonicProfile(), pkgs); err != nil {
		t.Fatalf("writeVendoredPackages(tectonic) = %v", err)
	}
	for name, want := range map[string]string{
		"figchild.sty":      `\ProvidesPackage{figchild}`,
		"tikz-triminos.sty": `\ProvidesPackage{tikz-triminos}`,
	} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("reading written %s: %v", name, err)
		}
		if len(data) == 0 {
			t.Errorf("written %s is empty", name)
		}
		if !strings.Contains(string(data), want) {
			t.Errorf("written %s does not contain %q", name, want)
		}
	}

	dir2 := t.TempDir()
	if err := writeVendoredPackages(dir2, latexmkProfile(), pkgs); err != nil {
		t.Fatalf("writeVendoredPackages(latexmk) = %v", err)
	}
	entries, err := os.ReadDir(dir2)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("latexmk profile wrote %d files, want 0", len(entries))
	}
}

// fcNameRe 从 figchild.sty 提取 \fc* 命令名, 容忍 \newcommand* 与可选花括号。
var fcNameRe = regexp.MustCompile(`\\newcommand\*?\s*\{?\\?(fc[A-Za-z0-9]*)`)

// fcCamelRe 是 figchildRe 编码的 CamelCase 形式 (fc + 大写)。
var fcCamelRe = regexp.MustCompile(`^fc[A-Z]`)

// TestFigchildLowercaseExceptions 扫描嵌入的 figchild.sty, 断言 \fc* 命令中
// 不匹配 ^fc[A-Z] 的小写例外恰为 figchildRe 硬编码的 5 个名字; 且这些名字
// 都被 figchildRe 命中, 而内核 \fcolorbox 不被命中 (正则与上游名单一致)。
func TestFigchildLowercaseExceptions(t *testing.T) {
	data, err := tikzAssets.ReadFile("tikzassets/figchild.sty")
	if err != nil {
		t.Fatal(err)
	}
	var lower []string
	seen := make(map[string]bool)
	for _, m := range fcNameRe.FindAllStringSubmatch(string(data), -1) {
		n := m[1]
		if !seen[n] {
			seen[n] = true
			if !fcCamelRe.MatchString(n) {
				lower = append(lower, n)
			}
		}
	}
	slices.Sort(lower)
	want := []string{"fcfrog", "fchamburger", "fcpink", "fcsheetA", "fcsheetB"}
	if !slices.Equal(lower, want) {
		t.Fatalf("lowercase fc exceptions = %v, want %v", lower, want)
	}
	for _, n := range want {
		if !figchildRe.MatchString(`\` + n) {
			t.Errorf("figchildRe does not match \\%s", n)
		}
	}
	if figchildRe.MatchString(`\fcolorbox`) {
		t.Error("figchildRe matches kernel \\fcolorbox, want no match")
	}
}

// TestTikzAssetsSHA256 把 tikzassets/README.md 记录的 SHA-256 变成测试不变量:
// 从 README 提取全部 64 位 hex token, 断言两份嵌入 .sty 的 sha256 都在其中
// (README 表与资产由测试互相锁定, 刷新资产时必须同步哈希)。
func TestTikzAssetsSHA256(t *testing.T) {
	readme, err := os.ReadFile("tikzassets/README.md")
	if err != nil {
		t.Fatal(err)
	}
	// 逐文件映射: 解析 README 表行的 `name.sty` ... `sha256`, 断言每份
	// 嵌入 .sty 的 sha256 与该行记录的哈希一致 (防复制粘贴时把两个哈希互换)。
	rowRe := regexp.MustCompile("`([\\w-]+\\.sty)`[^\\n]*?`([0-9a-f]{64})`")
	documented := make(map[string]string)
	for _, m := range rowRe.FindAllStringSubmatch(string(readme), -1) {
		documented[m[1]] = m[2]
	}
	if len(documented) == 0 {
		t.Fatal("no .sty sha256 rows parsed from tikzassets/README.md")
	}
	// 以嵌入资产为准逐一遍历 (而非遍历解析结果): 若某个 .sty 的行没被解析
	// 到, 这里会因查不到 documented 而失败, 避免"静默跳过一行"仍通过。
	entries, err := tikzAssets.ReadDir("tikzassets")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || filepath.Ext(name) != ".sty" {
			continue
		}
		checked++
		want, ok := documented[name]
		if !ok {
			t.Errorf("embedded %s has no sha256 row in tikzassets/README.md", name)
			continue
		}
		data, err := tikzAssets.ReadFile("tikzassets/" + name)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		got := hex.EncodeToString(sum[:])
		if got != want {
			t.Errorf("sha256(%s) = %s, README records %s", name, got, want)
		}
	}
	if checked != len(documented) {
		t.Errorf("checked %d embedded .sty, README documents %d", checked, len(documented))
	}
}
