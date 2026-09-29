package server

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The Match block setup-server.sh appends must be accepted by sshd.
func TestMatchBlockIsValidSSHDConfig(t *testing.T) {
	sshd, err := exec.LookPath("sshd")
	if err != nil {
		if _, err := os.Stat("/usr/sbin/sshd"); err != nil {
			t.Skip("sshd not installed")
		}
		sshd = "/usr/sbin/sshd"
	}
	block := regexp.MustCompile(`(?s)Match User \$TUNNEL_USER\n.*?ForceCommand \$NOLOGIN`).Find(Script)
	if block == nil {
		t.Fatal("Match block not found in setup-server.sh")
	}
	cfg := strings.NewReplacer("$TUNNEL_USER", "dwf", "$NOLOGIN", "/usr/bin/false").Replace(string(block))
	dir := t.TempDir()
	key := filepath.Join(dir, "hostkey")
	if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key).CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %s", out)
	}
	conf := filepath.Join(dir, "sshd_config")
	os.WriteFile(conf, []byte("HostKey "+key+"\n"+cfg+"\n"), 0o600)
	if out, err := exec.Command(sshd, "-t", "-f", conf).CombinedOutput(); err != nil {
		t.Fatalf("sshd rejected the block: %s\n%s", out, cfg)
	}
}

func TestScriptHasNoCarriageReturns(t *testing.T) {
	if strings.Contains(string(Script), "\r") {
		t.Fatal("the script sent to the server must use LF line endings")
	}
}

func TestScriptLocksDownTheUser(t *testing.T) {
	for _, want := range []string{"restrict,port-forwarding", "AllowTcpForwarding local", "PermitTTY no", "GatewayPorts no", `-t -f "$SSHD_CONFIG"`} {
		if !strings.Contains(string(Script), want) {
			t.Errorf("setup-server.sh no longer contains %q", want)
		}
	}
}
