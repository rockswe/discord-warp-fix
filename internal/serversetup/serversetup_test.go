package serversetup

import (
	"os/exec"
	"testing"
)

func TestShellQuote(t *testing.T) {
	for _, s := range []string{"plain", "it's", "a b; rm -rf /", `$(x) "y"`} {
		out, err := exec.Command("sh", "-c", "printf %s "+ShellQuote(s)).Output()
		if err != nil || string(out) != s {
			t.Errorf("ShellQuote(%q) round-trips to %q (%v)", s, out, err)
		}
	}
}

func TestRejectsBadInput(t *testing.T) {
	if err := Prepare(Options{TunnelUser: "Bad User", PubKey: "ssh-ed25519 AAAA"}); err == nil {
		t.Error("bad user name must be rejected before connecting")
	}
	if err := Prepare(Options{TunnelUser: "dwf", PubKey: "not a key"}); err == nil {
		t.Error("bad key must be rejected before connecting")
	}
}
