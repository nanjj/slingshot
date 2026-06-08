package usage

import "testing"

func TestVerbatim(t *testing.T) {
	v := Verbatim("add")

	// match
	p, err := ParseDefault(v, "add")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.String != "add" {
		t.Errorf("got %q, want %q", p.String, "add")
	}

	// mismatch
	_, err = ParseDefault(v, "remove")
	if err == nil {
		t.Error("expected error for mismatch")
	}
}
