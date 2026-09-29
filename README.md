# discord-warp-fix

Keeps Discord working on macOS when you reach it through Cloudflare WARP and
keep getting **"Messages failed to load"**.

[Türkçe açıklama aşağıda](#türkçe)

## Why this happens

When Discord is blocked where you live, WARP (the free 1.1.1.1 app) gets you
past the block. But WARP doesn't give you your own IP address. Everyone
connected to the same Cloudflare data center leaves the tunnel from a tiny
pool of shared exit addresses. In Istanbul, that pool was just two IPs in our
testing.

Discord limits how many requests a single IP may send. Thousands of people
behind one address blow through that limit, so Discord answers with
**HTTP 429 Too Many Requests**. The desktop app turns that into "Messages
failed to load", failed profile loads, and gateway reconnects. It "sometimes
works randomly" because the limit is a rolling window shared with strangers.

You can confirm this on your own Mac:

```sh
grep -cE '\[429\]|Failed to fetch messages' \
  ~/Library/Application\ Support/discord/logs/renderer_js.log
curl -s https://www.cloudflare.com/cdn-cgi/trace | grep ip=
```

Things that **don't** help: changing DNS, switching WARP between MASQUE and
WireGuard permanently, re-registering WARP, or IPv6 (Discord has no IPv6
addresses, and WARP presents the same shared IPv4 exit anyway).

## What this does

`dwf` is one Bash script plus two launchd agents. Pick a mode:

| Mode | Cost | What it does | Fixes it for good? |
|---|---|---|---|
| `reroll` | free | Watches Discord's log. On a burst of 429s it flips WARP's protocol until you land on a different shared exit IP, and notifies you. | No. It helps while one exit is quieter than the other. |
| `tunnel` | a small server abroad | Runs a permanent SSH tunnel to a server you control and relaunches Discord through it. Discord then uses that server's IP, which nobody else shares. | **Yes** |
| `proxy` | a proxy you already have | Same as `tunnel`, but through an existing SOCKS5 or HTTP proxy. | Yes, if the proxy IP isn't shared |

In `tunnel` and `proxy` modes the watchdog also:

- relaunches Discord through the proxy when it starts on its own, at login or after a self-update;
- falls back to plain WARP if the proxy stops working for two minutes, and tells you when it's back;
- restarts the tunnel automatically if the connection drops.

Keep WARP connected in every mode. It still carries the rest of your traffic,
Discord voice, and the SSH connection to your server.

## Install

Requirements: macOS, Cloudflare WARP, the Discord desktop app.

```sh
git clone https://github.com/rockswe/discord-warp-fix.git
cd discord-warp-fix
./install.sh          # puts dwf in ~/.local/bin
dwf setup             # choose a mode
dwf install           # start the background agents
```

For `tunnel` or `proxy` mode, finish with:

```sh
dwf launch            # relaunch Discord through the proxy
```

### Tunnel mode: getting a server

Any Linux server outside your country works, like a $4–5/month VPS from
providers such as Hetzner or DigitalOcean, or a free-tier VM. It only relays
Discord's text traffic, so the smallest plan is plenty.

`dwf setup` generates an SSH key and can prepare the server for you if you
give it an admin login (for example `root`). If you'd rather do it yourself,
copy `server/setup-server.sh` to the server and run:

```sh
sudo env DWF_USER=dwf bash setup-server.sh 'ssh-ed25519 AAAA... (the key dwf printed)'
```

The script creates a user that can **only** open outbound connections. It has
no shell, no terminal, and it can't listen on ports, so a leaked key can't be
used to log in to the server.

## Commands

```
dwf status            WARP exit IP, whether Discord uses the proxy, recent errors, agents
dwf check             test the configured mode end to end
dwf launch            relaunch Discord through the proxy
dwf launch --direct   relaunch Discord without the proxy
dwf reroll            move WARP to a different exit IP right now
dwf logs              what the watchdog has been doing
dwf uninstall         remove the agents (add --purge to delete the config)
```

Settings live in `~/.config/discord-warp-fix/config`. Run `dwf install`
again after editing it.

| Setting | Default | Meaning |
|---|---|---|
| `THRESHOLD` / `WINDOW` | `5` / `120` | errors within that many seconds that count as a rate-limit burst |
| `COOLDOWN` | `300` | seconds between automatic actions |
| `AUTO_RELAUNCH` | `1` | move a freshly started Discord onto the proxy |
| `FALLBACK_AFTER` | `120` | seconds of proxy failure before falling back to WARP (`0` = only notify) |
| `DISCORD_APP` | `Discord` | or `Discord PTB` / `Discord Canary` |
| `NOTIFY` | `1` | macOS notifications |

## Limitations

- **macOS only.** Windows and Linux aren't supported yet.
- **Voice isn't proxied.** Discord's voice engine ignores proxy settings, so calls keep going through WARP. That's fine, because voice servers aren't what gets rate-limited.
- **Relaunches take a few seconds.** `dwf launch`, auto-relaunch and fallback all quit and reopen Discord. Automatic relaunches only touch a Discord that started in the last three minutes, or one whose proxy is dead.
- **`reroll` can't create new exits.** When every shared exit is overloaded, it can only notify you.
- **Discord's updater** makes a few connections outside the proxy. They're rare and don't affect rate limits.

## How it was tested

- 32 unit tests (`make test`) run under the stock macOS Bash 3.2: log parsing, burst detection, cooldown, log rotation, config round trip, launchd plist validation, and the server's sshd block.
- End to end on macOS 26 with WARP connected, against a locked-down local SSH server using the exact `Match` block from `setup-server.sh`. Verified that shell access and reverse forwarding are refused, that Discord runs through the tunnel, that auto-relaunch fires once, and that fallback works when the tunnel dies.
- `setup-server.sh` itself hasn't yet been run on a real Linux server. Please open an issue if it fails on your distribution.

## Uninstall

```sh
dwf uninstall --purge
rm ~/.local/bin/dwf
rm -r ~/.local/share/discord-warp-fix
```

---

## Türkçe

Discord Türkiye'de erişime kapatıldığından beri çoğu kişi Cloudflare WARP
kullanıyor. WARP kimseye ayrı bir IP vermiyor: İstanbul'daki herkes aynı
birkaç ortak çıkış adresini paylaşıyor. Discord her IP için istek sınırı
uyguladığından, binlerce kişinin trafiği bu sınırı aşıyor ve Discord
**429 Too Many Requests** yanıtı veriyor. Uygulamada bu **"Mesajlar
yüklenemedi"** hatası olarak görünüyor.

DNS değiştirmek, WARP protokolünü kalıcı olarak değiştirmek ya da WARP'ı
yeniden kaydettirmek sorunu çözmez.

`dwf` üç mod sunar:

- **`reroll`** (ücretsiz): Discord'un günlük dosyasını izler. 429 hataları artınca WARP'ı başka bir ortak çıkış IP'sine geçirir ve bildirim gönderir. Sorunu azaltır ama kalıcı çözmez.
- **`tunnel`** (kalıcı çözüm): Yurt dışındaki kendi sunucunuza bir SSH tüneli açar ve Discord'u bu tünelden başlatır. Discord artık kimseyle paylaşmadığınız bir IP'den çıkar. Ayda birkaç dolarlık en küçük VPS yeterlidir.
- **`proxy`**: Elinizde zaten bir SOCKS5 veya HTTP proxy varsa Discord'u onun üzerinden çalıştırır.

Kurulum:

```sh
./install.sh
dwf setup      # modu seçin
dwf install    # arka plan servislerini başlatın
dwf launch     # tunnel/proxy modunda Discord'u proxy üzerinden yeniden açar
```

WARP'ı her modda açık tutun. Sesli görüşmeler ve diğer trafik WARP
üzerinden gitmeye devam eder. Şu an yalnızca macOS desteklenmektedir.

## License

MIT
