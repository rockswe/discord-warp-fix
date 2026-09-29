#!/bin/bash
# Installs dwf into ~/.local/bin and the server script into ~/.local/share.
set -eu
cd "$(dirname "$0")"
BIN_DIR="${DWF_BIN_DIR:-$HOME/.local/bin}"
SHARE_DIR="${DWF_SHARE_DIR:-$HOME/.local/share/discord-warp-fix}"

[ "$(uname -s)" = "Darwin" ] || { echo "discord-warp-fix currently supports macOS only." >&2; exit 1; }
command -v warp-cli >/dev/null 2>&1 || [ -x /usr/local/bin/warp-cli ] || \
  echo "warning: warp-cli not found. Install Cloudflare WARP (1.1.1.1) first." >&2

mkdir -p "$BIN_DIR" "$SHARE_DIR"
install -m 755 bin/dwf "$BIN_DIR/dwf"
install -m 755 server/setup-server.sh "$SHARE_DIR/setup-server.sh"
echo "installed $BIN_DIR/dwf"

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo "note: add $BIN_DIR to your PATH, e.g.:  echo 'export PATH=\"$BIN_DIR:\$PATH\"' >> ~/.zshrc" ;;
esac
echo
echo "Next: dwf setup"
