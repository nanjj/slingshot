package usage

import "testing"

func TestEither(t *testing.T) {
	a := Either(Verbatim("a"), Verbatim("b"), Verbatim("c"))

	// first branch
	p, err := ParseDefault(a, "a")
	if err != nil {
		t.Fatal(err)
	}
	if p.BranchID != 0 {
		t.Errorf("expected BranchID=0, got %d", p.BranchID)
	}

	// second branch
	p, err = ParseDefault(a, "b")
	if err != nil {
		t.Fatal(err)
	}
	if p.BranchID != 1 {
		t.Errorf("expected BranchID=1, got %d", p.BranchID)
	}

	// invalid
	_, err = ParseDefault(a, "z")
	if err == nil {
		t.Error("expected error for invalid input")
	}
}

func TestEitherVerbatim(t *testing.T) {
	a := EitherVerbatim("up", "down")
	p, err := ParseDefault(a, "down")
	if err != nil {
		t.Fatal(err)
	}
	if p.String != "down" {
		t.Errorf("got %q, want %q", p.String, "down")
	}
}
