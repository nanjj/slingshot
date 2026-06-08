package usage

import "testing"

func TestList(t *testing.T) {
	l := Verbatim("x").List(1)
	p, err := ParseDefault(l, "x", "x", "x")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.List) != 3 {
		t.Errorf("expected 3 items, got %d", len(p.List))
	}
	if p.String != "x x x" {
		t.Errorf("got %q, want %q", p.String, "x x x")
	}
}

func TestListInsufficient(t *testing.T) {
	l := Verbatim("x").List(2)
	_, err := ParseDefault(l, "x") // only 1, needs at least 2
	if err == nil {
		t.Error("expected error for insufficient items")
	}
}
