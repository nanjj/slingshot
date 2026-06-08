package usage

import "testing"

func TestFlag(t *testing.T) {
	f := Flag("verbose")

	// found among args
	p, err := ParseDefault(f, "other", "--verbose")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.String != "--verbose" {
		t.Errorf("got %q, want %q", p.String, "--verbose")
	}
}

func TestFlagNotFound(t *testing.T) {
	_, err := ParseDefault(Flag("force"), "other")
	if err == nil {
		t.Error("expected error when flag not in args")
	}
}
