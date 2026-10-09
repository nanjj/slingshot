package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const rsyncTestPage = `<!DOCTYPE html>
<html><head><title>T</title></head><body>
<pre class="src-go"><code>func main() {}</code></pre>
</body></html>
`

// writeRsyncTestPage creates an unhighlighted HTML page inside dir.
func writeRsyncTestPage(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "index.html")
	if err := os.WriteFile(path, []byte(rsyncTestPage), 0644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	return path
}

// TestCmdSiteRsyncRunHighlightDefault checks that the default branch runs the
// highlighter and that a second pass over the same tree is a no-op (idempotent).
func TestCmdSiteRsyncRunHighlightDefault(t *testing.T) {
	dir := t.TempDir()
	path := writeRsyncTestPage(t, dir)

	c := &cmdSiteRsync{}
	count, err := c.runHighlight(dir)
	if err != nil {
		t.Fatalf("runHighlight: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading result: %v", err)
	}
	if !strings.Contains(string(data), `style="color:`) {
		t.Errorf("expected highlighted output, got:\n%s", data)
	}

	count, err = c.runHighlight(dir)
	if err != nil {
		t.Fatalf("second runHighlight: %v", err)
	}
	if count != 0 {
		t.Errorf("second pass count = %d, want 0 (idempotent)", count)
	}
}

// TestCmdSiteRsyncRunHighlightDisabled checks the --no-highlight escape hatch:
// the tree must be left byte-identical and the count reported as zero, so no
// count line is printed by the caller.
func TestCmdSiteRsyncRunHighlightDisabled(t *testing.T) {
	dir := t.TempDir()
	path := writeRsyncTestPage(t, dir)

	c := &cmdSiteRsync{flagNoHighlight: true}
	count, err := c.runHighlight(dir)
	if err != nil {
		t.Fatalf("runHighlight: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 when --no-highlight is set", count)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading result: %v", err)
	}
	if string(data) != rsyncTestPage {
		t.Errorf("--no-highlight must not rewrite the tree, got:\n%s", data)
	}
}
