// Package server embeds setup-server.sh so the dwf binary can prepare a
// server without the repository around.
package server

import _ "embed"

// Script prepares a Linux server as a private Discord exit.
//
//go:embed setup-server.sh
var Script []byte
