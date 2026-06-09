package mdtowx

import (
	"os"
	"strings"
	"testing"
)

func TestConvertMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string // substrings that must appear
		not   []string // substrings that must NOT appear
	}{
		{
			name:  "empty",
			input: "",
			want:  []string{},
		},
		{
			name:  "paragraph",
			input: `Hello world.`,
			want: []string{
				`<p`, `>Hello world.</p>`,
				`margin:0.8em 0`, `line-height:1.8`,
			},
		},
		{
			name: "headings_h1_h4",
			input: `# H1
## H2
### H3
#### H4`,
			want: []string{
				`<h1 style="font-size:1.8em;font-weight:bold;margin:1.2em 0 0.5em`,
				`<h2 style="font-size:1.5em;font-weight:bold;margin:1.2em 0 0.5em`,
				`<h3 style="font-size:1.3em;font-weight:bold;margin:1em 0 0.4em`,
				`<h4 style="font-size:1.1em;font-weight:bold;margin:1em 0 0.4em`,
				`</h1>`, `</h2>`, `</h3>`, `</h4>`,
			},
		},
		{
			name:  "inline_styles",
			input: "**bold** *italic* `code` ~~strike~~",
			want: []string{
				`<strong>bold</strong>`,
				`<em>italic</em>`,
				// codespan - check partial style match (exact output has additional font props)
				`<code style="background:#f0f0f0;padding:2px 4px;border-radius:3px`,
				`>code</code>`,
				`<del style="text-decoration:line-through">strike</del>`,
			},
		},
		{
			name:  "blockquote",
			input: `> quote`,
			want: []string{
				`<blockquote style="border-left:4px solid #d0d0d0;padding:10px 15px;margin:1em 0;background:#f9f9f9"`,
				`quote`,
			},
		},
		{
			name:  "code_block_fenced",
			input: "```go\nfunc main() {}\n```\n",
			want: []string{
				`<pre style="background:#f5f5f5;padding:16px;border-radius:4px;overflow-x:auto"`,
				`<code style="background:none;padding:0;font-family:Consolas,'Courier New',monospace;font-size:0.9em"`,
				`func main() {}`,
				`</code></pre>`,
			},
			not: []string{
				`class="language-`, // custom renderer doesn't emit language class
			},
		},
		{
			name:  "link",
			input: `[text](https://example.com)`,
			want: []string{
				`<a href="https://example.com" style="color:#007bff;text-decoration:none"`,
				`>text</a>`,
			},
		},
		{
			name:  "image",
			input: `![alt](img.png "title")`,
			want: []string{
				`<img src="img.png" alt="alt" title="title" style="max-width:100%;height:auto;display:block;margin:0.8em 0"`,
			},
		},
		{
			name: "unordered_list",
			input: `- a
- b
- c`,
			want: []string{
				`<ul style="margin:0.5em 0;padding-left:2em"`,
				`<li style="margin:0.3em 0">a</li>`,
				`<li style="margin:0.3em 0">b</li>`,
				`<li style="margin:0.3em 0">c</li>`,
				`</ul>`,
			},
		},
		{
			name: "ordered_list",
			input: `1. one
2. two`,
			want: []string{
				`<ol style="margin:0.5em 0;padding-left:2em"`,
				`<li style="margin:0.3em 0">one</li>`,
				`<li style="margin:0.3em 0">two</li>`,
				`</ol>`,
			},
		},
		{
			name:  "thematic_break",
			input: `---`,
			want: []string{
				`<hr style="margin:1.5em 0;border:none;border-top:2px solid #eee"`,
			},
		},
		{
			name: "table",
			input: `| A | B |
|---|---|---|
| 1 | 2 |`,
			want: []string{
				`<table style="border-collapse:collapse;width:100%;margin:1em 0"`,
				`<th`, `style="background:#f5f5f5;font-weight:bold"`,
				`<td style="border:1px solid #ddd;padding:8px;text-align:left"`,
				`A`, `B`, `1`, `2`,
			},
		},
		{
			name:  "strikethrough",
			input: `~~deleted~~`,
			want: []string{
				`<del style="text-decoration:line-through"`,
				`>deleted</del>`,
			},
		},
		{
			name:  "raw_html_passthrough",
			input: `<sup><a id="fnr.1" href="#fn.1">1</a></sup>`,
			want: []string{
				`<sup><a id="fnr.1" href="#fn.1">1</a></sup>`,
			},
		},
		{
			name:  "bold_inside_paragraph",
			input: `p **b** p`,
			want: []string{
				`<p style="margin:0.8em 0;line-height:1.8">p <strong>b</strong> p</p>`,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ConvertMarkdown([]byte(tc.input))
			if err != nil {
				t.Fatalf("ConvertMarkdown() error = %v", err)
			}
			s := string(got)
			for _, w := range tc.want {
				if !strings.Contains(s, w) {
					t.Errorf("expected output to contain %q\n--- got ---\n%s", w, s)
				}
			}
			for _, n := range tc.not {
				if strings.Contains(s, n) {
					t.Errorf("expected output NOT to contain %q\n--- got ---\n%s", n, s)
				}
			}
		})
	}
}

func TestConvertFile(t *testing.T) {
	input := []byte("# Hello\n\nWorld.\n")
	dir := t.TempDir()
	path := dir + "/test.md"
	if err := os.WriteFile(path, input, 0644); err != nil {
		t.Fatal(err)
	}
	got, err := ConvertFile(path)
	if err != nil {
		t.Fatalf("ConvertFile error = %v", err)
	}
	if len(got) == 0 {
		t.Fatal("ConvertFile returned empty output")
	}
	if !strings.Contains(string(got), "World.") {
		t.Errorf("output should contain content, got: %s", string(got))
	}
}

