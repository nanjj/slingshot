package mdtowx

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)


func TestParseOrgKeywords(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantTitle string
		wantAuthor string
		wantThumb string
		wantDigest string
	}{
		{
			name:     "empty",
			input:    "",
			wantTitle: "",
		},
		{
			name: "title_and_author",
			input: `#+TITLE: My Article
#+AUTHOR: John Doe
#+OPTIONS: toc:nil

Content here.
`,
			wantTitle:  "My Article",
			wantAuthor: "John Doe",
		},
		{
			name: "all_keywords",
			input: `#+TITLE: Full Test
#+AUTHOR: Jane
#+THUMB_MEDIA_ID: abc123
#+DIGEST: A great article

Content.
`,
			wantTitle:  "Full Test",
			wantAuthor: "Jane",
			wantThumb:  "abc123",
			wantDigest: "A great article",
		},
		{
			name: "case_insensitive",
			input: `#+title: lowercase
#+Author: Mixed Case
`,
			wantTitle:  "lowercase",
			wantAuthor: "Mixed Case",
		},
		{
			name: "only_first_value_used",
			input: `#+TITLE: First
#+TITLE: Second
`,
			wantTitle: "First",
		},
		{
			name: "no_keywords",
			input:  "* Just a headline\n\nSome text.\n",
			wantTitle: "",
		},
		{
			name: "value_with_colon",
			input: `#+TITLE: Article: Part 1
`,
			wantTitle: "Article: Part 1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fm := parseOrgKeywords([]byte(tc.input))
			if fm.Title != tc.wantTitle {
				t.Errorf("Title: got %q, want %q", fm.Title, tc.wantTitle)
			}
			if fm.Author != tc.wantAuthor {
				t.Errorf("Author: got %q, want %q", fm.Author, tc.wantAuthor)
			}
			if fm.ThumbMediaID != tc.wantThumb {
				t.Errorf("ThumbMediaID: got %q, want %q", fm.ThumbMediaID, tc.wantThumb)
			}
			if fm.Digest != tc.wantDigest {
				t.Errorf("Digest: got %q, want %q", fm.Digest, tc.wantDigest)
			}
		})
	}
}

