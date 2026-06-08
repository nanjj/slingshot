package usage

import "testing"

func TestPlaceholder(t *testing.T) {
	p := Placeholder("name")

	// matches any value
	parsed, err := ParseDefault(p, "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.String != "hello" {
		t.Errorf("got %q, want %q", parsed.String, "hello")
	}

	// empty args → error
	_, err = ParseDefault(p)
	if err == nil {
		t.Error("expected error for empty args")
	}
}
