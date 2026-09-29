#!/bin/bash
# Prepares a Linux server (Debian/Ubuntu and most systemd distros) as a private
# Discord exit for discord-warp-fix.
#
# It creates a locked-down user that can ONLY open outbound TCP forwards
# (what `ssh -D` needs): no shell, no TTY, no agent/X11 forwarding, no
# listening ports on the server.
#
# Usage (as root):  DWF_USER=dwf bash setup-server.sh 'ssh-ed25519 AAAA... comment'
set -euo pipefail

PUBKEY="${1:-}"
TUNNEL_USER="${DWF_USER:-dwf}"
SSHD_CONFIG="${DWF_SSHD_CONFIG:-/etc/ssh/sshd_config}"
MARK="# discord-warp-fix"

die() { echo "setup-server: $*" >&2; exit 1; }

[ "$(id -u)" = 0 ] || die "run as root (or with sudo)"
case "$PUBKEY" in
  ssh-ed25519\ *|ssh-rsa\ *|ecdsa-sha2-nistp*\ *) ;;
  *) die "first argument must be the client's public key (ssh-ed25519 AAAA...)" ;;
esac
case "$TUNNEL_USER" in
  *[!a-z0-9_-]*|"") die "DWF_USER may only contain a-z, 0-9, _ and -" ;;
esac
command -v sshd >/dev/null 2>&1 || [ -x /usr/sbin/sshd ] || die "OpenSSH server isn't installed"
SSHD="$(command -v sshd || echo /usr/sbin/sshd)"
NOLOGIN="$(command -v nologin || echo /usr/sbin/nologin)"

if ! id "$TUNNEL_USER" >/dev/null 2>&1; then
  useradd --system --create-home --shell "$NOLOGIN" "$TUNNEL_USER"
  echo "created user $TUNNEL_USER"
fi
# '*' = no usable password but NOT locked, so sshd still accepts the key.
usermod -p '*' "$TUNNEL_USER"

HOME_DIR="$(getent passwd "$TUNNEL_USER" | cut -d: -f6)"
install -d -m 700 -o "$TUNNEL_USER" -g "$(id -gn "$TUNNEL_USER")" "$HOME_DIR/.ssh"
AUTH="$HOME_DIR/.ssh/authorized_keys"
LINE="restrict,port-forwarding $PUBKEY"
touch "$AUTH"
grep -qxF "$LINE" "$AUTH" || echo "$LINE" >> "$AUTH"
chown "$TUNNEL_USER:$(id -gn "$TUNNEL_USER")" "$AUTH"
chmod 600 "$AUTH"

if ! grep -qF "$MARK begin ($TUNNEL_USER)" "$SSHD_CONFIG"; then
  cp "$SSHD_CONFIG" "$SSHD_CONFIG.bak.discord-warp-fix"
  cat >> "$SSHD_CONFIG" <<CONF

$MARK begin ($TUNNEL_USER)
Match User $TUNNEL_USER
    AllowTcpForwarding local
    PermitTTY no
    X11Forwarding no
    AllowAgentForwarding no
    PermitTunnel no
    GatewayPorts no
    ForceCommand $NOLOGIN
$MARK end ($TUNNEL_USER)
CONF
  echo "added a Match block for $TUNNEL_USER to $SSHD_CONFIG"
fi

"$SSHD" -t -f "$SSHD_CONFIG" || die "sshd rejected the config; your original is at $SSHD_CONFIG.bak.discord-warp-fix"

if command -v systemctl >/dev/null 2>&1 && systemctl list-units --type=service 2>/dev/null | grep -qE '^\s*(ssh|sshd)\.service'; then
  systemctl reload ssh 2>/dev/null || systemctl reload sshd
elif command -v service >/dev/null 2>&1; then
  service ssh reload 2>/dev/null || service sshd reload 2>/dev/null || true
fi

echo "done. The Mac can now run: dwf check"
