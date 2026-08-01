// Package base — tests for FTS query expansion (Archimedes feedback, mail #489:
// BM25 did not match camelCase identifiers from natural-language queries).
package base

import (
	"testing"

	"gotest.tools/v3/assert"
)

// TestExpandFTSQuery verifies natural-language queries are OR-extended with
// the camelCase-merged form.
func TestExpandFTSQuery(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"register tool", `("register" AND "tool") OR "registertool"`},
		{"send publish", `("send" AND "publish") OR "sendpublish"`},
		// Single word: no extension needed.
		{"register", "register"},
		// Non-lowercase / FTS syntax: pass through untouched.
		{"RegisterTool", "RegisterTool"},
		{`"exact phrase"`, `"exact phrase"`},
		{"http client", `("http" AND "client") OR "httpclient"`},
	}
	for _, tc := range tests {
		got := expandFTSQuery(tc.in)
		assert.Equal(t, got, tc.want, "expandFTSQuery(%q)", tc.in)
	}
}

// TestSearchNodes_CamelCaseNaturalLanguage verifies that querying
// "register tool" finds the node RegisterTool (indexed as token
// "registertool" by the unicode61 tokenizer).
func TestSearchNodes_CamelCaseNaturalLanguage(t *testing.T) {
	store := indexProjectFixture(t, `package main

// RegisterTool registers a tool for later use.
func RegisterTool(name string) {}

func main() {
	RegisterTool("demo")
}
`)
	// Direct camelCase query works (existing behavior).
	nodes, total, err := store.SearchNodes("RegisterTool", "test-project", "", 10, 0)
	assert.NilError(t, err)
	assert.Assert(t, total >= 1, "camelCase query should match RegisterTool, got %d", total)

	// Natural-language query must match too via the merged form.
	nodes, total, err = store.SearchNodes("register tool", "test-project", "", 10, 0)
	assert.NilError(t, err)
	assert.Assert(t, total >= 1, "natural-language query should match RegisterTool, got %d", total)

	found := false
	for _, n := range nodes {
		if n.QualifiedName == "main.RegisterTool" {
			found = true
		}
	}
	assert.Assert(t, found, "results should include main.RegisterTool, got %v", nodes)
}
