package main

import (
	"strings"
	"testing"

	"github.com/nanjj/slingshot/internal/draft"
)

// TestFormatDraftContents pins the byte layout of "draft extract" output: a
// single article is emitted verbatim so redirection is lossless, while a
// multi-article draft gets one identifying comment per article.
func TestFormatDraftContents(t *testing.T) {
	body1 := "<p>first</p>"
	body2 := "<p>second</p>\n"

	tests := []struct {
		name     string
		articles []draft.ArticleInfo
		want     string
	}{
		{
			name:     "empty",
			articles: nil,
			want:     "",
		},
		{
			name: "single article verbatim",
			articles: []draft.ArticleInfo{
				{Title: "One", Content: body1},
			},
			want: body1,
		},
		{
			name: "two articles get header comments",
			articles: []draft.ArticleInfo{
				{Title: "One", Content: body1},
				{Title: "Two", Content: body2},
			},
			// The first article has no trailing newline, so exactly one is
			// inserted before the second header.
			want: "<!-- article 1: One -->\n" + body1 +
				"\n<!-- article 2: Two -->\n" + body2,
		},
		{
			name: "no extra newline when article already ends with one",
			articles: []draft.ArticleInfo{
				{Title: "One", Content: body2},
				{Title: "Two", Content: body2},
			},
			want: "<!-- article 1: One -->\n" + body2 +
				"<!-- article 2: Two -->\n" + body2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDraftContents(tt.articles)
			if got != tt.want {
				t.Errorf("formatDraftContents = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestFormatDraftContentsSingleArticleIsLossless guards the redirect use case:
// one article must round-trip without any added whitespace.
func TestFormatDraftContentsSingleArticleIsLossless(t *testing.T) {
	content := "<section>no trailing newline</section>"
	got := formatDraftContents([]draft.ArticleInfo{{Title: "T", Content: content}})
	if got != content {
		t.Errorf("single-article output altered: got %q, want %q", got, content)
	}
	if strings.HasSuffix(got, "\n") {
		t.Errorf("single-article output gained a trailing newline: %q", got)
	}
}
