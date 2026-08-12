package main

import (
	"os"
	"path/filepath"
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeTikz(tt.input); got != tt.want {
				t.Errorf("normalizeTikz() = %q, want %q", got, tt.want)
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

func TestTikzPackageLines(t *testing.T) {
	if got := tikzPackageLines(nil); got != "" {
		t.Errorf("tikzPackageLines(nil) = %q, want empty", got)
	}
	if got := tikzPackageLines([]string{"tkz-euclide"}); got != "\\usepackage{tkz-euclide}\n" {
		t.Errorf("tikzPackageLines single = %q", got)
	}
	want := "\\usepackage{a}\n\\usepackage{b}\n"
	if got := tikzPackageLines([]string{"a", "b"}); got != want {
		t.Errorf("tikzPackageLines multiple = %q, want %q", got, want)
	}
}
