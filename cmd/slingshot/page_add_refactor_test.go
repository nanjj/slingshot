package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/nanjj/slingshot/internal/config"
)

// TestIsValidPageName pins the path-safety contract. The whitelist alone would
// let "." and ".." through, and filepath.Join would then resolve them to the
// site root (clobbering index.html) or to its parent.
func TestIsValidPageName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"article", true},
		{"my-article", true},
		{"my_article", true},
		{"post.2026", true},
		{"A1", true},
		{"", false},
		{".", false},
		{"..", false},
		{"...", false},
		{".hidden", true}, // leading dot is fine as long as it is not dot-only
		{"a.b", true},
		{"a/b", false},
		{"a\\b", false},
		{"/etc/passwd", false},
		{"..\\..\\x", false},
		{"café", false},
		{"a b", false},
		{"a:b", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidPageName(tt.name); got != tt.want {
				t.Errorf("isValidPageName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// TestApplyPageDatePriorityAndStripOrder checks that applyPageDate both picks
// the right clue and strips the <p class="date"> paragraph, whose value can
// only be read before the strip.
func TestApplyPageDatePriorityAndStripOrder(t *testing.T) {
	orgDate := mustDate(t, "2006-01-02", "2001-01-01")

	// The clue is inside the paragraph that must be removed afterwards.
	content := []byte(`<html><body><p class="date">January 2, 2006</p><h1>T</h1></body></html>`)
	got := string(applyPageDate(content, false, "nope.html", t.TempDir(), "page"))

	if !strings.HasPrefix(got, "<!-- slingshot-date: 2006-01-02 -->\n") {
		t.Errorf("p.date clue not frozen into a comment:\n%s", got)
	}
	if strings.Contains(got, `class="date"`) {
		t.Errorf("<p class=\"date\"> paragraph not stripped:\n%s", got)
	}

	// Org date wins over the p.date paragraph.
	gotOrg := string(applyPageDate(content, true, writeOrg(t, orgDate), t.TempDir(), "page"))
	if !strings.HasPrefix(gotOrg, "<!-- slingshot-date: 2001-01-01 -->\n") {
		t.Errorf("org date did not win:\n%s", gotOrg)
	}
}

// TestApplyPageDateIdempotent checks that re-applying the date replaces the
// existing comment instead of stacking a second one, and is a no-op when the
// value is already correct.
func TestApplyPageDateIdempotent(t *testing.T) {
	dir := t.TempDir()
	content := []byte("<!DOCTYPE html>\n<html><body>hi</body></html>\n")
	src := writeOrgWithModTime(t, dir, mustDate(t, "2006-01-02", "2020-01-01"))

	run1 := applyPageDate(content, false, src, dir, "page")
	if n := strings.Count(string(run1), "slingshot-date:"); n != 1 {
		t.Fatalf("expected 1 comment after first run, got %d:\n%s", n, run1)
	}

	// Second run over the produced content must not add a comment.
	run2 := applyPageDate(run1, false, src, dir, "page")
	if n := strings.Count(string(run2), "slingshot-date:"); n != 1 {
		t.Errorf("expected 1 comment after second run, got %d:\n%s", n, run2)
	}
	if string(run1) != string(run2) {
		t.Errorf("applyPageDate is not idempotent:\n%q\n%q", run1, run2)
	}
}

// TestProcessFileDateStableAcrossUpdate is the end-to-end contract: adding a
// page freezes the source mtime, and updating the same source after touching
// its mtime keeps the original date (inherited from the page on disk).
func TestProcessFileDateStableAcrossUpdate(t *testing.T) {
	siteDir := t.TempDir()
	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "article.html")

	origMod := mustDate(t, "2006-01-02", "2019-07-04")
	body := "<!DOCTYPE html>\n<html><head><title>Article</title></head>" +
		"<body><h1 class=\"title\">Article</h1><p>body</p></body></html>\n"
	if err := os.WriteFile(srcPath, []byte(body), 0644); err != nil {
		t.Fatalf("writing source: %v", err)
	}
	if err := os.Chtimes(srcPath, origMod, origMod); err != nil {
		t.Fatalf("setting source mtime: %v", err)
	}

	c := &cmdPageAdd{}
	siteConfig := config.Site{Dir: siteDir}
	cmd := &cobra.Command{}

	// First add: no clue anywhere, so the source mtime is frozen.
	if _, err := c.processFile(cmd, srcPath, siteConfig); err != nil {
		t.Fatalf("first processFile: %v", err)
	}
	outPath := filepath.Join(siteDir, "article", "index.html")
	first := readAll(t, outPath)
	if !strings.HasPrefix(first, "<!-- slingshot-date: 2019-07-04 -->\n") {
		t.Fatalf("first add did not freeze the source mtime:\n%s", first)
	}

	// Touch the source: an update must keep the published date, not drift.
	later := mustDate(t, "2006-01-02", "2026-10-09")
	if err := os.Chtimes(srcPath, later, later); err != nil {
		t.Fatalf("touching source mtime: %v", err)
	}
	c.update = true
	if _, err := c.processFile(cmd, srcPath, siteConfig); err != nil {
		t.Fatalf("second processFile: %v", err)
	}
	second := readAll(t, outPath)
	if !strings.HasPrefix(second, "<!-- slingshot-date: 2019-07-04 -->\n") {
		t.Errorf("date drifted on update:\n%s", second)
	}
	if n := strings.Count(second, "slingshot-date:"); n != 1 {
		t.Errorf("expected 1 comment after update, got %d", n)
	}
}

// --- helpers ---

func readAll(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %q: %v", path, err)
	}
	return string(data)
}

// writeOrg writes a minimal .org file carrying the given date into a temp dir.
func writeOrg(t *testing.T, date time.Time) string {
	t.Helper()
	return writeOrgWithModTime(t, t.TempDir(), date)
}

// writeOrgWithModTime writes a minimal .org file with an explicit mtime.
func writeOrgWithModTime(t *testing.T, dir string, mod time.Time) string {
	t.Helper()
	path := filepath.Join(dir, "src.org")
	content := "#+TITLE: T\n#+DATE: <" + mod.Format("2006-01-02") + ">\n\n* Body\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing org file: %v", err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatalf("setting org mtime: %v", err)
	}
	return path
}
