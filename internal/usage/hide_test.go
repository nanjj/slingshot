package usage

import "testing"

func TestHideDelegatesParse(t *testing.T) {
	h := hide{atom: Placeholder("secret"), replacement: Verbatim("****")}
	p, err := ParseDefault(h, "myvalue")
	if err != nil {
		t.Fatal(err)
	}
	if p.String != "myvalue" {
		t.Errorf("got %q, want %q", p.String, "myvalue")
	}
}

func TestHideRendersReplacement(t *testing.T) {
	h := hide{atom: Placeholder("secret"), replacement: Verbatim("****")}
	if h.Render() != "****" {
		t.Errorf("got %q, want %q", h.Render(), "****")
	}
}
