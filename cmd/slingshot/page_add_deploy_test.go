package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nanjj/slingshot/internal/config"
)

// plantSiteConfig writes a config.yml under a temporary HOME declaring a single
// page site, so page add can resolve it without reading (or writing) the
// developer's real configuration.
//
// config.Load reads <HOME>/.config/slingshot/config.yml via os.UserHomeDir, so
// pointing HOME at the temp dir is enough on Unix. USERPROFILE covers Windows.
// Returns the site name and its directory.
func plantSiteConfig(t *testing.T, rsyncCmd string) (string, string) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	siteDir := t.TempDir()
	const siteName = "testsite"

	// The rsync value is quoted: an unquoted YAML scalar such as "true" would
	// be parsed as a boolean, and GetSites only accepts a string there.
	cfg := "sites:\n" +
		"  " + siteName + ":\n" +
		"    dir: " + siteDir + "\n" +
		"    type: page\n" +
		"    rsync: \"" + rsyncCmd + "\"\n"

	dir := filepath.Join(home, ".config", "slingshot")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(cfg), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	return siteName, siteDir
}

// TestPageAddRunRsyncChain drives cmdPageAdd.run end to end with --rsync: the
// page is generated, the site index is regenerated, and the configured rsync
// command is invoked from the site directory.
//
// The rsync command is a harmless stub (touch a marker file) so the test never
// reaches the network. doRsync chdirs into the site directory for the duration
// of the call, which is process-global; the test therefore must not run in
// parallel with anything else that depends on the working directory.
func TestPageAddRunRsyncChain(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "rsync-ran.marker")
	stub := "touch " + marker
	siteName, siteDir := plantSiteConfig(t, stub)

	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "post.html")
	body := "<!DOCTYPE html>\n<html><head><title>Post</title></head>" +
		"<body><h1 class=\"title\">Post</h1><p>body</p></body></html>\n"
	if err := os.WriteFile(srcPath, []byte(body), 0644); err != nil {
		t.Fatalf("writing source: %v", err)
	}

	// Keep the command and the colour helper from writing to the real stdout.
	prevColor := color.Output
	color.Output = io.Discard
	t.Cleanup(func() { color.Output = prevColor })

	c := &cmdPageAdd{global: &cmdGlobal{}, flagRsync: true}
	cmd := &cobra.Command{}
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	if err := c.run(cmd, []string{siteName, srcPath}); err != nil {
		t.Fatalf("run: %v", err)
	}

	// The page directory and its index.html were produced.
	pageIndex := filepath.Join(siteDir, "post", "index.html")
	if _, err := os.Stat(pageIndex); err != nil {
		t.Errorf("page index.html missing: %v", err)
	}

	// The site index was regenerated and links to the new page.
	siteIndex, err := os.ReadFile(filepath.Join(siteDir, "index.html"))
	if err != nil {
		t.Fatalf("reading site index: %v", err)
	}
	if !strings.Contains(string(siteIndex), "/post/") {
		t.Errorf("site index does not link the new page:\n%s", siteIndex)
	}

	// The rsync stub actually ran.
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("rsync stub did not run: %v", err)
	}
}

// TestPageAddRunRsyncMissingCommand checks the failure path: a site without an
// rsync command fails the deploy step instead of silently succeeding.
func TestPageAddRunRsyncMissingCommand(t *testing.T) {
	siteName, _ := plantSiteConfig(t, "")

	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "post.html")
	if err := os.WriteFile(srcPath, []byte("<html><body>x</body></html>\n"), 0644); err != nil {
		t.Fatalf("writing source: %v", err)
	}

	prevColor := color.Output
	color.Output = io.Discard
	t.Cleanup(func() { color.Output = prevColor })

	c := &cmdPageAdd{global: &cmdGlobal{}, flagRsync: true}
	cmd := &cobra.Command{}
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := c.run(cmd, []string{siteName, srcPath})
	if err == nil {
		t.Fatal("expected an error when the site has no rsync command")
	}
	if !strings.Contains(err.Error(), "rsync") {
		t.Errorf("error does not mention rsync: %v", err)
	}
}

// sanity check that the helper still resolves what the test planted.
func TestPlantSiteConfig(t *testing.T) {
	siteName, siteDir := plantSiteConfig(t, "true")
	cfg, _, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	site, ok := config.GetSite(cfg, siteName)
	if !ok {
		t.Fatalf("site %q not found in planted config", siteName)
	}
	if site.Dir != siteDir {
		t.Errorf("site dir = %q, want %q", site.Dir, siteDir)
	}
	if site.Rsync != "true" {
		t.Errorf("site rsync = %q, want %q", site.Rsync, "true")
	}
}
