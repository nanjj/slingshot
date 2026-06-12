package mdtowx

import (
	"os"
	"strings"
	"testing"
)

// TestSanitizeHTML_footnoteDefinition verifies that footnote definition
// anchors (<a id="fn.X" href="#fnr.X">) are stripped, not just footnote
// backlinks (<a role="doc-backlink" class="footref">).
func TestSanitizeHTML_footnoteDefinition(t *testing.T) {
	src := []byte(`<p>正文<sup><a id="fnr.1" class="footref" href="#fn.1" role="doc-backlink">1</a></sup>内容。</p>
<p><sup><a id="fn.1" href="#fnr.1">1</a></sup>脚注正文。</p>`)

	got := string(SanitizeHTML(src))

	// Both types of anchors should be removed
	if strings.Contains(got, `id="fnr.1"`) {
		t.Errorf("footnote backlink anchor not removed:\n%s", got)
	}
	if strings.Contains(got, `id="fn.1"`) {
		t.Errorf("footnote definition anchor not removed:\n%s", got)
	}

	// But the numbers should remain as plain text
	if !strings.Contains(got, "1") {
		t.Errorf("footnote number 1 missing from output:\n%s", got)
	}
}

// TestSanitizeHTML_realFile runs the sanitizer on the actual
// rsync-and-outrage HTML to verify no footnote anchors survive.
func TestSanitizeHTML_realFile(t *testing.T) {
	data, err := os.ReadFile("/home/nanjj/.local/src/gitcode.com/nanjunjie/nanjunjie/rsync-and-outrage/rsync-and-outrage.html")
	if err != nil {
		t.Skipf("real HTML file not available: %v", err)
	}

	got := string(SanitizeHTML(data))

	// Check for any id="fn. or id="fnr. anchors
	if strings.Contains(got, `id="fn.`) {
		t.Errorf("footnote definition anchor survived (id=\"fn.\")")
	}
	if strings.Contains(got, `id="fnr.`) {
		t.Errorf("footnote backlink anchor survived (id=\"fnr.\")")
	}
	if strings.Contains(got, "doc-backlink") {
		t.Errorf("doc-backlink role survived")
	}
	if strings.Contains(got, "footref") {
		t.Errorf("footref class survived")
	}
}
