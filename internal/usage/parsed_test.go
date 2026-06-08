package usage

import "testing"

func TestParsedGet(t *testing.T) {
	p := &Parsed{String: "hello", Skipped: false}
	if g := p.Get("default"); g != "hello" {
		t.Errorf("got %q, want %q", g, "hello")
	}

	p2 := &Parsed{Skipped: true}
	if g := p2.Get("default"); g != "default" {
		t.Errorf("got %q, want %q", g, "default")
	}
}

func TestParseString(t *testing.T) {
	p := ParseString("test")
	if p.String != "test" {
		t.Errorf("got %q, want %q", p.String, "test")
	}
}

func TestParseDefault(t *testing.T) {
	v := Verbatim("hello")
	p, err := ParseDefault(v, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if p.String != "hello" {
		t.Errorf("got %q, want %q", p.String, "hello")
	}
}

func TestExplainOnly(t *testing.T) {
	u := Usage{Verbatim("cmd")}
	_, err := u.Parse([]string{"cmd"}, Config{ExplainOnly: true})
	if err != ErrExplainOnly {
		t.Errorf("got %v, want %v", err, ErrExplainOnly)
	}
}
