package usage

import "testing"

func TestSequence(t *testing.T) {
	s := Sequence(Verbatim("add"), Placeholder("name"))
	p, err := ParseDefault(s, "add", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.List) != 2 {
		t.Fatalf("expected 2 parsed, got %d", len(p.List))
	}
	if p.List[0].String != "add" {
		t.Errorf("got %q, want %q", p.List[0].String, "add")
	}
	if p.List[1].String != "hello" {
		t.Errorf("got %q, want %q", p.List[1].String, "hello")
	}
}

func TestMakePath(t *testing.T) {
	s := MakePath(Placeholder("remote"), Placeholder("image"))
	p, err := ParseDefault(s, "default:ubuntu/22.04")
	if err != nil {
		t.Fatal(err)
	}
	if p.String != "default:ubuntu/22.04" {
		t.Errorf("got %q", p.String)
	}
	if len(p.List) != 2 {
		t.Fatalf("expected 2 sub-results, got %d", len(p.List))
	}
}

func TestColon(t *testing.T) {
	c := Colon(Placeholder("name"))
	p, err := ParseDefault(c, "remote:")
	if err != nil {
		t.Fatal(err)
	}
	if p.String != "remote:" {
		t.Errorf("got %q, want %q", p.String, "remote:")
	}
}
