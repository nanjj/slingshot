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
				`line-height:1.6`, `font-size:16px`,
				// No font-family or color — WeChat Reader uses its own defaults
			},
		},
		{
			name: "headings_h1_h4",
			input: `# H1
## H2
### H3
#### H4`,
			want: []string{
				`<h2 style="text-align:center;line-height:1.5`,
				`font-size:140%;margin:80px 10px 40px 10px;font-weight:normal">H1</h2>`,
				`<h2 style="text-align:center;line-height:1.5`,
				`font-size:140%;margin:80px 10px 40px 10px;font-weight:normal">H2</h2>`,
				`<h3 style="text-align:left;line-height:1.5`,
				`font-size:120%;margin:40px 10px 20px 10px;font-weight:bold">H3</h3>`,
				`<h3 style="text-align:left;line-height:1.5`,
				`font-size:120%;margin:40px 10px 20px 10px;font-weight:bold">H4</h3>`,
			},
		},
		{
			name:  "inline_styles",
			input: "**bold** *italic* `code` ~~strike~~",
			want: []string{
				`<strong>bold</strong>`,
				`<em>italic</em>`,
				// codespan - check partial style match
				`<code style="text-align:left;color:#ff3502;line-height:1.5;font-size:90%`,
				`>code</code>`,
				`<del style="text-decoration:line-through">strike</del>`,
			},
		},
		{
			name:  "blockquote",
			input: `> quote`,
			want: []string{
				`<blockquote style="text-align:left;color:rgb(91,91,91);line-height:1.5;font-size:16px`,
				`quote`,
				// No font-family — matches real WeChat articles
			},
		},
		{
			name:  "code_block_fenced",
			input: "```go\nfunc main() {}\n```\n",
			want: []string{
				`<section class="code-snippet__fix code-snippet__go">`,
				`<pre class="code-snippet__go" data-lang="go">`,
				`<span class="code-snippet_outer">func main() {}</span>`,
				`</section>`,
			},
			not: []string{
				`class="language-`,       // custom renderer doesn't emit language class
				`style="background:#f5f5`, // no longer uses inline styles
			},
		},
		{
			name:  "code_block_indented",
			input: "\tfunc foo() {}\n",
			want: []string{
				`<section class="code-snippet__fix code-snippet__text">`,
				`<pre class="code-snippet__text" data-lang="">`,
				`<span class="code-snippet_outer">func foo() {}</span>`,
				`</section>`,
			},
		},
		{
			name:  "code_block_fenced_no_lang",
			input: "```\nhello\n```\n",
			want: []string{
				`<section class="code-snippet__fix code-snippet__text">`,
				`<pre class="code-snippet__text" data-lang="">`,
				`<span class="code-snippet_outer">hello</span>`,
				`</section>`,
			},
		},
		{
			name:  "code_block_with_html_entities",
			input: "```html\n<div> & \"quotes\"\n```\n",
			want: []string{
				`<span class="code-snippet_outer">&lt;div&gt; &amp; &#34;quotes&#34;</span>`,
			},
			not: []string{
				`<div>`,
			},
		},
		{
			name:  "code_block_empty_fenced",
			input: "```go\n```\n",
			want: []string{
				`<section class="code-snippet__fix code-snippet__go">`,
				`<pre class="code-snippet__go" data-lang="go">`,
				`</pre>`,
				`</section>`,
			},
		},
		{
			name:  "code_block_malicious_lang",
			// goldmark extracts only the first word of the info string as language
			input: "```\"><img src=x onerror=alert(1)>\ncode\n```\n",
			want: []string{
				// language "><img" is not a valid CSS class → falls back to "text"
				`<section class="code-snippet__fix code-snippet__text">`,
				// data-lang gets html.EscapeString applied
				`data-lang="&#34;&gt;&lt;img"`,
				`<span class="code-snippet_outer">code</span>`,
				`</section>`,
			},
			not: []string{
				`class="code-snippet__"<img`, // class should not contain raw malicious chars
				`data-lang=""><img`,          // data-lang should not be raw
			},
		},

		{
			name:  "link",
			input: `[text](https://example.com)`,
			want: []string{
				`<a href="https://example.com" style="color:rgb(13,117,252);text-decoration:none"`,
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
				`<ul style="`,
				`padding-left:1.5em`,
				`<li style="display:block;margin:0 8px"><section style="margin:0;padding:0">• a</section></li>`,
				`<li style="display:block;margin:0 8px"><section style="margin:0;padding:0">• b</section></li>`,
				`<li style="display:block;margin:0 8px"><section style="margin:0;padding:0">• c</section></li>`,
				`</ul>`,
			},
			not: []string{
				`<p style="`, `<ol`, `<span`, `text-indent`, `margin-right: 10px`,
			},
		},
		{
			name: "ordered_list",
			input: `1. one
2. two`,
			want: []string{
				`<ol style="`,
				`padding-left:1.5em`,
				`<li style="display:block;margin:0 8px"><section style="margin:0;padding:0">1. one</section></li>`,
				`<li style="display:block;margin:0 8px"><section style="margin:0;padding:0">2. two</section></li>`,
				`</ol>`,
			},
			not: []string{
				`<p style="`, `<ul`, `<span`, `text-indent`, `margin-right: 10px`,
			},
		},
		{
			name:  "thematic_break",
			input: `---`,
			want: []string{
				`<hr style="margin:1.5em 0;border:none;border-top:1px solid #eee"`,
			},
		},
		{
			name: "table",
			input: `| A | B |
|---|---|---|
| 1 | 2 |`,
			want: []string{
				`border-collapse:collapse;margin:20px 0`,
				`<th`, `font-size:80%`,
				`border:1px solid #dfdfdf;padding:4px 8px`,
				`<td style="text-align:left;line-height:1.5;font-size:80%`,
				`border:1px solid #dfdfdf;padding:4px 8px`,
				`A`, `B`, `1`, `2`,
				// No font-family or color — matches real WeChat articles
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
				`<p style="text-align:left;line-height:1.6;font-size:16px`, `p <strong>b</strong> p</p>`,
				// No font-family or color — matches real WeChat articles
			},
		},
		// Front matter tests
		{
			name: "front_matter_title_author",
			input: `---
title: My Article
author: John Doe
---

# Hello`,
			want: []string{
				`<h2`, `Hello`,
			},
		},
		{
			name:  "front_matter_only_title",
			input: "---\ntitle: Just Title\n---\n\nContent.",
			want: []string{
				`Content.`,
			},
		},
		{
			name: "front_matter_empty_no_double_title",
			input: `---
title: Test
---

# Title in body`,
			want: []string{
				`<h2`, `Title in body`,
			},
			not: []string{
				`Test`, // front matter content should not appear in HTML
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ConvertMarkdown([]byte(tc.input))
			if err != nil {
				t.Fatalf("ConvertMarkdown() error = %v", err)
			}
			s := string(result.HTML)
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
	t.Run("basic", func(t *testing.T) {
		input := []byte("# Hello\n\nWorld.\n")
		dir := t.TempDir()
		path := dir + "/test.md"
		if err := os.WriteFile(path, input, 0644); err != nil {
			t.Fatal(err)
		}
		result, err := ConvertFile(path)
		if err != nil {
			t.Fatalf("ConvertFile error = %v", err)
		}
		if len(result.HTML) == 0 {
			t.Fatal("ConvertFile returned empty HTML")
		}
		if !strings.Contains(string(result.HTML), "World.") {
			t.Errorf("output should contain content, got: %s", string(result.HTML))
		}
	})

	t.Run("with_front_matter", func(t *testing.T) {
		input := []byte("---\ntitle: Test Title\nauthor: An Author\n---\n\n# Hello\n")
		dir := t.TempDir()
		path := dir + "/front.md"
		if err := os.WriteFile(path, input, 0644); err != nil {
			t.Fatal(err)
		}
		result, err := ConvertFile(path)
		if err != nil {
			t.Fatalf("ConvertFile error = %v", err)
		}
		if result.Title != "Test Title" {
			t.Errorf("expected title 'Test Title', got %q", result.Title)
		}
		if result.Author != "AnAuthor" {
			t.Errorf("expected author 'AnAuthor', got %q", result.Author)
		}
		if !strings.Contains(string(result.HTML), "Hello") {
			t.Errorf("HTML should contain content, got: %s", string(result.HTML))
		}
	})

	t.Run("without_front_matter", func(t *testing.T) {
		input := []byte("# Just Content\n")
		dir := t.TempDir()
		path := dir + "/plain.md"
		if err := os.WriteFile(path, input, 0644); err != nil {
			t.Fatal(err)
		}
		result, err := ConvertFile(path)
		if err != nil {
			t.Fatalf("ConvertFile error = %v", err)
		}
		if result.Title != "" {
			t.Errorf("expected empty title, got %q", result.Title)
		}
		if result.Author != "" {
			t.Errorf("expected empty author, got %q", result.Author)
		}
	})

	t.Run("sidecar_yaml_overrides_front_matter", func(t *testing.T) {
		// Both front matter and sidecar YAML — YAML wins
		mdInput := []byte("---\ntitle: FM Title\nauthor: FM Author\nthumb_media_id: fm123\n---\n\n# Content\n")
		yamlInput := []byte("title: YAML Title\nauthor: YAML Author\nthumb_media_id: yaml456\n")
		dir := t.TempDir()
		mdPath := dir + "/article.md"
		yamlPath := dir + "/article.yaml"
		if err := os.WriteFile(mdPath, mdInput, 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(yamlPath, yamlInput, 0644); err != nil {
			t.Fatal(err)
		}
		result, err := ConvertFile(mdPath)
		if err != nil {
			t.Fatalf("ConvertFile error = %v", err)
		}
		if result.Title != "YAML Title" {
			t.Errorf("expected title 'YAML Title', got %q", result.Title)
		}
		if result.Author != "YAMLAuth" {
			// Sanitized: "YAML Author" (11 chars) → "YAMLAuth" (8)
			t.Errorf("expected author 'YAMLAuth', got %q", result.Author)
		}
		if result.ThumbMediaID != "yaml456" {
			t.Errorf("expected thumb_media_id 'yaml456', got %q", result.ThumbMediaID)
		}
		if !strings.Contains(string(result.HTML), "Content") {
			t.Errorf("HTML should contain markdown content")
		}
	})

	t.Run("sidecar_yaml_without_front_matter", func(t *testing.T) {
		// No front matter in markdown, metadata comes from YAML file
		mdInput := []byte("# Just Content\n")
		yamlInput := []byte("title: Sidecar Title\nauthor: Side Author\n")
		dir := t.TempDir()
		mdPath := dir + "/pure.md"
		yamlPath := dir + "/pure.yaml"
		if err := os.WriteFile(mdPath, mdInput, 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(yamlPath, yamlInput, 0644); err != nil {
			t.Fatal(err)
		}
		result, err := ConvertFile(mdPath)
		if err != nil {
			t.Fatalf("ConvertFile error = %v", err)
		}
		if result.Title != "Sidecar Title" {
			t.Errorf("expected title 'Sidecar Title', got %q", result.Title)
		}
		if result.Author != "SideAuth" {
			// Sanitized: "Side Author" (11 chars) → "SideAuth" (8)
			t.Errorf("expected author 'SideAuth', got %q", result.Author)
		}
	})

	t.Run("sidecar_yaml_partial_override", func(t *testing.T) {
		// YAML only provides title; author and thumb_media_id come from front matter
		mdInput := []byte("---\ntitle: FM Title\nauthor: FM Author\nthumb_media_id: fm789\n---\n\n# Content\n")
		yamlInput := []byte("title: Override Title\n")
		dir := t.TempDir()
		mdPath := dir + "/partial.md"
		yamlPath := dir + "/partial.yaml"
		if err := os.WriteFile(mdPath, mdInput, 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(yamlPath, yamlInput, 0644); err != nil {
			t.Fatal(err)
		}
		result, err := ConvertFile(mdPath)
		if err != nil {
			t.Fatalf("ConvertFile error = %v", err)
		}
		if result.Title != "Override Title" {
			t.Errorf("expected title 'Override Title', got %q", result.Title)
		}
		if result.Author != "FMAuthor" {
			// Sanitized: spaces stripped, so "FM Author" → "FMAuthor"
			t.Errorf("expected author 'FMAuthor', got %q", result.Author)
		}
		if result.ThumbMediaID != "fm789" {
			t.Errorf("expected thumb_media_id 'fm789', got %q", result.ThumbMediaID)
		}
	})

	t.Run("sidecar_yaml_yml_extension", func(t *testing.T) {
		// .yml extension (not .yaml) should also be recognized
		mdInput := []byte("# Content\n")
		yamlInput := []byte("title: From YML\nauthor: YML Author\n")
		dir := t.TempDir()
		mdPath := dir + "/test.md"
		ymlPath := dir + "/test.yml"
		if err := os.WriteFile(mdPath, mdInput, 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(ymlPath, yamlInput, 0644); err != nil {
			t.Fatal(err)
		}
		result, err := ConvertFile(mdPath)
		if err != nil {
			t.Fatalf("ConvertFile error = %v", err)
		}
		if result.Title != "From YML" {
			t.Errorf("expected title 'From YML', got %q", result.Title)
		}
		if result.Author != "YMLAutho" {
			// Sanitized: "YML Author" (10 chars) → "YMLAutho" (8)
			t.Errorf("expected author 'YMLAutho', got %q", result.Author)
		}
	})

	t.Run("sidecar_yaml_prefers_yaml_over_yml", func(t *testing.T) {
		// Both .yaml and .yml exist — .yaml should be used
		mdInput := []byte("# Content\n")
		yamlInput := []byte("title: From Yaml\nauthor: Yaml Auth\n")
		ymlInput := []byte("title: From Yml\nauthor: Yml Auth\n")
		dir := t.TempDir()
		mdPath := dir + "/prefer.md"
		yamlPath := dir + "/prefer.yaml"
		ymlPath := dir + "/prefer.yml"
		if err := os.WriteFile(mdPath, mdInput, 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(yamlPath, yamlInput, 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(ymlPath, ymlInput, 0644); err != nil {
			t.Fatal(err)
		}
		result, err := ConvertFile(mdPath)
		if err != nil {
			t.Fatalf("ConvertFile error = %v", err)
		}
		if result.Title != "From Yaml" {
			t.Errorf("expected title 'From Yaml' (.yaml preferred), got %q", result.Title)
		}
	})

	t.Run("sidecar_yaml_invalid_ignored", func(t *testing.T) {
		// Invalid YAML should be ignored, front matter takes effect
		mdInput := []byte("---\ntitle: Valid Title\nauthor: Valid\n---\n\n# Content\n")
		yamlInput := []byte("{{ invalid yaml\nbroken")
		dir := t.TempDir()
		mdPath := dir + "/broken.md"
		yamlPath := dir + "/broken.yaml"
		if err := os.WriteFile(mdPath, mdInput, 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(yamlPath, yamlInput, 0644); err != nil {
			t.Fatal(err)
		}
		result, err := ConvertFile(mdPath)
		if err != nil {
			t.Fatalf("ConvertFile error = %v", err)
		}
		if result.Title != "Valid Title" {
			t.Errorf("expected title 'Valid Title' from front matter, got %q", result.Title)
		}
		if result.Author != "Valid" {
			t.Errorf("expected author 'Valid' from front matter, got %q", result.Author)
		}
	})

	t.Run("sidecar_yaml_empty_ignored", func(t *testing.T) {
		// Empty YAML file should be ignored
		mdInput := []byte("---\ntitle: From FM\n---\n\n# Content\n")
		dir := t.TempDir()
		mdPath := dir + "/empty-yaml.md"
		yamlPath := dir + "/empty-yaml.yaml"
		if err := os.WriteFile(mdPath, mdInput, 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(yamlPath, []byte{}, 0644); err != nil {
			t.Fatal(err)
		}
		result, err := ConvertFile(mdPath)
		if err != nil {
			t.Fatalf("ConvertFile error = %v", err)
		}
		if result.Title != "From FM" {
			t.Errorf("expected title 'From FM' from front matter, got %q", result.Title)
		}
	})

	t.Run("sidecar_yaml_author_sanitized", func(t *testing.T) {
		// Author from YAML should be sanitized (long author truncated)
		mdInput := []byte("# Content\n")
		yamlInput := []byte("title: T\nauthor: a b c d e f g h i j\n")
		dir := t.TempDir()
		mdPath := dir + "/long.md"
		yamlPath := dir + "/long.yaml"
		if err := os.WriteFile(mdPath, mdInput, 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(yamlPath, yamlInput, 0644); err != nil {
			t.Fatal(err)
		}
		result, err := ConvertFile(mdPath)
		if err != nil {
			t.Fatalf("ConvertFile error = %v", err)
		}
		if result.Author != "abcdefgh" {
			t.Errorf("expected sanitized author 'abcdefgh', got %q", result.Author)
		}
	})

	t.Run("sidecar_yaml_title_too_long", func(t *testing.T) {
		// Title from YAML exceeding 64 chars should error
		longTitle := ""
		for i := 0; i < 65; i++ {
			longTitle += "a"
		}
		mdInput := []byte("# Content\n")
		yamlInput := []byte("title: " + longTitle + "\n")
		dir := t.TempDir()
		mdPath := dir + "/long-title.md"
		yamlPath := dir + "/long-title.yaml"
		if err := os.WriteFile(mdPath, mdInput, 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(yamlPath, yamlInput, 0644); err != nil {
			t.Fatal(err)
		}
		_, err := ConvertFile(mdPath)
		if err == nil {
			t.Fatal("expected error for title > 64 chars, got nil")
		}
		if !strings.Contains(err.Error(), "exceeds 64 characters") {
			t.Errorf("unexpected error: %v", err)
		}
	})
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

func TestValidateTitle(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if err := ValidateTitle(""); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("exactly_64", func(t *testing.T) {
		title := string(make([]rune, 64))
		if err := ValidateTitle(title); err != nil {
			t.Errorf("expected nil for 64 chars, got %v", err)
		}
	})

	t.Run("65_chars_error", func(t *testing.T) {
		title := string(make([]rune, 65))
		if err := ValidateTitle(title); err == nil {
			t.Error("expected error for 65 chars, got nil")
		} else if !strings.Contains(err.Error(), "exceeds 64 characters") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("multi_byte_64", func(t *testing.T) {
		// 64 Chinese characters = 64 runes
		title := ""
		for i := 0; i < 64; i++ {
			title += "中"
		}
		if err := ValidateTitle(title); err != nil {
			t.Errorf("expected nil for 64 Chinese chars, got %v", err)
		}
	})

	t.Run("multi_byte_65_error", func(t *testing.T) {
		title := ""
		for i := 0; i < 65; i++ {
			title += "文"
		}
		if err := ValidateTitle(title); err == nil {
			t.Error("expected error for 65 Chinese chars, got nil")
		}
	})
}

func TestSanitizeAuthor(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got := SanitizeAuthor(""); got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("under_8_unchanged", func(t *testing.T) {
		got := SanitizeAuthor("John")
		if got != "John" {
			t.Errorf("expected 'John', got %q", got)
		}
	})

	t.Run("exactly_8_unchanged", func(t *testing.T) {
		got := SanitizeAuthor("12345678")
		if got != "12345678" {
			t.Errorf("expected '12345678', got %q", got)
		}
	})

	t.Run("truncate_over_8", func(t *testing.T) {
		got := SanitizeAuthor("123456789")
		if got != "12345678" {
			t.Errorf("expected '12345678', got %q", got)
		}
	})

	t.Run("strip_spaces_then_truncate", func(t *testing.T) {
		got := SanitizeAuthor("A B C D E F")
		// "ABCDEF" is 6 chars — after stripping spaces it's "ABCDEF" (6), under 8
		if got != "ABCDEF" {
			t.Errorf("expected 'ABCDEF', got %q", got)
		}
	})

	t.Run("strip_spaces_over_8", func(t *testing.T) {
		got := SanitizeAuthor("a b c d e f g h i")
		// after stripping spaces: "abcdefghi" (9 chars) → truncated to "abcdefgh" (8)
		if got != "abcdefgh" {
			t.Errorf("expected 'abcdefgh', got %q", got)
		}
	})

	t.Run("multi_byte_truncate", func(t *testing.T) {
		got := SanitizeAuthor("一二三四五六七八九")
		if got != "一二三四五六七八" {
			t.Errorf("expected '一二三四五六七八', got %q", got)
		}
	})

	t.Run("spaces_multi_byte", func(t *testing.T) {
		got := SanitizeAuthor("一 二 三 四 五 六 七 八 九")
		// strip spaces: "一二三四五六七八九" (9 runes) → "一二三四五六七八" (8)
		if got != "一二三四五六七八" {
			t.Errorf("expected '一二三四五六七八', got %q", got)
		}
	})
}

func TestConvertMarkdown_TitleTooLong(t *testing.T) {
	// 65 'a' characters
	longTitle := ""
	for i := 0; i < 65; i++ {
		longTitle += "a"
	}
	input := "---\ntitle: " + longTitle + "\n---\n\n# Hello\n"
	_, err := ConvertMarkdown([]byte(input))
	if err == nil {
		t.Fatal("expected error for title > 64 chars, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds 64 characters") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestConvertMarkdown_AuthorSanitized(t *testing.T) {
	input := "---\ntitle: OK\nauthor: a b c d e f g h i\n---\n\n# Hello\n"
	result, err := ConvertMarkdown([]byte(input))
	if err != nil {
		t.Fatalf("ConvertMarkdown() error = %v", err)
	}
	if result.Author != "abcdefgh" {
		t.Errorf("expected sanitized author 'abcdefgh', got %q", result.Author)
	}
}

func TestConvertMarkdown_AuthorNotExceeding(t *testing.T) {
	input := "---\ntitle: OK\nauthor: John\n---\n\n# Hello\n"
	result, err := ConvertMarkdown([]byte(input))
	if err != nil {
		t.Fatalf("ConvertMarkdown() error = %v", err)
	}
	if result.Author != "John" {
		t.Errorf("expected author 'John', got %q", result.Author)
	}
}

func TestConvertMarkdown_ThumbMediaID(t *testing.T) {
	t.Run("extracted_from_front_matter", func(t *testing.T) {
		input := "---\ntitle: With Cover\nauthor: Me\nthumb_media_id: abc123def\n---\n\n# Hello\n"
		result, err := ConvertMarkdown([]byte(input))
		if err != nil {
			t.Fatalf("ConvertMarkdown() error = %v", err)
		}
		if result.ThumbMediaID != "abc123def" {
			t.Errorf("expected thumb_media_id 'abc123def', got %q", result.ThumbMediaID)
		}
	})

	t.Run("empty_when_not_in_front_matter", func(t *testing.T) {
		input := "# No front matter\n"
		result, err := ConvertMarkdown([]byte(input))
		if err != nil {
			t.Fatalf("ConvertMarkdown() error = %v", err)
		}
		if result.ThumbMediaID != "" {
			t.Errorf("expected empty thumb_media_id, got %q", result.ThumbMediaID)
		}
	})
}
