// Package server embeds setup-server.sh so the dwf binary can prepare a
// server without the repository around.
package server

import (
	"bytes"
	_ "embed"
)

//go:embed setup-server.sh
var raw []byte

// Script prepares a Linux server as a private Discord exit. Carriage
// returns are stripped: a Windows checkout may have converted the file to
// CRLF, and bash on the server would choke on them.
var Script = bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
