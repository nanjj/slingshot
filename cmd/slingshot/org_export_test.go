package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/nanjj/slingshot/internal/config"
)

// orgFixture is a small .org document carrying a code block, so the export
// also exercises the syntax-highlight path.
const orgFixture = `#+TITLE: Sample
#+DATE: <2006-01-02>

* Hello

A paragraph.

#+begin_src go
fmt.Println("hi")
#+end_src
`

// requireEmacs skips the test when the org-export dependency is unavailable.
// The export pipeline is the unit under test, so a missing Emacs is an
// environment limitation, not a failure.
func requireEmacs(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("emacs"); err != nil {
		t.Skip("emacs not installed; skipping org export test")
	}
}

// TestOrgToHTMLFileIsolatesSourceDirectory pins the core contract of the
// temporary-directory export: the returned HTML lives outside the source
// directory, the source directory gains no file at all (a pre-existing
// draft-flavoured <name>.html must survive byte for byte), and cleanup removes
// every temporary artefact.
func TestOrgToHTMLFileIsolatesSourceDirectory(t *testing.T) {
	requireEmacs(t)

	srcDir := t.TempDir()
	orgPath := filepath.Join(srcDir, "sample.org")
	if err := os.WriteFile(orgPath, []byte(orgFixture), 0644); err != nil {
		t.Fatalf("writing org source: %v", err)
	}

	// A draft-pipeline artefact sitting next to the org source.
	const marker = "<!-- MARKER: draft version -->\n<html><body>draft</body></html>\n"
	draftHTML := filepath.Join(srcDir, "sample.html")
	if err := os.WriteFile(draftHTML, []byte(marker), 0644); err != nil {
		t.Fatalf("writing draft html: %v", err)
	}

	before := listDir(t, srcDir)

	htmlPath, cleanup, err := orgToHTMLFile(orgPath)
	if err != nil {
		t.Fatalf("orgToHTMLFile: %v", err)
	}

	// The export must not land in (or under) the source directory.
	if strings.HasPrefix(htmlPath, srcDir+string(filepath.Separator)) {
		t.Errorf("exported HTML %q is inside the source directory %q", htmlPath, srcDir)
	}
	if filepath.Dir(htmlPath) == srcDir {
		t.Errorf("exported HTML %q is directly in the source directory", htmlPath)
	}

	exported, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("reading exported HTML: %v", err)
	}
	if !strings.Contains(string(exported), "Hello") {
		t.Errorf("exported HTML does not look like the org export:\n%s", exported)
	}

	// The source directory is untouched: same entries, same draft content.
	after := listDir(t, srcDir)
	if strings.Join(before, ",") != strings.Join(after, ",") {
		t.Errorf("source directory changed:\nbefore: %v\nafter:  %v", before, after)
	}
	got, err := os.ReadFile(draftHTML)
	if err != nil {
		t.Fatalf("reading draft html: %v", err)
	}
	if string(got) != marker {
		t.Errorf("pre-existing <name>.html was modified:\nwant: %q\ngot:  %q", marker, got)
	}

	// Cleanup removes the whole temporary export directory.
	tmpDir := filepath.Dir(htmlPath)
	cleanup()
	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Errorf("temporary directory %q survived cleanup (err=%v)", tmpDir, err)
	}
	if _, err := os.Stat(htmlPath); !os.IsNotExist(err) {
		t.Errorf("exported HTML %q survived cleanup (err=%v)", htmlPath, err)
	}
}

// TestProcessFileOrgDoesNotPolluteSource is the end-to-end counterpart: running
// the page pipeline over an .org file leaves the source directory alone while
// still producing the site page.
func TestProcessFileOrgDoesNotPolluteSource(t *testing.T) {
	requireEmacs(t)

	srcDir := t.TempDir()
	siteDir := t.TempDir()
	orgPath := filepath.Join(srcDir, "article.org")
	if err := os.WriteFile(orgPath, []byte(orgFixture), 0644); err != nil {
		t.Fatalf("writing org source: %v", err)
	}
	const marker = "<!-- MARKER: draft version -->\n"
	draftHTML := filepath.Join(srcDir, "article.html")
	if err := os.WriteFile(draftHTML, []byte(marker), 0644); err != nil {
		t.Fatalf("writing draft html: %v", err)
	}

	c := &cmdPageAdd{}
	cmd := &cobra.Command{}
	if _, err := c.processFile(cmd, orgPath, config.Site{Dir: siteDir}); err != nil {
		t.Fatalf("processFile: %v", err)
	}

	if _, err := os.Stat(filepath.Join(siteDir, "article", "index.html")); err != nil {
		t.Errorf("page index.html was not produced: %v", err)
	}
	got, err := os.ReadFile(draftHTML)
	if err != nil {
		t.Fatalf("reading draft html: %v", err)
	}
	if string(got) != marker {
		t.Errorf("page pipeline overwrote the source-side <name>.html:\nwant: %q\ngot:  %q", marker, got)
	}
	if entries := listDir(t, srcDir); len(entries) != 2 {
		t.Errorf("source directory gained files: %v", entries)
	}
}

// listDir returns the sorted entry names of dir.
func listDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading directory %q: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}
