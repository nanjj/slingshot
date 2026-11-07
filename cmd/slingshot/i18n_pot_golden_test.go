package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPotMatchesIncusGolden pins the one property that matters for the incus
// migration: given the same source files and the same header, the generator
// must reproduce xgettext-go's output byte for byte.
//
// The golden file is incus's own po/incus.pot. It is only checked when the
// incus checkout is present, so the test skips elsewhere rather than failing
// on a machine that does not have it.
func TestPotMatchesIncusGolden(t *testing.T) {
	const incus = "/home/nanjj/.local/src/github.com/lxc/incus"

	golden := filepath.Join(incus, "po", "incus.pot")
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Skipf("incus golden not available: %v", err)
	}

	// The exact file list and header incus passes to xgettext-go today.
	sources := []string{
		"cmd/incus/*.go",
		"cmd/incus/color/*.go",
		"cmd/incus/usage/*.go",
		"shared/cliconfig/*.go",
	}

	infos, err := extractMsgidsFrom(incus, sources)
	if err != nil {
		t.Fatalf("extractMsgidsFrom: %v", err)
	}

	// POT-Creation-Date is taken from the golden file's own header so the
	// comparison does not depend on when the test runs.
	header := potHeader(
		"incus",
		"lxc-devel@lists.linuxcontainers.org",
		"2026-11-07 09:40+0800",
	)

	got := writePot(infos, header)
	if got != string(want) {
		gotPath := filepath.Join(t.TempDir(), "incus.pot")
		if err := os.WriteFile(gotPath, []byte(got), 0644); err != nil {
			t.Fatal(err)
		}
		t.Fatalf("generated pot differs from %s; wrote %s for inspection", golden, gotPath)
	}
}
