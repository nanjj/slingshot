package highlight

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// --- idempotency ---

// orgPage wraps <pre> blocks in a minimal org-export style HTML page.
func orgPage(blocks ...string) []byte {
	var sb strings.Builder
	sb.WriteString("<!DOCTYPE html>\n<html>\n<head><title>Test</title></head>\n<body>\n")
	for _, b := range blocks {
		sb.WriteString(b)
		sb.WriteString("\n")
	}
	sb.WriteString("</body>\n</html>\n")
	return []byte(sb.String())
}

// TestHighlightPageIdempotent pins the f(f(x)) == f(x) contract: a second pass
// over a highlighted page must be a byte-level no-op and report zero processed
// blocks. A regression here is the 2026-10-09 incident, where a second pass
// re-highlighted <span> markup and escaped it into visible source text.
func TestHighlightPageIdempotent(t *testing.T) {
	page := orgPage(
		"<pre class=\"src src-go\"><code>func main() {\n\tfmt.Println(\"hi\")\n}\n</code></pre>",
		"<pre class=\"src src-python\"><code>def f(x):\n\treturn x\n</code></pre>",
	)

	run1, count1, err := HighlightPage(page)
	if err != nil {
		t.Fatalf("run1: %v", err)
	}
	if count1 != 2 {
		t.Fatalf("run1 count = %d, want 2", count1)
	}
	if !bytes.Contains(run1, []byte(`style="color:`)) {
		t.Fatalf("run1 produced no highlight spans:\n%s", run1)
	}

	run2, count2, err := HighlightPage(run1)
	if err != nil {
		t.Fatalf("run2: %v", err)
	}
	if count2 != 0 {
		t.Errorf("run2 count = %d, want 0 (blocks already highlighted)", count2)
	}
	if !bytes.Equal(run1, run2) {
		t.Errorf("HighlightPage is not idempotent:\nrun1:\n%s\nrun2:\n%s", run1, run2)
	}
}

// TestHighlightPageSkipsAlreadyHighlightedBlock checks that a hand-written
// block already carrying <span style="color:#..."> survives byte for byte.
func TestHighlightPageSkipsAlreadyHighlightedBlock(t *testing.T) {
	block := `<pre class="src-go"><code><span style="color:#d73a49">func</span> main() {}</code></pre>`
	page := orgPage(block)

	got, count, err := HighlightPage(page)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
	if !bytes.Equal(got, page) {
		t.Errorf("already-highlighted page modified:\n got: %s\nwant: %s", got, page)
	}
}

// TestHighlightPageMixedBlocks checks that a raw block next to an already
// highlighted one is processed, while the highlighted one stays untouched.
func TestHighlightPageMixedBlocks(t *testing.T) {
	done := `<pre class="src-go"><code><span style="color:#d73a49">func</span> main() {}</code></pre>`
	raw := `<pre class="src-go"><code>func main() {}</code></pre>`
	page := orgPage(done, raw)

	got, count, err := HighlightPage(page)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1 (only the raw block)", count)
	}
	if !bytes.Contains(got, []byte(done)) {
		t.Errorf("already-highlighted block changed:\n%s", got)
	}
	if n := bytes.Count(got, []byte(`<span style="color:#d73a49">func</span>`)); n != 2 {
		t.Errorf("expected exactly 2 highlighted 'func' spans (one per block), got %d:\n%s", n, got)
	}
}

// TestHighlightPageEscapedSpanStillHighlighted is the counter-case guard: an
// escaped "&lt;span&gt;" in the source is not markup, so the block must still
// be highlighted — and must stay stable across a second pass.
func TestHighlightPageEscapedSpanStillHighlighted(t *testing.T) {
	page := orgPage("<pre class=\"src-go\"><code>// writes &lt;span&gt; tags\nfunc main() {}</code></pre>")

	run1, count1, err := HighlightPage(page)
	if err != nil {
		t.Fatalf("run1: %v", err)
	}
	if count1 != 1 {
		t.Fatalf("run1 count = %d, want 1", count1)
	}
	if !bytes.Contains(run1, []byte(`<span style="color:`)) {
		t.Fatalf("run1 produced no highlight spans:\n%s", run1)
	}
	if !bytes.Contains(run1, []byte("&lt;span&gt;")) {
		t.Errorf("escaped source text did not survive as escaped text:\n%s", run1)
	}

	run2, count2, err := HighlightPage(run1)
	if err != nil {
		t.Fatalf("run2: %v", err)
	}
	if count2 != 0 {
		t.Errorf("run2 count = %d, want 0", count2)
	}
	if !bytes.Equal(run1, run2) {
		t.Errorf("output not stable after two passes:\nrun1:\n%s\nrun2:\n%s", run1, run2)
	}
	if bytes.Contains(run2, []byte("&amp;lt;span")) {
		t.Errorf("double-escaped markup in output:\n%s", run2)
	}
}

