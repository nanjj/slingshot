package deepseek

import (
	"strings"
	"testing"
)

// TestFetchLocalRejectsEmptyURL checks the guard before any exec.
func TestFetchLocalRejectsEmptyURL(t *testing.T) {
	_, err := FetchLocal(t.Context(), "")
	if err == nil {
		t.Fatal("expected an error for empty URL")
	}
	if !strings.Contains(err.Error(), "empty url") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestFetchLocalMissingBinary checks the LookPath guard when lightpanda is
// not on PATH. We clear PATH for the duration of the test.
func TestFetchLocalMissingBinary(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")
	_, err := FetchLocal(t.Context(), "https://example.com")
	if err == nil {
		t.Fatal("expected an error when lightpanda is missing")
	}
	if !strings.Contains(err.Error(), "lightpanda not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}
