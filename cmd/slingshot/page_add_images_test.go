package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFile creates parent directories as needed and writes content.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("creating parent of %q: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing %q: %v", path, err)
	}
}

// TestCopyPageImagesNestedRelativePath pins the new sourceDir contract: a
// relative image path is resolved against the source directory and copied into
// the page directory preserving its subdirectory structure.
func TestCopyPageImagesNestedRelativePath(t *testing.T) {
	sourceDir := t.TempDir()
	pageDir := t.TempDir()

	const payload = "PNG-BYTES"
	writeFile(t, filepath.Join(sourceDir, "images", "sub", "pic.png"), payload)

	content := []byte(`<p>x</p><img src="images/sub/pic.png" alt="">`)
	copied, err := copyPageImages(content, sourceDir, pageDir)
	if err != nil {
		t.Fatalf("copyPageImages: %v", err)
	}
	if copied != 1 {
		t.Fatalf("copied = %d, want 1", copied)
	}

	dst := filepath.Join(pageDir, "images", "sub", "pic.png")
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("reading copied image %q: %v", dst, err)
	}
	if string(got) != payload {
		t.Errorf("copied content = %q, want %q", got, payload)
	}
}

// TestCopyPageImagesMissingWarnsAndContinues checks that a missing image is
// reported in the returned error while the remaining images are still copied.
func TestCopyPageImagesMissingWarnsAndContinues(t *testing.T) {
	sourceDir := t.TempDir()
	pageDir := t.TempDir()

	writeFile(t, filepath.Join(sourceDir, "ok.png"), "OK")

	// A missing image is a warning, not a hard failure: the call returns the
	// number of images it did copy together with a descriptive error.
	content := []byte(`<img src="ok.png"><img src="missing.png">`)
	copied, err := copyPageImages(content, sourceDir, pageDir)
	if err == nil {
		t.Fatal("expected a warning error for the missing image")
	}
	if !strings.Contains(err.Error(), "image not found") {
		t.Errorf("warning does not mention the missing image: %v", err)
	}
	if !strings.Contains(err.Error(), "missing.png") {
		t.Errorf("warning does not name the missing file: %v", err)
	}
	if copied != 1 {
		t.Errorf("copied = %d, want 1 (the present image must still be copied)", copied)
	}
	if _, statErr := os.Stat(filepath.Join(pageDir, "ok.png")); statErr != nil {
		t.Errorf("present image was not copied: %v", statErr)
	}
}

// TestCopyPageImagesSkipsRemoteReferences pins that absolute URLs, data URIs
// and protocol-relative references are never treated as local files.
func TestCopyPageImagesSkipsRemoteReferences(t *testing.T) {
	sourceDir := t.TempDir()
	pageDir := t.TempDir()

	content := []byte(`<img src="http://example.com/a.png">` +
		`<img src="https://example.com/b.png">` +
		`<img src="data:image/png;base64,AAAA">` +
		`<img src="//cdn.example.com/c.png">`)

	copied, err := copyPageImages(content, sourceDir, pageDir)
	if err != nil {
		t.Fatalf("copyPageImages: %v", err)
	}
	if copied != 0 {
		t.Errorf("copied = %d, want 0 for remote references", copied)
	}
	entries, err := os.ReadDir(pageDir)
	if err != nil {
		t.Fatalf("reading page dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("page dir gained entries for remote references: %v", entries)
	}
}

// TestCopyPageImagesNoImages covers the short-circuit: content without <img>
// returns (0, nil) and creates nothing.
func TestCopyPageImagesNoImages(t *testing.T) {
	pageDir := t.TempDir()
	copied, err := copyPageImages([]byte("<p>no images here</p>"), t.TempDir(), pageDir)
	if err != nil || copied != 0 {
		t.Errorf("copyPageImages = (%d, %v), want (0, nil)", copied, err)
	}
}
