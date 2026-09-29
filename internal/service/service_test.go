package service

import (
	"os"
	"strconv"
	"testing"
)

func TestClaimAndRelease(t *testing.T) {
	t.Setenv("DWF_STATE_DIR", t.TempDir())
	if _, ok := Running(); ok {
		t.Fatal("nothing should be running yet")
	}
	if err := Claim(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(PIDFile())
	if string(b) != strconv.Itoa(os.Getpid()) {
		t.Fatalf("pid file has %q", b)
	}
	// the test binary's name contains "test", not "dwf", so it's treated
	// as a stale file; Claim must still succeed
	if err := Claim(); err != nil {
		t.Fatal(err)
	}
	Release()
	if _, err := os.Stat(PIDFile()); !os.IsNotExist(err) {
		t.Fatal("Release should remove our pid file")
	}
}

func TestStalePIDFile(t *testing.T) {
	t.Setenv("DWF_STATE_DIR", t.TempDir())
	os.MkdirAll(os.Getenv("DWF_STATE_DIR"), 0o755)
	os.WriteFile(PIDFile(), []byte("999999"), 0o644)
	if _, ok := Running(); ok {
		t.Fatal("a dead pid isn't running")
	}
}
