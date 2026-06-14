package imgconv

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsSVG(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"image.svg", true},
		{"image.SVG", true},
		{"path/to/image.svg", true},
		{"image.png", false},
		{"image.jpg", false},
		{"image", false},
		{"", false},
	}
	for _, tc := range tests {
		got := IsSVG(tc.path)
		if got != tc.want {
			t.Errorf("IsSVG(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestToPNG(t *testing.T) {
	if _, err := exec.LookPath("rsvg-convert"); err != nil {
		t.Skip("rsvg-convert not found, skipping")
	}

	// Create a minimal valid SVG
	svgContent := `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10">
  <rect width="10" height="10" fill="red"/>
</svg>`

	svgPath := filepath.Join(t.TempDir(), "test.svg")
	if err := os.WriteFile(svgPath, []byte(svgContent), 0644); err != nil {
		t.Fatal(err)
	}

	pngPath, err := ToPNG(svgPath)
	if err != nil {
		t.Fatalf("ToPNG failed: %v", err)
	}
	defer os.Remove(pngPath)

	// Verify the output is a PNG file
	data, err := os.ReadFile(pngPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "\x89PNG") {
		t.Errorf("output file is not a PNG (magic bytes: %x)", data[:4])
	}

	// Verify error on nonexistent file
	_, err = ToPNG("/nonexistent/file.svg")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}
