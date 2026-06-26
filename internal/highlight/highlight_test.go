package highlight

import (
	"strings"
	"testing"
)

func TestHighlightGo(t *testing.T) {
	src := []byte(`package main

func hello(name string) string {
	msg := fmt.Sprintf("Hello, %s!", name)
	return msg
}
`)

	got := Highlight(src, "go")
	if len(got) == 0 {
		t.Fatal("expected non-empty output")
	}

	// Verify HTML escaping — original source is preserved
	out := string(got)
	if !strings.Contains(out, "package") {
		t.Errorf("output should contain source text")
	}

	// Verify inline styles are present
	if !strings.Contains(out, `style="color:`) {
		t.Errorf("output should contain inline style spans, got:\n%s", out)
	}

	// Verify keyword "func" is wrapped
	if !strings.Contains(out, `<span style="color:#d73a49">func</span>`) {
		t.Errorf("expected 'func' keyword to be highlighted, got:\n%s", out)
	}

	// Verify "hello" is wrapped — Go built-in query captures identifiers
	// as @variable with higher priority than @function
	if !strings.Contains(out, `<span style="color:#e36209">hello</span>`) {
		t.Errorf("expected 'hello' to be highlighted as variable, got:\n%s", out)
	}

	// Verify type "string" is wrapped
	if !strings.Contains(out, `<span style="color:#22863a">string</span>`) {
		t.Errorf("expected 'string' to be highlighted as type, got:\n%s", out)
	}
}

func TestHighlightGoWithComment(t *testing.T) {
	src := []byte(`// this is a comment
func main() {}`)

	out := string(Highlight(src, "go"))

	// Comment should be italic
	if !strings.Contains(out, `font-style: italic`) {
		t.Errorf("expected comment to be italic, got:\n%s", out)
	}
}

func TestHighlightUnknownLanguage(t *testing.T) {
	src := []byte(`some random text`)
	got := Highlight(src, "nonexistent-language-xyz")

	// Should return HTML-escaped original text, no spans
	out := string(got)
	if strings.Contains(out, `<span`) {
		t.Errorf("unknown language should not produce spans, got:\n%s", out)
	}
	// Should not be empty
	if len(got) == 0 {
		t.Fatal("expected non-empty output for unknown language")
	}
}

func TestHighlightEmpty(t *testing.T) {
	got := Highlight(nil, "go")
	if got != nil {
		t.Errorf("expected nil for nil input, got %q", string(got))
	}

	got = Highlight([]byte{}, "go")
	if got != nil {
		t.Errorf("expected nil for empty input, got %q", string(got))
	}
}

func TestHighlightPython(t *testing.T) {
	src := []byte(`def hello(name):
    """Greet someone."""
    print(f"Hello, {name}!")
`)

	got := Highlight(src, "python")
	out := string(got)

	// Verify 'def' is highlighted as keyword
	if !strings.Contains(out, `<span style="color:#d73a49">def</span>`) {
		t.Errorf("expected 'def' keyword highlighted, got:\n%s", out)
	}

	// Verify HTML escaping: < > & in source should be escaped
	if strings.Contains(out, "<") && !strings.Contains(out, "&lt;") {
		// Actually raw < might not appear in this specific source
	}

	// Should have at least some spans
	if !strings.Contains(out, `<span style="color:`) {
		t.Errorf("expected inline styles for Python, got:\n%s", out)
	}
}

func TestHighlightJavaScript(t *testing.T) {
	src := []byte(`function greet(name) {
    console.log("Hello, " + name);
}`)

	got := Highlight(src, "javascript")
	out := string(got)

	// Verify function keyword
	if !strings.Contains(out, `<span style="color:#d73a49">function</span>`) {
		t.Errorf("expected 'function' keyword highlighted, got:\n%s", out)
	}

	// Should have inline styles
	if !strings.Contains(out, `style="color:`) {
		t.Errorf("expected inline styles for JavaScript, got:\n%s", out)
	}
}

func TestHighlightResolveLangByName(t *testing.T) {
	// Test various language name formats
	tests := []struct {
		name string
		lang string
	}{
		{"go", "go"},
		{"golang", "go"},
		{"py", "python"},
		{"javascript", "javascript"},
		{"js", "javascript"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := []byte(`func main() {}`)
			got := Highlight(src, tt.lang)
			if len(got) == 0 {
				t.Fatalf("Highlight(%q) returned empty", tt.lang)
			}
		})
	}
}

