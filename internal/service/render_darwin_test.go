package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlistIsValid(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.plist")
	os.WriteFile(p, []byte(Plist("/Users/x/My Tools/dwf & co", "/tmp/l<og>.log")), 0o644)
	if out, err := exec.Command("plutil", "-lint", p).CombinedOutput(); err != nil {
		t.Fatalf("%s", out)
	}
	out, _ := exec.Command("plutil", "-extract", "ProgramArguments.0", "raw", p).Output()
	if strings.TrimSpace(string(out)) != "/Users/x/My Tools/dwf & co" {
		t.Fatalf("path not escaped correctly: %q", out)
	}
}