func TestConvertOrgFile(t *testing.T) {
	// Skip if emacs is not available.
	if _, err := exec.LookPath("emacs"); err != nil {
		t.Skip("emacs not found, skipping Org conversion test")
	}

	t.Run("basic", func(t *testing.T) {
		orgContent := `#+TITLE: Org Test
#+AUTHOR: Tester

* Section 1

This is *bold* and /italic/ text.

* Section 2

- item 1
- item 2
`
		dir := t.TempDir()
		path := dir + "/test.org"
		if err := os.WriteFile(path, []byte(orgContent), 0644); err != nil {
			t.Fatal(err)
		}

		result, err := ConvertOrgFile(path)
		if err != nil {
			t.Fatalf("ConvertOrgFile error = %v", err)
		}

		if result.Title != "Org Test" {
			t.Errorf("expected title 'Org Test', got %q", result.Title)
		}
		if result.Author != "Tester" {
			t.Errorf("expected author 'Tester', got %q", result.Author)
		}
		if len(result.HTML) == 0 {
			t.Fatal("ConvertOrgFile returned empty HTML")
		}

		html := string(result.HTML)
		if !strings.Contains(html, "Section 1") {
			t.Errorf("HTML should contain content, got: %s", html)
		}
		if !strings.Contains(html, "•") {
			t.Errorf("HTML should contain list bullets, got: %s", html)
		}
	})

	t.Run("with_sidecar_yaml", func(t *testing.T) {
		orgContent := `#+TITLE: Org Title
#+AUTHOR: Org Author

Content.
`
		yamlContent := "title: YAML Title\n"
		dir := t.TempDir()
		orgPath := dir + "/article.org"
		yamlPath := dir + "/article.yaml"
		if err := os.WriteFile(orgPath, []byte(orgContent), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
			t.Fatal(err)
		}

		result, err := ConvertOrgFile(orgPath)
		if err != nil {
			t.Fatalf("ConvertOrgFile error = %v", err)
		}

		if result.Title != "YAML Title" {
			t.Errorf("expected title 'YAML Title' (sidecar overrides), got %q", result.Title)
		}
	})

	t.Run("code_block_with_language", func(t *testing.T) {
		orgContent := `#+TITLE: Code Test

#+begin_src go
package main
func main() {}
#+end_src
`
		dir := t.TempDir()
		path := dir + "/code.org"
		if err := os.WriteFile(path, []byte(orgContent), 0644); err != nil {
			t.Fatal(err)
		}

		result, err := ConvertOrgFile(path)
		if err != nil {
			t.Fatalf("ConvertOrgFile error = %v", err)
		}

		html := string(result.HTML)
		if !strings.Contains(html, `code-snippet__go`) {
			t.Errorf("HTML should contain Go code block class, got: %s", html)
		}
		if !strings.Contains(html, `data-lang="go"`) {
			t.Errorf("HTML should contain data-lang='go', got: %s", html)
		}
		if !strings.Contains(html, `package main`) {
			t.Errorf("HTML should contain code content, got: %s", html)
		}
	})

	t.Run("gfm_table", func(t *testing.T) {
		orgContent := `#+TITLE: Table Test

| A | B |
|---+---|
| 1 | 2 |
`
		dir := t.TempDir()
		path := dir + "/table.org"
		if err := os.WriteFile(path, []byte(orgContent), 0644); err != nil {
			t.Fatal(err)
		}

		result, err := ConvertOrgFile(path)
		if err != nil {
			t.Fatalf("ConvertOrgFile error = %v", err)
		}

		html := string(result.HTML)
		if !strings.Contains(html, "<table") {
			t.Errorf("HTML should contain table element, got: %s", html)
		}
		if !strings.Contains(html, "<th") {
			t.Errorf("HTML should contain table headers, got: %s", html)
		}
		if !strings.Contains(html, "<td") {
			t.Errorf("HTML should contain table cells, got: %s", html)
		}
	})

	t.Run("ordered_list", func(t *testing.T) {
		orgContent := `#+TITLE: List Test

1. First
2. Second
3. Third
`
		dir := t.TempDir()
		path := dir + "/list.org"
		if err := os.WriteFile(path, []byte(orgContent), 0644); err != nil {
			t.Fatal(err)
		}

		result, err := ConvertOrgFile(path)
		if err != nil {
			t.Fatalf("ConvertOrgFile error = %v", err)
		}

		html := string(result.HTML)
		if !strings.Contains(html, "1.") {
			t.Errorf("HTML should contain ordered list number '1.', got: %s", html)
		}
		if !strings.Contains(html, "2.") {
			t.Errorf("HTML should contain ordered list number '2.', got: %s", html)
		}
	})

	t.Run("no_title", func(t *testing.T) {
		orgContent := `* Just Content

Some text.
`
		dir := t.TempDir()
		path := dir + "/notitle.org"
		if err := os.WriteFile(path, []byte(orgContent), 0644); err != nil {
			t.Fatal(err)
		}

		result, err := ConvertOrgFile(path)
		if err != nil {
			t.Fatalf("ConvertOrgFile error = %v", err)
		}

		if result.Title != "" {
			t.Errorf("expected empty title when no #+TITLE, got %q", result.Title)
		}
	})

	t.Run("link_conversion", func(t *testing.T) {
		orgContent := `#+TITLE: Link

[[https://example.com][Example Link]]
`
		dir := t.TempDir()
		path := dir + "/link.org"
		if err := os.WriteFile(path, []byte(orgContent), 0644); err != nil {
			t.Fatal(err)
		}

		result, err := ConvertOrgFile(path)
		if err != nil {
			t.Fatalf("ConvertOrgFile error = %v", err)
		}

		html := string(result.HTML)
		if !strings.Contains(html, `href="https://example.com"`) {
			t.Errorf("HTML should contain link URL, got: %s", html)
		}
		if !strings.Contains(html, "Example Link") {
			t.Errorf("HTML should contain link text, got: %s", html)
		}
	})
}
