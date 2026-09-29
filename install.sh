#!/bin/sh
# Installs dwf on macOS or Linux into ~/.local/bin.
#
#   curl -fsSL https://raw.githubusercontent.com/rockswe/erisim/main/install.sh | sh
#
# Run from a clone with Go installed and it builds from source instead.
set -eu

REPO="rockswe/erisim"
BIN_DIR="${DWF_BIN_DIR:-$HOME/.local/bin}"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  darwin | linux) ;;
  *) echo "On Windows, use install.ps1 instead." >&2; exit 1 ;;
esac
arch=$(uname -m)
case "$arch" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) echo "Unsupported CPU: $arch" >&2; exit 1 ;;
esac

mkdir -p "$BIN_DIR"
tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT

here=$(cd "$(dirname "$0")" 2>/dev/null && pwd) || here=""
if [ -n "$here" ] && grep -qs 'module github.com/rockswe/erisim' "$here/go.mod" && command -v go >/dev/null 2>&1; then
  version=$(git -C "$here" describe --tags --always --dirty 2>/dev/null || echo dev)
  echo "building dwf $version from source..."
  (cd "$here" && go build -ldflags "-s -w -X main.version=$version" -o "$tmp" ./cmd/dwf)
else
  url="https://github.com/$REPO/releases/latest/download/dwf-$os-$arch"
  echo "downloading $url"
  curl -fsSL "$url" -o "$tmp"
fi
chmod 755 "$tmp"

# The Bash version ran as two launchd agents; stop them before replacing
# the file they point at.
if [ "$os" = darwin ]; then
  for label in com.discord-warp-fix.watch com.discord-warp-fix.tunnel; do
    launchctl bootout "gui/$(id -u)/$label" 2>/dev/null || true
    rm -f "${HOME:?}/Library/LaunchAgents/${label:?}.plist"
  done
fi

mv "$tmp" "$BIN_DIR/dwf"
trap - EXIT
echo "installed $BIN_DIR/dwf ($("$BIN_DIR/dwf" version))"

# Already set up: restart the service so it runs the new binary.
if [ -f "${XDG_CONFIG_HOME:-$HOME/.config}/discord-warp-fix/config" ]; then
  "$BIN_DIR/dwf" install
fi

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *)
    echo
    echo "$BIN_DIR isn't on your PATH yet. Add it with:"
    case "${SHELL:-}" in
      */zsh) rc="$HOME/.zshrc" ;;
      */bash) rc="$HOME/.bashrc" ;;
      *) rc="$HOME/.profile" ;;
    esac
    echo "  echo 'export PATH=\"$BIN_DIR:\$PATH\"' >> $rc && . $rc"
    ;;
esac
echo
echo "Next: dwf setup"
