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
