package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPORoundTripExistingFiles guarantees that rewriting the .po files that
// are already in the tree changes nothing. This is the safety net for
// touching the parser and writer at all.
func TestPORoundTripExistingFiles(t *testing.T) {
	dir := filepath.Join("..", "..", "internal", "i18n", "locales")

	for _, locale := range []string{"en_US", "zh_CN"} {
		path := filepath.Join(dir, locale, "slingshot.po")
		want, err := os.ReadFile(path)
		if err != nil {
			t.Skipf("skipping %s: %v", path, err)
		}

		got := writePO(parsePOFull(string(want)))

		// savePO historically emits no trailing newline after the last entry.
		if strings.TrimRight(got, "\n") != strings.TrimRight(string(want), "\n") {
			t.Errorf("%s round-trip mismatch", path)
		}
	}
}

// TestPORoundTripIsIdempotent checks parse→write→parse→write stability for
// entries carrying locations, flags and plural forms.
func TestPORoundTripIsIdempotent(t *testing.T) {
	src := "# translator note\n" +
		"#: cmd/a.go:12 cmd/b.go:7\n" +
		"#, c-format\n" +
		"msgid \"hello %s\"\n" +
		"msgstr \"bonjour %s\"\n" +
		"\n" +
		"msgid \"one file\"\n" +
		"msgid_plural \"%d files\"\n" +
		"msgstr[0] \"un fichier\"\n" +
		"msgstr[1] \"%d fichiers\"\n" +
		"\n" +
		"msgid \"line1\\nline2\"\n" +
		"msgstr \"ligne1\\nligne2\"\n"

	once := writePO(parsePOFull(src))
	twice := writePO(parsePOFull(once))
	if once != twice {
		t.Fatalf("not idempotent:\n--- once ---\n%s\n--- twice ---\n%s", once, twice)
	}

	entries := parsePOFull(src)
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}
	if got := entries[0].Locations; len(got) != 2 || got[0] != "cmd/a.go:12" || got[1] != "cmd/b.go:7" {
		t.Errorf("locations = %v", got)
	}
	if got := entries[0].Flags; len(got) != 1 || got[0] != "c-format" {
		t.Errorf("flags = %v", got)
	}
	if entries[1].MsgidPlural != "%d files" {
		t.Errorf("msgid_plural = %q", entries[1].MsgidPlural)
	}
	if len(entries[1].MsgstrPlural) != 2 || entries[1].MsgstrPlural[1] != "%d fichiers" {
		t.Errorf("msgstr_plural = %v", entries[1].MsgstrPlural)
	}
	if entries[2].Msgid != "line1\nline2" {
		t.Errorf("multiline msgid = %q", entries[2].Msgid)
	}
}

// TestPOEscapedQuoteContinuation covers the strings.Trim bug that the old
// continuation parser had: a value ending in an escaped quote.
func TestPOEscapedQuoteContinuation(t *testing.T) {
	src := "msgid \"\"\n" +
		"\"say \\\"hi\\\"\"\n" +
		"msgstr \"\"\n"

	entries := parsePOFull(src)
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Msgid != `say "hi"` {
		t.Errorf("msgid = %q, want %q", entries[0].Msgid, `say "hi"`)
	}
}
