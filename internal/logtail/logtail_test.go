package logtail

import (
	"os"
	"path/filepath"
	"testing"
)

func read(t *testing.T, name string) []byte {
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func appendTo(t *testing.T, path string, b []byte) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(b); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func poll(t *testing.T, tl *Tailer, want int, why string) {
	t.Helper()
	got, err := tl.Poll()
	if err != nil || got != want {
		t.Fatalf("%s: Poll() = %d, %v; want %d", why, got, err, want)
	}
}

func TestTailer(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "renderer_js.log")
	appendTo(t, log, read(t, "burst.log"))

	tl := &Tailer{Path: log}
	poll(t, tl, 0, "startup skips history")
	appendTo(t, log, read(t, "quiet.log"))
	poll(t, tl, 0, "quiet lines")
	appendTo(t, log, read(t, "burst.log"))
	poll(t, tl, 5, "new burst")
	poll(t, tl, 0, "nothing new")

	// a line written in two pieces counts once, when it's complete
	appendTo(t, log, []byte("[x] GET /a [4"))
	poll(t, tl, 0, "half a line")
	appendTo(t, log, []byte("29]\n"))
	poll(t, tl, 1, "line completed")

	// Discord rotates by renaming the log and starting a new one
	if err := os.Rename(log, log+".old"); err != nil {
		t.Fatal(err)
	}
	appendTo(t, log, read(t, "burst.log"))
	poll(t, tl, 5, "rotated log read from the start")

	// truncation
	if err := os.WriteFile(log, []byte("[e] GET /b [429]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	poll(t, tl, 1, "truncated log read from the start")

	// deleted, then recreated
	os.Remove(log)
	poll(t, tl, 0, "missing log")
	appendTo(t, log, read(t, "burst.log"))
	poll(t, tl, 5, "recreated log")
}

func TestMissingAtStartup(t *testing.T) {
	log := filepath.Join(t.TempDir(), "renderer_js.log")
	tl := &Tailer{Path: log}
	poll(t, tl, 0, "no log yet")
	appendTo(t, log, read(t, "burst.log"))
	poll(t, tl, 5, "log appears after startup")
}