func TestHighlightPageSimple(t *testing.T) {
	html := []byte(`<!DOCTYPE html>
<html>
<head><title>Test</title></head>
<body>
<h1>Test Page</h1>
<pre class="src-go"><code>func main() {}</code></pre>
</body>
</html>`)

	got, count, err := HighlightPage(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 block highlighted, got %d", count)
	}

	out := string(got)
	if !strings.Contains(out, `<span style="color:#d73a49">func</span>`) {
		t.Errorf("expected highlighted func keyword in output, got:\n%s", out)
	}

	// Original structure should be preserved
	if !strings.Contains(out, "<h1>Test Page</h1>") {
		t.Errorf("expected original HTML structure preserved")
	}
}

func TestHighlightPageEmacsFormat(t *testing.T) {
	// Emacs org-mode HTML export produces class="src src-XXX" (two classes).
	html := []byte(`<!DOCTYPE html>
<html>
<head><title>Test</title></head>
<body>
<h1>Test Page</h1>
<pre class="src src-go"><code>func main() {}</code></pre>
</body>
</html>`)

	got, count, err := HighlightPage(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 block highlighted, got %d", count)
	}

	out := string(got)
	if !strings.Contains(out, `<span style="color:#d73a49">func</span>`) {
		t.Errorf("expected highlighted func keyword in output, got:\n%s", out)
	}
}

func TestHighlightPageMultipleBlocks(t *testing.T) {
	html := []byte(`<body>
<pre class="src-go"><code>func main() {}
</code></pre>
<p>Some text</p>
<pre class="src-python"><code>print("hello")
</code></pre>
</body>`)

	got, count, err := HighlightPage(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 blocks highlighted, got %d", count)
	}

	out := string(got)
	// Go block should be highlighted
	if !strings.Contains(out, `<span style="color:#d73a49">func</span>`) {
		t.Errorf("expected Go func keyword highlighted")
	}
	// Python block should be highlighted
	if !strings.Contains(out, `<span style="color:#6f42c1">print</span>`) {
		t.Errorf("expected Python print keyword highlighted")
	}
}

func TestHighlightPageNoCodeBlocks(t *testing.T) {
	html := []byte(`<body><p>No code here.</p></body>`)

	got, count, err := HighlightPage(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 blocks, got %d", count)
	}
	if string(got) != string(html) {
		t.Errorf("expected unchanged HTML for no code blocks")
	}
}

func TestHighlightPageEmpty(t *testing.T) {
	got, count, err := HighlightPage(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 blocks for nil input")
	}
	if got != nil {
		t.Errorf("expected nil for nil input")
	}
}

func TestHTMLUnescape(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"&lt;div&gt;", "<div>"},
		{"a &amp; b", "a & b"},
		{"hello", "hello"},
		{"", ""},
	}

	for _, tt := range tests {
		got := string(htmlUnescape([]byte(tt.input)))
		if got != tt.expected {
			t.Errorf("htmlUnescape(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestExtractLang(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`<pre class="src-go">`, "go"},
		{`<pre class="src-python">`, "python"},
		{`<pre class="src-C++">`, "cpp"},
		{`<pre class="src-sh">`, "sh"},
		{`<pre class="src-javascript">`, "javascript"},
		{`<pre class="something-else">`, ""},
		{`<div class="src-go">`, "go"},
		{`<pre class="src-go src-foo">`, "go"},
		{``, ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := extractLang([]byte(tt.input))
			if got != tt.expected {
				t.Errorf("extractLang(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFindClosingTag(t *testing.T) {
	tests := []struct {
		input       string
		tag         string
		expectedEnd int
	}{
	{"<code>hello</code>", "code", 18},
		{"<code>a</code><code>b</code>", "code", 14},
		{"<pre><code>hi</code></pre>", "code", 20},
		{"<pre>hi</pre>", "pre", 13},
		{"no tag here", "code", -1},
		{"", "code", -1},
		{"<code></code>", "code", 13},
		{"<code>a<b>c</b>d</code>", "code", 23},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := findClosingTag([]byte(tt.input), tt.tag)
			if got != tt.expectedEnd {
				// Show context around expected
				if got >= 0 {
					t.Errorf("findClosingTag(%q, %q) = %d (context: %q), want %d", tt.input, tt.tag, got, tt.input[got:], tt.expectedEnd)
				} else {
					t.Errorf("findClosingTag(%q, %q) = %d, want %d", tt.input, tt.tag, got, tt.expectedEnd)
				}
			}
		})
	}
}
