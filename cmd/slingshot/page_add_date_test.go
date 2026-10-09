package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mustDate(t *testing.T, layout, value string) time.Time {
	t.Helper()
	d, err := time.Parse(layout, value)
	if err != nil {
		t.Fatalf("parsing test date %q: %v", value, err)
	}
	return d
}

// TestResolvePageDatePriority pins the clue precedence chain. Every case
// supplies all clues at once; the expected value is the one the documented
// priority must pick.
func TestResolvePageDatePriority(t *testing.T) {
	orgDate := mustDate(t, "2006-01-02", "2001-01-01")
	inputComment := mustDate(t, "2006-01-02", "2002-02-02")
	outputComment := mustDate(t, "2006-01-02", "2003-03-03")
	paraDate := mustDate(t, "2006-01-02", "2004-04-04")
	mtime := mustDate(t, "2006-01-02", "2005-05-05")

	inputWithComment := []byte("<!-- slingshot-date: 2002-02-02 -->\n<html></html>")
	outputWithComment := []byte("<!-- slingshot-date: 2003-03-03 -->\n<html></html>")
	inputWithPara := []byte(`<html><p class="date">April 4, 2004</p></html>`)
	inputPlain := []byte("<html><body>no clues</body></html>")

	tests := []struct {
		name       string
		orgDate    time.Time
		inputHTML  []byte
		outputHTML []byte
		srcModTime time.Time
		want       time.Time
	}{
		{"org date wins", orgDate, inputWithComment, outputWithComment, mtime, orgDate},
		{"input comment over output comment", time.Time{}, inputWithComment, outputWithComment, mtime, inputComment},
		{"output comment inherited on update", time.Time{}, inputPlain, outputWithComment, mtime, outputComment},
		{"p.date fallback", time.Time{}, inputWithPara, nil, mtime, paraDate},
		{"p.date beats mtime", time.Time{}, inputWithPara, nil, mtime, paraDate},
		{"mtime last resort", time.Time{}, inputPlain, nil, mtime, mtime},
		{"no clues at all", time.Time{}, nil, nil, time.Time{}, time.Time{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolvePageDate(tt.orgDate, tt.inputHTML, tt.outputHTML, tt.srcModTime)
			if !got.Equal(tt.want) {
				t.Errorf("resolvePageDate = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestParseDateParagraph covers the lenient layouts accepted from org export's
// postamble, including a value wrapped across lines and one carrying markup.
func TestParseDateParagraph(t *testing.T) {
	want := mustDate(t, "2006-01-02", "2006-01-02")
	tests := []struct {
		name string
		in   string
		want time.Time
	}{
		{"long month", `<p class="date">January 2, 2006</p>`, want},
		{"abbreviated month", `<p class="date">Jan 2, 2006</p>`, want},
		{"iso", `<p class="date">2006-01-02</p>`, want},
		{"weekday prefix", `<p class="date">Monday, January 2, 2006</p>`, want},
		{"attribute before class", `<p id="x" class="date">Jan 2, 2006</p>`, want},
		{"wrapped value", "<p class=\"date\">\n  Jan 2,\n  2006\n</p>", want},
		{"nested markup", `<p class="date"><span>Jan 2, 2006</span></p>`, want},
		{"no paragraph", `<p>Jan 2, 2006</p>`, time.Time{}},
		{"unparseable text", `<p class="date">sometime in 2006</p>`, time.Time{}},
		{"empty body", `<p class="date"></p>`, time.Time{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDateParagraph([]byte(tt.in))
			if !got.Equal(tt.want) {
				t.Errorf("parseDateParagraph(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestEmbedDateCommentIdempotent checks that embedding replaces rather than
// stacks comments, and that the value can be updated in place.
func TestEmbedDateCommentIdempotent(t *testing.T) {
	body := []byte("<html><body>hi</body></html>")
	first := mustDate(t, "2006-01-02", "2020-01-01")
	second := mustDate(t, "2006-01-02", "2021-02-03")

	run1 := embedDateComment(body, first)
	if n := strings.Count(string(run1), "slingshot-date:"); n != 1 {
		t.Fatalf("expected exactly 1 comment, got %d:\n%s", n, run1)
	}
	if !strings.HasPrefix(string(run1), "<!-- slingshot-date: 2020-01-01 -->\n") {
		t.Errorf("unexpected run1 output:\n%s", run1)
	}

	run2 := embedDateComment(run1, first)
	if string(run1) != string(run2) {
		t.Errorf("re-embedding the same date is not a no-op:\n%q\n%q", run1, run2)
	}

	run3 := embedDateComment(run1, second)
	if n := strings.Count(string(run3), "slingshot-date:"); n != 1 {
		t.Errorf("expected exactly 1 comment after update, got %d:\n%s", n, run3)
	}
	if !strings.HasPrefix(string(run3), "<!-- slingshot-date: 2021-02-03 -->\n") {
		t.Errorf("date was not updated:\n%s", run3)
	}
	if !strings.Contains(string(run3), "hi") {
		t.Errorf("body lost during re-embed:\n%s", run3)
	}
}

// TestStripDateComments checks that stray comments anywhere in the document are
// removed without touching surrounding content or leaving blank lines.
func TestStripDateComments(t *testing.T) {
	in := []byte("<html>\n<!-- slingshot-date: 2020-01-01 -->\n<body>\n" +
		"<!-- SLINGSHOT-DATE: 2021-02-03 -->\n</body>\n</html>\n")
	got := string(stripDateComments(in))
	if strings.Contains(strings.ToLower(got), "slingshot-date") {
		t.Errorf("comments survived stripping:\n%s", got)
	}
	if got != "<html>\n<body>\n</body>\n</html>\n" {
		t.Errorf("unexpected stripping result:\n%q", got)
	}
}

// TestPageDatePipelineStable exercises the filesystem-level contract: a source
// with no date clue gets the source mtime frozen into the output, and a second
// pass over the produced page keeps the same value instead of drifting.
func TestPageDatePipelineStable(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "article.html")
	if err := os.WriteFile(srcPath, []byte("<html><body>plain</body></html>"), 0644); err != nil {
		t.Fatalf("writing source: %v", err)
	}
	mtime := mustDate(t, "2006-01-02", "2019-07-04")
	if err := os.Chtimes(srcPath, mtime, mtime); err != nil {
		t.Fatalf("setting mtime: %v", err)
	}

	// First pass: no org date, no input/output comment, no p.date.
	input, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("reading source: %v", err)
	}
	date1 := resolvePageDate(time.Time{}, input, readDateComment(filepath.Join(dir, "out", "index.html")), sourceModTime(srcPath))
	if want := mustDate(t, "2006-01-02", "2019-07-04"); !date1.Equal(want) {
		t.Fatalf("first pass date = %v, want %v", date1, want)
	}
	out1 := embedDateComment(input, date1)
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	outPath := filepath.Join(outDir, "index.html")
	if err := os.WriteFile(outPath, out1, 0644); err != nil {
		t.Fatalf("writing output: %v", err)
	}

	// Second pass: the date now comes from the existing output page.
	date2 := resolvePageDate(time.Time{}, input, readDateComment(outPath), sourceModTime(srcPath))
	if !date2.Equal(date1) {
		t.Errorf("second pass date drifted: %v -> %v", date1, date2)
	}
	out2 := embedDateComment(out1, date2)
	if string(out2) != string(out1) {
		t.Errorf("second pass changed the page:\n%q\n%q", out1, out2)
	}
	if n := strings.Count(string(out2), "slingshot-date:"); n != 1 {
		t.Errorf("expected exactly 1 comment after two passes, got %d", n)
	}
}
