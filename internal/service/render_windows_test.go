package service

import "testing"

func TestCommand(t *testing.T) {
	if got := Command(`C:\Users\x\dwf.exe`); got != `"C:\Users\x\dwf.exe" run --background` {
		t.Fatal(got)
	}
}
