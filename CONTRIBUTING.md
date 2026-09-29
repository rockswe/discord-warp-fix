# Contributing

Bug reports and pull requests are welcome, especially from **Windows** and
**Linux desktop** users: those parts are written but haven't been run on a
real machine yet.

## Reporting a problem

Please include:

- your OS and version, and how Discord is installed (official installer, Flatpak, Snap, PTB or Canary);
- the output of `dwf version` and `dwf status`;
- the last lines of `dwf logs`.

`dwf status` shows your WARP exit IP. That's a shared Cloudflare address,
not your home IP, so it's fine to post.

## Building and testing

You need Go 1.26 or newer.

```sh
go build ./cmd/dwf     # or: make build
make test              # unit tests
make lint              # gofmt, go vet for all three OSes, shellcheck
make dist              # cross-compile all release binaries into dist/
```

CI runs the tests on macOS, Linux and Windows. It also turns an Ubuntu
runner into a real tunnel server with `dwf setup --admin`.

## Where things live

| Path | What |
|---|---|
| `cmd/dwf` | entry point |
| `internal/cli` | the commands and the glue between packages |
| `internal/watchdog` | the rules: when to reroll, relaunch or fall back. Pure logic with tests |
| `internal/tunnel` | built-in SSH client and SOCKS5 server |
| `internal/discord` | finding, starting and quitting Discord. **Per-OS files** |
| `internal/service` | starting dwf at login. **Per-OS files** |
| `internal/notify` | desktop notifications. **Per-OS files** |
| `internal/warp` | `warp-cli` wrapper and the reroll loop |
| `internal/logtail`, `internal/burst` | reading Discord's log and spotting 429 bursts |
| `server/setup-server.sh` | prepares the tunnel server (embedded in the binary) |

## Unverified assumptions

These are the educated guesses a real machine should confirm. A PR that
fixes one, or an issue that confirms one works, helps a lot.

### Windows

- [ ] Discord's log is at `%APPDATA%\discord\logs\renderer_js.log` and contains the same `[429]` and `Failed to fetch messages` lines as on macOS.
- [ ] `Update.exe --processStart Discord.exe --process-start-args "--proxy-server=..."` starts Discord with the proxy, and `dwf status` then shows `running through socks5://...`.
- [ ] Discord's traffic really goes through the proxy once started that way. Some Windows tools inject a DLL instead, which may mean the flag alone isn't enough.
- [ ] The main `Discord.exe` process is found (the one without `--type=` on its command line).
- [ ] Quitting by terminating `Discord.exe` doesn't corrupt anything or leave Discord in the tray.
- [ ] `warp-cli.exe` is at `C:\Program Files\Cloudflare\Cloudflare WARP\` and prints `Status update:` and `Tunnel Protocol:` like on macOS.
- [ ] The toast notification in `internal/notify/notify_windows.go` shows up.
- [ ] Starting from the `HKCU\...\Run` key works, and the console window it opens disappears (`FreeConsole`).
- [ ] `install.ps1` works, including when a previous version is running.

### Linux desktop

- [ ] Discord's log is at `~/.config/discord/logs/renderer_js.log`, or under `~/.var/app/com.discordapp.Discord/` for Flatpak.
- [ ] `discord --proxy-server=...` and `flatpak run com.discordapp.Discord --proxy-server=...` both pass the flag through.
- [ ] The systemd user service can start Discord and send `notify-send` notifications on both X11 and Wayland.

## Pull requests

- Keep platform-specific code in the `_darwin.go`, `_linux.go` and `_windows.go` files.
- Run `make lint test` before pushing.
- Say in the PR which OS you tested on and how.
