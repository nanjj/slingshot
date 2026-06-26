package mdtowx

import (
	"testing"
)

func TestRemoveCJCSpace(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "space between two CJK chars",
			in:   "相 信",
			want: "相信",
		},
		{
			name: "space between two CJK chars in sentence",
			in:   "我们 相信 你",
			want: "我们相信你",
		},
		{
			name: "multiple consecutive spaces",
			in:   "相   信",
			want: "相信",
		},
		{
			name: "newline between CJK chars",
			in:   "有\n多",
			want: "有多",
		},
		{
			name: "tab between CJK chars",
			in:   "树\t上",
			want: "树上",
		},
		{
			name: "carriage return between CJK chars",
			in:   "信\r息",
			want: "信息",
		},
		{
			name: "mixed whitespace between CJK chars",
			in:   "人\t世间\n有 多\r少树",
			want: "人世间有多少树",
		},
		{
			name: "ASCII text unchanged",
			in:   "I am an AI",
			want: "I am an AI",
		},
		{
			name: "CJK on one side only (left ASCII)",
			in:   "Hello 世界",
			want: "Hello 世界",
		},
		{
			name: "CJK on one side only (right ASCII)",
			in:   "你好 World",
			want: "你好 World",
		},
		{
			name: "newline between HTML tags preserved",
			in:   "</p>\n<p>",
			want: "</p>\n<p>",
		},
		{
			name: "newline between ASCII and CJK preserved",
			in:   "Tree\n语言",
			want: "Tree\n语言",
		},
		{
			name: "newline between CJK and ASCII preserved",
			in:   "语言\nTree",
			want: "语言\nTree",
		},
		{
			name: "mixed CJK and ASCII",
			in:   "A 相 信 B",
			want: "A 相信 B",
		},
		{
			name: "Japanese with spaces",
			in:   "これ は テスト",
			want: "これはテスト",
		},
		{
			name: "Greek letters",
			in:   "α β γ",
			want: "αβγ",
		},
		{
			name: "Cyrillic",
			in:   "при вет",
			want: "привет",
		},
		{
			name: "Korean",
			in:   "안 녕",
			want: "안녕",
		},
		{
			name: "empty string",
			in:   "",
			want: "",
		},
		{
			name: "only spaces",
			in:   "   ",
			want: "   ",
		},
		{
			name: "single CJK no space",
			in:   "信",
			want: "信",
		},
		{
			name: "space at boundaries preserved",
			in:   " 相 信 ",
			want: " 相信 ",
		},
		{
			name: "CJK with punctuation",
			in:   "， ，",
			want: "，，",
		},
		{
			name: "no change single space",
			in:   " ",
			want: " ",
		},
		{
			name: "newline at start preserved",
			in:   "\n相 信",
			want: "\n相信",
		},
		{
			name: "newline at end preserved",
			in:   "相 信\n",
			want: "相信\n",
		},
		{
			name: "org source line wrap: 人世间有\\n多少树",
			in:   "人世间有\n多少树",
			want: "人世间有多少树",
		},
		{
			name: "org source line wrap: 非得爬到树\\n上",
			in:   "非得爬到树\n上",
			want: "非得爬到树上",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(RemoveCJCSpace([]byte(tt.in)))
			if got != tt.want {
				t.Errorf("RemoveCJCSpace(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}


func TestSanitizeHTML_removeCJCSpace(t *testing.T) {
	// Test that SanitizeHTML removes spaces between CJK chars in text nodes
	// but preserves them in HTML-safe ways.
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "CJK space in paragraph",
			in:   `<p>相 信</p>`,
			want: `<p>相信</p>`,
		},
		{
			name: "CJK space in heading",
			in:   `<h2>标 题</h2>`,
			want: `<h2>标题</h2>`,
		},
		{
			name: "CJK space around inline element",
			in:   `<p>前 后<strong>粗体</strong>继 续</p>`,
			want: `<p>前后<strong>粗体</strong>继续</p>`,
		},
		{
			name: "ASCII text preserved",
			in:   `<p>Hello World</p>`,
			want: `<p>Hello World</p>`,
		},
		{
			name: "mixed ASCII and CJK preserved on boundary",
			in:   `<p>你好 World 2024</p>`,
			want: `<p>你好 World 2024</p>`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(SanitizeHTML([]byte(tt.in)))
			if got != tt.want {
				t.Errorf("SanitizeHTML(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSanitizeHTML_mailto(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "mailto link",
			in:   `<a href="mailto:fermi@dscli.io">费米</a>`,
			want: `费米`,
		},
		{
			name: "mailto with mixed content",
			in:   `联系 <a href="mailto:user@example.com">用户</a> 获取帮助`,
			want: `联系 用户 获取帮助`,
		},
		{
			name: "normal link preserved",
			in:   `<a href="https://example.com">正常链接</a>`,
			want: `<a href="https://example.com">正常链接</a>`,
		},
		{
			name: "normal link preserved with other text",
			in:   `这是<a href="https://example.com">链接</a>。`,
			want: `这是<a href="https://example.com">链接</a>。`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(SanitizeHTML([]byte(tt.in)))
			if got != tt.want {
				t.Errorf("SanitizeHTML() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeHTML_footnote(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "footnote backlink with role",
			in:   `<sup><a id="fnr.1" href="#fn.1" role="doc-backlink">1</a></sup>`,
			want: `1`,
		},
		{
			name: "footnote backlink with class",
			in:   `<sup><a class="footref" href="#fn.1">1</a></sup>`,
			want: `1`,
		},
		{
			name: "footnote with both class and role",
			in:   `<sup><a id="fnr.2" class="footref" href="#fn.2" role="doc-backlink">2</a></sup>`,
			want: `2`,
		},
		{
			name: "regular sup preserved",
			in:   `<sup>注册商标</sup>`,
			want: `<sup>注册商标</sup>`,
		},
		{
			name: "footnote in paragraph context",
			in:   `<p>这是正文<sup><a class="footref" href="#fn.1" role="doc-backlink">1</a></sup>继续</p>`,
			want: `<p>这是正文1继续</p>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(SanitizeHTML([]byte(tt.in)))
			if got != tt.want {
				t.Errorf("SanitizeHTML() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeHTML_mixed(t *testing.T) {
	input := `<p>本文参考了相关文献<sup><a class="footref" href="#fn.1" role="doc-backlink">1</a></sup>。如需联系请发邮件至 <a href="mailto:author@example.com">作者</a>。</p>`
	want := `<p>本文参考了相关文献1。如需联系请发邮件至 作者。</p>`

	got := string(SanitizeHTML([]byte(input)))
	if got != want {
		t.Errorf("SanitizeHTML() = %q, want %q", got, want)
	}
}

func TestSanitizeHTML_noChange(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{
			name: "plain text",
			in:   `Hello, World!`,
		},
		{
			name: "regular HTML",
			in:   `<p>这是一个<strong>正常</strong>段落。</p>`,
		},
		{
			name: "image tag",
			in:   `<img src="https://example.com/image.png" alt="示例图片">`,
		},
		{
			name: "code block",
			in:   `<pre><code>func main() {}</code></pre>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(SanitizeHTML([]byte(tt.in)))
			if got != tt.in {
				t.Errorf("SanitizeHTML() = %q, want %q (no change)", got, tt.in)
			}
		})
	}
}

func TestSanitizeHTML_empty(t *testing.T) {
	got := string(SanitizeHTML([]byte("")))
	if got != "" {
		t.Errorf("SanitizeHTML() = %q, want empty", got)
	}
}

func TestSanitizeHTML_nestedInSup(t *testing.T) {
	// Test that a regular <sup> with nested formatting is preserved.
	input := `<sup><em>注</em></sup>`
	want := `<sup><em>注</em></sup>`
	got := string(SanitizeHTML([]byte(input)))
	if got != want {
		t.Errorf("SanitizeHTML() = %q, want %q", got, want)
	}
}
