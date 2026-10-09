package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/nanjj/slingshot/internal/draft"
)

// writeCachedTokenConfig points HOME at a temporary directory holding a config
// with an already-valid access token, so draft extract's token lookup is served
// from cache and never touches the network or the developer's real config.
func writeCachedTokenConfig(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".config", "slingshot")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}
	cfg := "wechat:\n  access_token: test-token\n  token_expires_at: \"9999999999\"\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(cfg), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}
}

// stubDraftShow points draft.GetURL at an httptest server answering with the
// given news_item body, and restores the previous endpoint on cleanup.
func stubDraftShow(t *testing.T, newsItems string) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"news_item":` + newsItems + `}`))
	}))
	t.Cleanup(ts.Close)

	prev := draft.GetURL
	draft.GetURL = ts.URL
	t.Cleanup(func() { draft.GetURL = prev })
}

// newExtractCmd builds the command with output captured in buffers.
func newExtractCmd() (*cmdDraftExtract, *cobra.Command, *strings.Builder, *strings.Builder) {
	c := &cmdDraftExtract{global: &cmdGlobal{}}
	cmd := &cobra.Command{}
	var out, errBuf strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	return c, cmd, &out, &errBuf
}

// TestDraftExtractRunWritesOutputFile covers the --output path: the parent
// directory is created on demand and the file holds the article content.
func TestDraftExtractRunWritesOutputFile(t *testing.T) {
	writeCachedTokenConfig(t)
	stubDraftShow(t, `[{"title":"T","content":"<p>hello</p>"}]`)

	c, cmd, _, errBuf := newExtractCmd()
	outFile := filepath.Join(t.TempDir(), "nested", "deep", "out.html")
	c.output = outFile

	if err := c.run(cmd, []string{"media-id"}); err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("reading output file: %v", err)
	}
	if string(data) != "<p>hello</p>" {
		t.Errorf("output file = %q, want %q", data, "<p>hello</p>")
	}
	if !strings.Contains(errBuf.String(), outFile) {
		t.Errorf("stderr does not report the output path: %q", errBuf.String())
	}
}

// TestDraftExtractRunStdoutIsClean checks that stdout carries only the raw
// content: no colour escape codes, nothing else.
func TestDraftExtractRunStdoutIsClean(t *testing.T) {
	writeCachedTokenConfig(t)
	stubDraftShow(t, `[{"title":"T","content":"<section>x</section>"}]`)

	c, cmd, out, _ := newExtractCmd()
	if err := c.run(cmd, []string{"media-id"}); err != nil {
		t.Fatalf("run: %v", err)
	}

	if out.String() != "<section>x</section>" {
		t.Errorf("stdout = %q, want raw content", out.String())
	}
	if strings.Contains(out.String(), "\x1b[") {
		t.Errorf("stdout contains ANSI escape codes: %q", out.String())
	}
}

// TestDraftExtractRunNoArticles covers the empty-draft case: a stderr notice
// and a nil error, matching "draft show".
func TestDraftExtractRunNoArticles(t *testing.T) {
	writeCachedTokenConfig(t)
	stubDraftShow(t, `[]`)

	c, cmd, out, errBuf := newExtractCmd()
	if err := c.run(cmd, []string{"media-id"}); err != nil {
		t.Fatalf("run returned error for empty draft: %v", err)
	}
	if out.String() != "" {
		t.Errorf("stdout should be empty, got %q", out.String())
	}
	if errBuf.String() == "" {
		t.Error("stderr missing the no-articles notice")
	}
	if strings.Contains(errBuf.String(), "\x1b[") {
		t.Errorf("stderr notice contains ANSI escape codes: %q", errBuf.String())
	}
}

// TestDraftExtractRunMultiArticleSeparator verifies the header layout survives
// the whole run path, not just the pure formatter.
func TestDraftExtractRunMultiArticleSeparator(t *testing.T) {
	writeCachedTokenConfig(t)
	stubDraftShow(t, `[{"title":"A","content":"<p>a</p>"},{"title":"B","content":"<p>b</p>"}]`)

	c, cmd, out, _ := newExtractCmd()
	if err := c.run(cmd, []string{"media-id"}); err != nil {
		t.Fatalf("run: %v", err)
	}

	want := "<!-- article 1: A -->\n<p>a</p>\n<!-- article 2: B -->\n<p>b</p>"
	if out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
}