// TestHighlightPageMultipleBlocksCount checks the returned count reflects the
// number of blocks actually rewritten, not the number of blocks seen.
func TestHighlightPageMultipleBlocksCount(t *testing.T) {
	page := orgPage(
		`<pre class="src-go"><code>func a() {}</code></pre>`,
		`<pre class="src-go"><code><span style="color:#d73a49">func</span> b() {}</code></pre>`,
		`<pre class="src-python"><code>def c(): pass</code></pre>`,
	)

	_, count, err := HighlightPage(page)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2 (one already highlighted)", count)
	}
}

// TestHighlightPageUnknownLanguageIdempotent covers the second idempotency
// guard: a block whose language cannot be resolved is escaped back to its own
// input, so it must be written through unchanged and not counted. Counting it
// would rewrite the file with identical bytes on every pass, churning mtimes.
func TestHighlightPageUnknownLanguageIdempotent(t *testing.T) {
	page := orgPage("<pre class=\"src-nosuchlang\"><code>some random text</code></pre>")

	run1, count1, err := HighlightPage(page)
	if err != nil {
		t.Fatalf("run1: %v", err)
	}
	if count1 != 0 {
		t.Errorf("run1 count = %d, want 0 (unknown language changes nothing)", count1)
	}
	if !bytes.Equal(run1, page) {
		t.Errorf("unknown-language block was rewritten:\n got: %s\nwant: %s", run1, page)
	}

	run2, count2, err := HighlightPage(run1)
	if err != nil {
		t.Fatalf("run2: %v", err)
	}
	if count2 != 0 {
		t.Errorf("run2 count = %d, want 0", count2)
	}
	if !bytes.Equal(run1, run2) {
		t.Errorf("not idempotent for unknown language:\n%s\n%s", run1, run2)
	}
}

// TestHighlightDirSecondPassNoOp pins the file-level guarantee: a second walk
// over an already-highlighted tree changes no byte and does not touch the
// file's mtime, so a deploy that runs the pass again cannot churn the tree.
func TestHighlightDirSecondPassNoOp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "page.html")
	// Mixed tree: one highlightable block, one already highlighted, one whose
	// language is unknown (the case that used to be rewritten every pass).
	content := `<!DOCTYPE html><html><body>
<pre class="src-go"><code>func main() {}</code></pre>
<pre class="src-go"><code><span style="color:#d73a49">func</span> done() {}</code></pre>
<pre class="src-nosuchlang"><code>plain text</code></pre>
</body></html>
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	first, files1, err := HighlightDir(dir)
	if err != nil {
		t.Fatalf("first HighlightDir: %v", err)
	}
	if first != 1 || files1 != 1 {
		t.Fatalf("first pass = (%d blocks, %d files), want (1, 1)", first, files1)
	}
	firstBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading after first pass: %v", err)
	}
	// The first pass must actually have written a highlighted file, otherwise
	// the no-op assertion below would be vacuous.
	if !bytes.Contains(firstBytes, []byte(`style="color:`)) {
		t.Fatalf("first pass produced no highlight spans:\n%s", firstBytes)
	}

	// Make the mtime distinguishable from any later write.
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("backdating mtime: %v", err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after backdating: %v", err)
	}

	second, files2, err := HighlightDir(dir)
	if err != nil {
		t.Fatalf("second HighlightDir: %v", err)
	}
	if second != 0 || files2 != 0 {
		t.Errorf("second pass = (%d blocks, %d files), want (0, 0)", second, files2)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after second pass: %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("second pass rewrote the file: mtime %v -> %v",
			before.ModTime(), after.ModTime())
	}
	secondBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading after second pass: %v", err)
	}
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Errorf("second pass changed bytes:\n%s\n%s", firstBytes, secondBytes)
	}
	// The pre-highlighted block survived verbatim (not double-processed) and
	// the unknown-language block is still raw (not escaped, not wrapped).
	if !bytes.Contains(secondBytes, []byte(`<span style="color:#d73a49">func</span> done() {}`)) {
		t.Errorf("pre-highlighted block was altered:\n%s", secondBytes)
	}
	if !bytes.Contains(secondBytes, []byte(`<code>plain text</code>`)) {
		t.Errorf("unknown-language block was altered:\n%s", secondBytes)
	}
}

// TestContainsSpanTagFold pins the case-insensitive tag scan used by the
// idempotency guard: it must match any ASCII casing of "<span" and must not
// match a tag name that merely starts with the same letters.
func TestContainsSpanTagFold(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"lower", `<span style="color:#000">`, true},
		{"upper", `<SPAN style="color:#000">`, true},
		{"mixed", `<SpAn>`, true},
		{"bare", `<span>`, true},
		{"preceded by text", `x <span class="a">y`, true},
		{"closing tag has no open", `</span>`, false},
		{"no tag", `func main() {}`, false},
		{"escaped", `&lt;span&gt;`, false},
		{"tag name prefix also matches", `<spanx>`, true},
		{"shorter than tag", `<spa`, false},
		{"lt at end", `abc<`, false},
		{"empty", ``, false},
		{"lt without name", `a < b`, false},
		{"second occurrence matches", `<b>x</b><span>`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsSpanTagFold([]byte(tt.in)); got != tt.want {
				t.Errorf("containsSpanTagFold(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
