package deepseek

import (
	"testing"

	"github.com/nanjj/slingshot/internal/config"
)

// TestResolveKey covers the three resolution paths: env var, config key,
// and the error when neither is set.
func TestResolveKey(t *testing.T) {
	t.Run("env wins over config", func(t *testing.T) {
		t.Setenv("DEEPSEEK_API_KEY", "from-env")
		cfg := &config.Config{Extra: map[string]any{
			"deepseek": map[string]any{"key": "from-config"},
		}}

		key, err := ResolveKey(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key != "from-env" {
			t.Fatalf("expected from-env, got %q", key)
		}
	})

	t.Run("config fallback", func(t *testing.T) {
		t.Setenv("DEEPSEEK_API_KEY", "")
		cfg := &config.Config{Extra: map[string]any{
			"deepseek": map[string]any{"key": "from-config"},
		}}

		key, err := ResolveKey(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key != "from-config" {
			t.Fatalf("expected from-config, got %q", key)
		}
	})

	t.Run("missing everywhere", func(t *testing.T) {
		t.Setenv("DEEPSEEK_API_KEY", "")
		cfg := &config.Config{Extra: map[string]any{}}

		_, err := ResolveKey(cfg)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
	})
}

// TestSearchRejectsEmptyQuery checks the client-side guard before any
// network call is made.
func TestSearchRejectsEmptyQuery(t *testing.T) {
	c := NewClient("dummy")
	_, err := c.Search(t.Context(), SearchRequest{Query: "  "})
	if err == nil {
		t.Fatal("expected an error for empty query")
	}
}

// TestFetchRejectsEmptyURL checks the same guard for Fetch.
func TestFetchRejectsEmptyURL(t *testing.T) {
	c := NewClient("dummy")
	_, err := c.Fetch(t.Context(), "")
	if err == nil {
		t.Fatal("expected an error for empty URL")
	}
}
