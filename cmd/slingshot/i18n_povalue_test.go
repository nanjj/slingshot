package main

import (
	"strings"
	"testing"
)

// TestWritePOValue covers both shapes the writer emits. split=false is the
// .po form (one quoted line, never broken); split=true is the .pot form (first
// segment on the keyword line, the rest indented). Getting the split wrong
// silently changes the template's byte layout, which is what msgmerge reads.
func TestWritePOValue(t *testing.T) {
	tests := []struct {
		name    string
		keyword string
		value   string
		split   bool
		want    string
	}{
		{
			name:    "single line, no split",
			keyword: "msgid",
			value:   "hello",
			split:   false,
			want:    "msgid \"hello\"\n",
		},
		{
			name:    "multi line but split disabled stays on one line",
			keyword: "msgid",
			value:   "a\nb",
			split:   false,
			want:    "msgid \"a\\nb\"\n",
		},
		{
			name:    "single line with split enabled stays on one line",
			keyword: "msgstr",
			value:   "x",
			split:   true,
			want:    "msgstr \"x\"\n",
		},
		{
			// xgettext's layout: an empty first segment, then one quoted line
			// per source line. The empty segment is what makes msgmerge read
			// the value as a continuation block rather than a one-line string.
			name:    "multi line with split",
			keyword: "msgid",
			value:   "a\nb",
			split:   true,
			want:    "msgid \"\"\n\"a\\n\"\n\"b\"\n",
		},
		{
			name:    "trailing newline does not produce an empty segment",
			keyword: "msgid",
			value:   "a\n",
			split:   true,
			want:    "msgid \"\"\n\"a\\n\"\n",
		},
		{
			name:    "quotes are escaped",
			keyword: "msgid",
			value:   `say "hi"`,
			split:   false,
			want:    "msgid \"say \\\"hi\\\"\"\n",
		},
		{
			name:    "keyword padded to column 8 when it is short",
			keyword: "msgid",
			value:   "",
			split:   false,
			want:    "msgid \"\"\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			writePOValue(&b, tt.keyword, tt.value, tt.split)
			if got := b.String(); got != tt.want {
				t.Fatalf("writePOValue(%q, %q, %v) = %q, want %q",
					tt.keyword, tt.value, tt.split, got, tt.want)
			}
		})
	}
}
