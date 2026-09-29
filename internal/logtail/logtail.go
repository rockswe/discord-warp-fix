// Package logtail follows Discord's renderer log and counts lines that
// signal rate limiting.
package logtail

import (
	"bytes"
	"errors"
	"io"
	"os"
	"regexp"
)

// DefaultPattern matches the log lines Discord writes when it's rate-limited.
var DefaultPattern = regexp.MustCompile(`\[429\]|Failed to fetch messages`)

const maxRead = 8 << 20 // never read more than 8 MiB per poll

// Tailer remembers how far into the log it has read. The first Poll only
// finds the end of the file, so old errors aren't counted.
type Tailer struct {
	Path    string
	Pattern *regexp.Regexp

	started bool
	info    os.FileInfo
	offset  int64
	partial []byte
}

// Poll returns how many new complete lines match the pattern.
func (t *Tailer) Poll() (int, error) {
	pat := t.Pattern
	if pat == nil {
		pat = DefaultPattern
	}
	fi, err := os.Stat(t.Path)
	if errors.Is(err, os.ErrNotExist) {
		t.info, t.offset, t.partial = nil, 0, nil
		t.started = true
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !t.started {
		t.started, t.info, t.offset = true, fi, fi.Size()
		return 0, nil
	}
	if t.info == nil || !os.SameFile(fi, t.info) || fi.Size() < t.offset {
		t.offset, t.partial = 0, nil // rotated, recreated or truncated
	}
	t.info = fi
	if fi.Size() == t.offset {
		return 0, nil
	}
	f, err := os.Open(t.Path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	start := t.offset
	if fi.Size()-start > maxRead {
		start, t.partial = fi.Size()-maxRead, nil
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return 0, err
	}
	buf, err := io.ReadAll(io.LimitReader(f, fi.Size()-start))
	if err != nil {
		return 0, err
	}
	t.offset = start + int64(len(buf))
	data := append(t.partial, buf...)
	last := bytes.LastIndexByte(data, '\n')
	if last < 0 {
		t.partial = data
		return 0, nil
	}
	t.partial = append([]byte(nil), data[last+1:]...)
	n := 0
	for _, line := range bytes.Split(data[:last], []byte{'\n'}) {
		if pat.Match(line) {
			n++
		}
	}
	return n, nil
}
