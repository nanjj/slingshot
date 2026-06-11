package mdtowx

import (
	"testing"
)

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
