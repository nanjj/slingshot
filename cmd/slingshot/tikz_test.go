package main

import (
	"os"
	"path/filepath"
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeTikz(tt.input); got != tt.want {
				t.Errorf("normalizeTikz() = %q, want %q", got, tt.want)
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectTikzPackages(tt.content)
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
