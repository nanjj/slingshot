package main

import (
	"reflect"
	"testing"
)

func TestExtractCommentedPackages(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
		rest    string
	}{
		{
			name:    "no comment",
			content: "\\begin{tikzpicture}\n\\draw (0,0);\n\\end{tikzpicture}\n",
			want:    nil,
			rest:    "\\begin{tikzpicture}\n\\draw (0,0);\n\\end{tikzpicture}\n",
		},
		{
			name:    "commented usepackage before picture",
			content: "%\\usepackage{physketch.general}\n\\begin{tikzpicture}\n\\draw (0,0);\n\\end{tikzpicture}\n",
			want:    []string{"physketch.general"},
			rest:    "\\begin{tikzpicture}\n\\draw (0,0);\n\\end{tikzpicture}\n",
		},
		{
			name:    "space after percent is not a directive",
			content: "% \\usepackage{physketch.general}\n\\begin{tikzpicture}\n\\draw (0,0);\n\\end{tikzpicture}\n",
			want:    nil,
			rest:    "% \\usepackage{physketch.general}\n\\begin{tikzpicture}\n\\draw (0,0);\n\\end{tikzpicture}\n",
		},
		{
			name:    "commented usepackage inside picture is left alone",
			content: "\\begin{tikzpicture}\n%\\usepackage{physketch.general}\n\\draw (0,0);\n\\end{tikzpicture}\n",
			want:    nil,
			rest:    "\\begin{tikzpicture}\n%\\usepackage{physketch.general}\n\\draw (0,0);\n\\end{tikzpicture}\n",
		},
		{
			name:    "comma separated",
			content: "%\\usepackage{physketch.general,amsmath}\n\\begin{tikzpicture}\n\\end{tikzpicture}\n",
			want:    []string{"physketch.general", "amsmath"},
			rest:    "\\begin{tikzpicture}\n\\end{tikzpicture}\n",
		},
		{
			name:    "optional argument and trailing spaces",
			content: "%\\usepackage[svgnames]{xcolor}  \n\\begin{tikzpicture}\n\\end{tikzpicture}\n",
			want:    []string{"xcolor"},
			rest:    "\\begin{tikzpicture}\n\\end{tikzpicture}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, rest := extractCommentedPackages(tt.content)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("packages = %v, want %v", got, tt.want)
			}
			if rest != tt.rest {
				t.Errorf("rest = %q, want %q", rest, tt.rest)
			}
		})
	}
}

func TestUserModePackageName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"physketch.general", "physketch"},
		{"amsmath", "amsmath"},
		{"a.b.c", "a"},
		{".hidden", ".hidden"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := userModePackageName(tt.in); got != tt.want {
			t.Errorf("userModePackageName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
