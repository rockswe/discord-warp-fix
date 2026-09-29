// Command dwf keeps Discord working behind Cloudflare WARP's shared,
// rate-limited exit IPs. See README.md.
package main

import (
	"os"

	"github.com/rockswe/erisim/internal/cli"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() { os.Exit(cli.Main(os.Args[1:], version)) }
