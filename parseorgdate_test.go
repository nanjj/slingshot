package main

import (
	"testing"
	"time"
)

func TestParseOrgDate(t *testing.T) {
	tests := []struct {
		input string
		want  string // YYYY-MM-DD format, empty means expect zero time
	}{
		// Existing formats that must still work
		{"#+DATE: <2007-06-18 Mon>", "2007-06-18"},
		{"#+DATE: <2007-06-18>", "2007-06-18"},
		{"#+DATE: <Monday, 18 June 2007>", "2007-06-18"},
		{"#+DATE: <Mon, 18 Jun 2007>", "2007-06-18"},
		{"#+DATE: <18 June 2007>", "2007-06-18"},
		{"#+DATE: 2007-06-18", "2007-06-18"},
		{"#+DATE: Monday, 18 June 2007", "2007-06-18"},
		{"#+DATE: June 18, 2007", "2007-06-18"},
		{"#+DATE: 2007-06-18 15:04:05", "2007-06-18"},
		// Case-insensitive prefix
		{"#+date: <2007-06-18 Mon>", "2007-06-18"},
		{"#+Date: 2007-06-18", "2007-06-18"},
		// New format: with time
		{"#+DATE: <2017-10-24 Fri 12:30>", "2017-10-24"},
		{"#+DATE: <2017-10-24 12:30>", "2017-10-24"},
		// New format: with time and seconds (bonus)
		{"#+DATE: <2017-10-24 Fri 12:30:00>", "2017-10-24"},
		// Not found cases
		{"#+OTHER: <2017-10-24>", ""},
		{"", ""},
	}

	for _, tt := range tests {
		got := parseOrgDate(tt.input)
		if tt.want == "" {
			if !got.IsZero() {
				t.Errorf("parseOrgDate(%q) = %v, want zero time", tt.input, got)
			}
		} else {
			wantTime, _ := time.Parse("2006-01-02", tt.want)
			if got.Format("2006-01-02") != tt.want {
				t.Errorf("parseOrgDate(%q) = %v (%s), want %s", tt.input, got, got.Format("2006-01-02"), tt.want)
			}
			// Also verify year/month/day match exactly
			if got.Year() != wantTime.Year() || got.Month() != wantTime.Month() || got.Day() != wantTime.Day() {
				t.Errorf("parseOrgDate(%q) date mismatch: got %d-%02d-%02d, want %s",
					tt.input, got.Year(), got.Month(), got.Day(), tt.want)
			}
		}
	}
}
