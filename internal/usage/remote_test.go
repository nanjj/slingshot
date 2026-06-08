package usage

import "testing"

func TestMakeRemote(t *testing.T) {
	r := MakeRemote(Placeholder("remote"), false)
	p, err := ParseDefault(r, "default:")
	if err != nil {
		t.Fatal(err)
	}
	if p.RemoteName != "default" {
		t.Errorf("got %q, want %q", p.RemoteName, "default")
	}
}

func TestRemoteOptional(t *testing.T) {
	r := MakeRemote(Placeholder("remote"), true)
	p, err := ParseDefault(r, "no-colon")
	if err != nil {
		t.Fatal(err)
	}
	if !p.Skipped {
		t.Error("expected skipped when no colon")
	}
}

func TestRemoteColon(t *testing.T) {
	p, err := ParseDefault(RemoteColon, "myserver:")
	if err != nil {
		t.Fatal(err)
	}
	if p.RemoteName != "myserver" {
		t.Errorf("got %q, want %q", p.RemoteName, "myserver")
	}
}

func TestRemoteColonOpt(t *testing.T) {
	// with colon
	p, err := ParseDefault(RemoteColonOpt, "myserver:")
	if err != nil {
		t.Fatal(err)
	}
	if p.RemoteName != "myserver" {
		t.Errorf("got %q, want %q", p.RemoteName, "myserver")
	}

	// without colon → optional skipped
	q, err := ParseDefault(RemoteColonOpt, "plainvalue")
	if err != nil {
		t.Fatal(err)
	}
	if !q.Skipped {
		t.Error("expected skipped when no colon")
	}
}
