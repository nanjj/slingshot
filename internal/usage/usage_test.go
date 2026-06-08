package usage

import "testing"

func TestUsageParse(t *testing.T) {
	u := Usage{
		Verbatim("add"),
		Placeholder("name"),
	}

	parsed, err := u.Parse([]string{"add", "myfile"})
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != 2 {
		t.Fatalf("expected 2 results, got %d", len(parsed))
	}
	if parsed[0].String != "add" {
		t.Errorf("got %q, want %q", parsed[0].String, "add")
	}
	if parsed[1].String != "myfile" {
		t.Errorf("got %q, want %q", parsed[1].String, "myfile")
	}
}

func TestUsageTooManyArgs(t *testing.T) {
	u := Usage{Verbatim("cmd")}
	_, err := u.Parse([]string{"cmd", "extra"})
	if err == nil {
		t.Fatal("expected error for too many arguments")
	}
}

func TestUsageNotEnoughArgs(t *testing.T) {
	u := Usage{Placeholder("req")}
	_, err := u.Parse(nil)
	if err == nil {
		t.Fatal("expected error for not enough arguments")
	}
}

func TestPredefinedAtoms(t *testing.T) {
	tests := []struct {
		name string
		atom Atom
		args []string
	}{
		{"File", File, []string{"main.go"}},
		{"ID", ID, []string{"abc123"}},
		{"Key", Key, []string{"mykey"}},
		{"Value", Value, []string{"myvalue"}},
		{"Name", Name, []string{"testname"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := ParseDefault(tt.atom, tt.args...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p.String != tt.args[0] {
				t.Errorf("got %q, want %q", p.String, tt.args[0])
			}
		})
	}
}
