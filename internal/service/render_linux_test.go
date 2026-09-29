package service

import (
	"strings"
	"testing"
)

func TestUnit(t *testing.T) {
	u := Unit("/home/x/my tools/dwf", map[string]string{"DISPLAY": ":0", "PATH": "/usr/bin"})
	for _, want := range []string{`ExecStart="/home/x/my tools/dwf" run`, `Environment="DISPLAY=:0"`, "Restart=always", "WantedBy=default.target"} {
		if !strings.Contains(u, want) {
			t.Errorf("unit missing %q:\n%s", want, u)
		}
	}
}
