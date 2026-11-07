package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExtractRecordsLocations checks the "#: file:line" data that the .pot
// needs and that the bare msgid set could not carry.
func TestExtractRecordsLocations(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "cmd", "demo")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}

	src := `package demo

func f() {
	_ = i18n.G("same string")
	_ = i18n.G("same string")
	_ = i18n.G("format %s")
}
`
	if err := os.WriteFile(filepath.Join(sub, "demo.go"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	infos, err := extractMsgids(dir)
	if err != nil {
		t.Fatalf("extractMsgids: %v", err)
	}

	info := infos["same string"]
	if info == nil {
		t.Fatal("missing msgid")
	}

	// Two call sites in one file, so two distinct locations.
	if len(info.Locations) != 2 {
		t.Errorf("locations = %v, want 2 entries", info.Locations)
	}
	for _, loc := range info.Locations {
		if loc != "cmd/demo/demo.go:4" && loc != "cmd/demo/demo.go:5" {
			t.Errorf("unexpected location %q", loc)
		}
	}

	if hasFlag(info.Flags, "c-format") {
		t.Errorf("plain string wrongly flagged c-format")
	}

	if !hasFlag(infos["format %s"].Flags, "c-format") {
		t.Errorf("format string not flagged c-format")
	}
}

// TestNeedsCFormat covers the %% case that a naive contains("%") gets wrong.
func TestNeedsCFormat(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"plain", false},
		{"100%% sure", false},
		{"%s", true},
		{"%d items", true},
		{"%%%s", true},
		{"", false},
	} {
		if got := needsCFormat(tc.in); got != tc.want {
			t.Errorf("needsCFormat(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestExtractPlural covers i18n.NG(), which has no call sites yet but must
// not silently lose its plural form once one appears.
func TestExtractPlural(t *testing.T) {
	dir := t.TempDir()
	src := `package p

func f() {
	_ = i18n.NG("one file", "%d files", 3)
}
`
	if err := os.WriteFile(filepath.Join(dir, "p.go"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	infos, err := extractMsgids(dir)
	if err != nil {
		t.Fatalf("extractMsgids: %v", err)
	}

	info := infos["one file"]
	if info == nil {
		t.Fatal("missing msgid")
	}
	if info.Plural != "%d files" {
		t.Errorf("plural = %q, want %q", info.Plural, "%d files")
	}
	if !hasFlag(info.Flags, "c-format") {
		t.Errorf("plural format not flagged c-format")
	}
}