func TestExtractImagePaths(t *testing.T) {
	t.Run("no_images", func(t *testing.T) {
		html := []byte(`<p>no images here</p>`)
		refs := ExtractImagePaths(html, "/base")
		if len(refs) != 0 {
			t.Errorf("expected 0 refs, got %d: %v", len(refs), refs)
		}
	})

	t.Run("local_images", func(t *testing.T) {
		html := []byte(`<img src="images/foo.png" style="..."><img src="bar.jpg" style="...">`)
		refs := ExtractImagePaths(html, "/base/dir")
		if len(refs) != 2 {
			t.Fatalf("expected 2 refs, got %d: %v", len(refs), refs)
		}
		// Original src preserved
		if refs[0].Src != "images/foo.png" {
			t.Errorf("expected src='images/foo.png', got %q", refs[0].Src)
		}
		// AbsPath should be resolved
		if !strings.HasPrefix(refs[0].AbsPath, "/base/dir/images/foo.png") {
			t.Errorf("expected resolved abs path, got %q", refs[0].AbsPath)
		}
		if refs[1].Src != "bar.jpg" {
			t.Errorf("expected src='bar.jpg', got %q", refs[1].Src)
		}
	})

	t.Run("skips_http_urls", func(t *testing.T) {
		html := []byte(`<img src="https://example.com/img.png"><img src="http://example.com/img.jpg">`)
		refs := ExtractImagePaths(html, "/base")
		if len(refs) != 0 {
			t.Errorf("expected 0 refs for http URLs, got %d: %v", len(refs), refs)
		}
	})

	t.Run("absolute_path", func(t *testing.T) {
		html := []byte(`<img src="/absolute/path/img.png">`)
		refs := ExtractImagePaths(html, "/base")
		if len(refs) != 1 {
			t.Fatalf("expected 1 ref, got %d", len(refs))
		}
		if refs[0].Src != "/absolute/path/img.png" {
			t.Errorf("expected src='/absolute/path/img.png', got %q", refs[0].Src)
		}
		if refs[0].AbsPath != "/absolute/path/img.png" {
			t.Errorf("expected AbsPath='/absolute/path/img.png', got %q", refs[0].AbsPath)
		}
	})

	t.Run("mixed_urls", func(t *testing.T) {
		html := []byte(`<img src="local.png"><img src="https://remote.com/img.jpg"><img src="data:image/png;base64,abc">`)
		refs := ExtractImagePaths(html, "/base")
		if len(refs) != 1 {
			t.Errorf("expected 1 local ref, got %d: %v", len(refs), refs)
		}
		if refs[0].Src != "local.png" {
			t.Errorf("expected src='local.png', got %q", refs[0].Src)
		}
	})
}

func TestReplaceImageURLs(t *testing.T) {
	t.Run("single_replacement", func(t *testing.T) {
		html := []byte(`<img src="img.png" style="...">`)
		replacements := map[string]string{
			"img.png": "http://mmbiz.qpic.cn/abc123",
		}
		result := ReplaceImageURLs(html, replacements)
		if !strings.Contains(string(result), `src="http://mmbiz.qpic.cn/abc123"`) {
			t.Errorf("expected new URL, got: %s", string(result))
		}
	})

	t.Run("multiple_replacements", func(t *testing.T) {
		html := []byte(`<img src="img1.png"><img src="img2.png">`)
		replacements := map[string]string{
			"img1.png": "http://mmbiz.qpic.cn/url1",
			"img2.png": "http://mmbiz.qpic.cn/url2",
		}
		result := ReplaceImageURLs(html, replacements)
		s := string(result)
		if !strings.Contains(s, `src="http://mmbiz.qpic.cn/url1"`) {
			t.Errorf("expected url1, got: %s", s)
		}
		if !strings.Contains(s, `src="http://mmbiz.qpic.cn/url2"`) {
			t.Errorf("expected url2, got: %s", s)
		}
	})

	t.Run("no_match_unchanged", func(t *testing.T) {
		html := []byte(`<img src="unknown.png">`)
		result := ReplaceImageURLs(html, map[string]string{
			"other.png": "http://mmbiz.qpic.cn/url",
		})
		if string(result) != string(html) {
			t.Errorf("expected unchanged HTML, got: %s", string(result))
		}
	})
}

func TestExtractAndReplaceRoundTrip(t *testing.T) {
	// Test a full roundtrip: extract refs, create replacements, replace URLs.
	baseDir := "/some/base"

	html := []byte(`<p><img src="images/photo.png" style="..."></p>`)
	refs := ExtractImagePaths(html, baseDir)
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d: %v", len(refs), refs)
	}

	// refs[0].Src should be the original relative path
	if refs[0].Src != "images/photo.png" {
		t.Errorf("expected Src='images/photo.png', got %q", refs[0].Src)
	}
	// refs[0].AbsPath should be the resolved path
	if refs[0].AbsPath != "/some/base/images/photo.png" {
		t.Errorf("expected AbsPath='/some/base/images/photo.png', got %q", refs[0].AbsPath)
	}

	// Replace using the original Src
	replacements := map[string]string{
		refs[0].Src: "http://mmbiz.qpic.cn/newurl",
	}
	result := ReplaceImageURLs(html, replacements)
	s := string(result)
	if !strings.Contains(s, `src="http://mmbiz.qpic.cn/newurl"`) {
		t.Errorf("expected new URL, got: %s", s)
	}
	if strings.Contains(s, `src="images/photo.png"`) {
		t.Errorf("old path should be replaced, got: %s", s)
	}
}
