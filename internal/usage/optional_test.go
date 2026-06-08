package usage

import "testing"

func TestOptionalPresent(t *testing.T) {
	o := Verbatim("x").Optional()
	p, err := ParseDefault(o, "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Skipped {
		t.Error("expected not skipped")
	}
	if p.String != "x" {
		t.Errorf("got %q, want %q", p.String, "x")
	}
}

func TestOptionalAbsent(t *testing.T) {
	o := Verbatim("x").Optional()
	p, err := ParseDefault(o) // no args
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.Skipped {
		t.Error("expected skipped")
	}
}
