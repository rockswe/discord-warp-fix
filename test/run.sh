#!/bin/bash
# shellcheck disable=SC2034  # globals set here are read by the sourced dwf functions
# Unit tests for bin/dwf. Runs under the stock macOS /bin/bash 3.2.
set -u
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf -- "${TMP:?}"' EXIT
export DWF_CONFIG_DIR="$TMP/config" DWF_STATE_DIR="$TMP/state" DWF_LOG_FILE="$TMP/dwf.log" \
       DWF_AGENT_DIR="$TMP/agents" DWF_SHARE_DIR="$TMP/share" DWF_SOURCE_ONLY=1
# shellcheck source=bin/dwf
. "$ROOT/bin/dwf"
NOTIFY=0

pass=0; fail=0
eq() { # eq name expected actual
  if [ "$2" = "$3" ]; then pass=$((pass + 1)); else fail=$((fail + 1)); printf 'FAIL %s: expected [%s] got [%s]\n' "$1" "$2" "$3"; fi
}
ok() { if "${@:2}"; then pass=$((pass + 1)); else fail=$((fail + 1)); printf 'FAIL %s\n' "$1"; fi; }
not() { ! "$@"; }

# etime parsing
eq "etime mm:ss"        65     "$(etime_to_seconds 01:05)"
eq "etime hh:mm:ss"     3725   "$(etime_to_seconds 01:02:05)"
eq "etime dd-hh:mm:ss"  90061  "$(etime_to_seconds 1-01:01:01)"
eq "etime leading 0s"   539    "$(etime_to_seconds 08:59)"

# error counting
eq "count burst fixture" 5 "$(count_errors < "$ROOT/test/fixtures/renderer_burst.log")"
eq "count quiet fixture" 0 "$(count_errors < "$ROOT/test/fixtures/renderer_quiet.log")"

# event window pruning
eq "prune keeps recent" "150 160" "$(prune_events "100 150 160" 120)"
eq "prune empty"        ""        "$(prune_events "" 120)"
eq "word_count"         3         "$(word_count "1 2 3")"
eq "word_count empty"   0         "$(word_count "")"

# watchdog tick: startup skips history, then detects a burst in new lines
DISCORD_LOG="$TMP/renderer_js.log"; THRESHOLD=5; WINDOW=120; COOLDOWN=300; LAST_ACTION=0
cp "$ROOT/test/fixtures/renderer_burst.log" "$DISCORD_LOG"
W_OFFSET=-1; EVENTS=""
ok  "startup ignores old errors"      not watch_tick 1000
eq  "no events after startup"         0 "$(word_count "$EVENTS")"
cat "$ROOT/test/fixtures/renderer_quiet.log" >> "$DISCORD_LOG"
ok  "quiet lines don't trigger"       not watch_tick 1005
cat "$ROOT/test/fixtures/renderer_burst.log" >> "$DISCORD_LOG"
ok  "new burst triggers"              watch_tick 1010
eq  "burst counted"                   5 "$(word_count "$EVENTS")"
LAST_ACTION=1010
ok  "cooldown suppresses repeat"      not watch_tick 1100
ok  "window expires old events"       not watch_tick 1200
eq  "events pruned after window"      0 "$(word_count "$EVENTS")"

# rotation: Discord renames the log and starts a new file
mv "$DISCORD_LOG" "$DISCORD_LOG.old"
cp "$ROOT/test/fixtures/renderer_burst.log" "$DISCORD_LOG"
LAST_ACTION=0
ok  "rotated log is read from start"  watch_tick 2000

# proxy URL per mode
MODE=reroll; eq "proxy url reroll" "" "$(proxy_url)"
MODE=tunnel; SOCKS_PORT=1080; eq "proxy url tunnel" "socks5://127.0.0.1:1080" "$(proxy_url)"
eq "curl uses remote DNS" "socks5h://127.0.0.1:1080" "$(curl_proxy)"
MODE=proxy; PROXY_URL="http://203.0.113.7:3128"; eq "proxy url proxy" "http://203.0.113.7:3128" "$(proxy_url)"

# config round trip, including values with spaces
MODE=tunnel; SSH_HOST="203.0.113.7"; DISCORD_APP="Discord PTB"; THRESHOLD=7
write_config
MODE=""; SSH_HOST=""; DISCORD_APP=""; THRESHOLD=""; DISCORD_LOG=""
load_config
eq "config mode"      tunnel        "$MODE"
eq "config host"      203.0.113.7   "$SSH_HOST"
eq "config app"       "Discord PTB" "$DISCORD_APP"
eq "config threshold" 7             "$THRESHOLD"
eq "config log path"  "$HOME/Library/Application Support/discordptb/logs/renderer_js.log" "$DISCORD_LOG"
eq "config perms"     600 "$(stat -f %Lp "$CONFIG_FILE")"

# launchd plists are valid
ok "watch plist lints"  write_plist "$WATCH_LABEL" watch >/dev/null
ok "tunnel plist lints" write_plist "$TUNNEL_LABEL" tunnel >/dev/null

# the server script's sshd block is accepted by sshd
SSHD_TMP="$TMP/sshd_config"
printf 'HostKey %s/hk\n' "$TMP" > "$SSHD_TMP"
ssh-keygen -q -t ed25519 -N "" -f "$TMP/hk" >/dev/null
sed -n '/^Match User/,/ForceCommand/p' "$ROOT/server/setup-server.sh" \
  | sed "s/\$TUNNEL_USER/dwf/; s#\$NOLOGIN#/usr/bin/false#" >> "$SSHD_TMP"
ok "server sshd block is valid" /usr/sbin/sshd -t -f "$SSHD_TMP"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" = 0 ]
