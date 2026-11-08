package main

import (
	"testing"

	u "github.com/nanjj/slingshot/internal/usage"
)

// TestArgString covers the index and skip checks that every amap subcommand
// relies on. Returning "" instead of an error here would turn a missing
// argument into a query for the empty string, which the API answers with a
// different error far from the cause.
func TestArgString(t *testing.T) {
	parsed := []*u.Parsed{
		{String: "first"},
		{Skipped: true},
		{String: "third"},
	}

	tests := []struct {
		name    string
		i       int
		want    string
		wantErr bool
	}{
		{name: "present", i: 0, want: "first"},
		{name: "skipped is an error", i: 1, wantErr: true},
		{name: "present after a skip", i: 2, want: "third"},
		{name: "past the end", i: 3, wantErr: true},
		{name: "negative index", i: -1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := argString(parsed, tt.i)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("argString(%d) = %q, want an error", tt.i, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("argString(%d) unexpected error: %v", tt.i, err)
			}
			if got != tt.want {
				t.Fatalf("argString(%d) = %q, want %q", tt.i, got, tt.want)
			}
		})
	}

	if _, err := argString(nil, 0); err == nil {
		t.Fatal("argString(nil, 0) = nil error, want one")
	}
}

// TestArgOptionalString covers the same boundary checks, returning ok=false
// rather than an error so callers can apply a default.
func TestArgOptionalString(t *testing.T) {
	parsed := []*u.Parsed{
		{String: "first"},
		{Skipped: true},
		{String: ""},
	}

	tests := []struct {
		name   string
		i      int
		want   string
		wantOK bool
	}{
		{name: "present", i: 0, want: "first", wantOK: true},
		{name: "skipped", i: 1, wantOK: false},
		{name: "present but empty", i: 2, want: "", wantOK: true},
		{name: "past the end", i: 3, wantOK: false},
		{name: "negative index", i: -1, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := argOptionalString(parsed, tt.i)
			if ok != tt.wantOK {
				t.Fatalf("argOptionalString(%d) ok = %v, want %v", tt.i, ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Fatalf("argOptionalString(%d) = %q, want %q", tt.i, got, tt.want)
			}
		})
	}

	if _, ok := argOptionalString(nil, 0); ok {
		t.Fatal("argOptionalString(nil, 0) ok = true, want false")
	}
}
