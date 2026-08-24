package site

import (
	"os"
	"strings"
	"testing"
)

// TestDefaultCSSTableFrame ensures org-exported tables keep their rules.
// ox-html drops the presentational attributes (border/rules/frame) for
// HTML5 doctype exports, so DefaultCSS must restore them via CSS:
// frame="hsides" → top/bottom borders, rules="groups" → thead rule,
// cellpadding="6" → cell padding.
func TestDefaultCSSTableFrame(t *testing.T) {
	css := DefaultCSS()

	checks := []struct {
		name string
		frag string
	}{
		{"table border-collapse", "table { border-collapse:collapse; }"},
		{"table top rule", "border-top: 1px solid #000;"},
		{"table bottom rule", "border-bottom: 1px solid #000;"},
		{"thead group rule", "thead th {\n  border-bottom: 1px solid #000;"},
		{"cell padding (cellpadding=6)", "th, td { padding: 6px; }"},
		{"sentinel v2", "/* === slingshot-responsive: v2 === */"},
	}
	for _, c := range checks {
		if !strings.Contains(css, c.frag) {
			t.Errorf("DefaultCSS missing %s: %q", c.name, c.frag)
		}
	}
	// Bump CSSVersion whenever DefaultCSS() changes so cached stylesheets refresh.
	if CSSVersion != "4" {
		t.Errorf("CSSVersion = %q, want 4 (bump on style change for cache-busting)", CSSVersion)
	}
}

// TestUpgradeCSSFromV1 verifies that a site CSS with the v1 sentinel is
// upgraded to the latest DefaultCSS, so existing sites pick up new rules.
func TestUpgradeCSSFromV1(t *testing.T) {
	dir := t.TempDir()
	old := "/* === slingshot-responsive: v1 === */\nbody { font-family: sans-serif; }\n"
	if err := os.WriteFile(dir+"/style.css", []byte(old), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	upgraded, err := UpgradeCSS(dir, false)
	if err != nil {
		t.Fatalf("UpgradeCSS: %v", err)
	}
	if !upgraded {
		t.Fatal("UpgradeCSS did not upgrade v1 CSS")
	}
	css, err := os.ReadFile(dir + "/style.css")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(css), "slingshot-responsive: v2") {
		t.Errorf("upgraded CSS missing v2 sentinel; got head: %q", css[:60])
	}
}
