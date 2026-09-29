// Package logx appends timestamped lines to dwf's log file and, when asked,
// echoes them to the terminal.
package logx

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	mu    sync.Mutex
	path  string
	echo  bool
	nowFn = time.Now
)

// Init sets the log file. With toStdout, lines are also printed.
func Init(file string, toStdout bool) {
	mu.Lock()
	defer mu.Unlock()
	path, echo = file, toStdout
}

// Printf writes one log line.
func Printf(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	line := fmt.Sprintf("%s %s\n", nowFn().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
	if echo {
		fmt.Print(line)
	}
	if path == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(line)
}
