# discord-warp-fix

Keeps Discord working when you reach it through Cloudflare WARP and keep
getting **"Messages failed to load"**. One small program for macOS, Linux and
Windows.

**Türkçe kurulum rehberi:** [aşağıda](#türkçe). İngilizce kısmı okumanıza gerek yok.

| Platform | Status |
|---|---|
| macOS | Tested end to end with WARP and the Discord app |
| Linux | Server side tested in CI on Ubuntu. The desktop side (starting Discord, notifications) hasn't been tried on a real desktop yet |
| Windows | Builds and passes unit tests in CI, but **hasn't been run on a real Windows PC yet**. If something breaks, please [open an issue](../../issues) or send a PR. See [CONTRIBUTING.md](CONTRIBUTING.md) |

## Why this happens

When Discord is blocked where you live, WARP (the free 1.1.1.1 app) gets you
past the block. But WARP doesn't give you your own IP address. Everyone
connected to the same Cloudflare data center leaves from a tiny pool of
shared exit addresses. In Istanbul that pool was just two IPs in our testing.

Discord limits how many requests one IP may send. Thousands of people behind
one address blow through that limit, so Discord answers with **HTTP 429 Too
Many Requests**, and the app shows "Messages failed to load". It "sometimes
works randomly" because the limit is a rolling window shared with strangers.

Things that **don't** help: changing DNS, switching WARP between MASQUE and
WireGuard for good, re-registering WARP, or IPv6. Discord has no IPv6
addresses, and WARP shows the same shared IPv4 exit anyway.

## What dwf does

| Mode | Cost | What it does | Fixes it for good? |
|---|---|---|---|
| `reroll` | free | Watches Discord's log. On a burst of 429s it flips WARP until you land on the other shared exit, and notifies you. | No. It only helps while one exit is quieter. |
| `tunnel` | a small server abroad | Keeps an SSH tunnel to a server you control and runs Discord through it. Discord then uses that server's IP, which nobody else shares. | **Yes** |
| `proxy` | a proxy you already have | Same as `tunnel`, through an existing SOCKS5 or HTTP proxy. | Yes, if its IP isn't shared |

In `tunnel` and `proxy` modes dwf also:

- moves Discord onto the proxy when it starts on its own, at login or after an update;
- falls back to plain WARP if the proxy is down for two minutes, and tells you when it's back;
- reconnects the tunnel by itself when the connection drops.

The SSH client is built in, so nothing else needs to be installed. Keep WARP
connected in every mode: it still carries your other traffic, Discord voice,
and the connection to your server.

## Install

**macOS and Linux**, in a terminal:

```sh
curl -fsSL https://raw.githubusercontent.com/rockswe/discord-warp-fix/main/install.sh | sh
```

**Windows**, in PowerShell:

```powershell
irm https://raw.githubusercontent.com/rockswe/discord-warp-fix/main/install.ps1 | iex
```

Or download a binary from [Releases](../../releases), or build from source
with Go 1.26+: `go build ./cmd/dwf`.

Then:

```sh
dwf setup      # pick a mode
dwf install    # run in the background, now and at every login
dwf launch     # tunnel/proxy modes only: restart Discord through the proxy
```

### Tunnel mode: the server

Any Linux server outside your country works, like a $4–5/month VPS from
Hetzner or DigitalOcean, or a free-tier VM. It only relays Discord's text
traffic, so the smallest plan is plenty. Pick Ubuntu or Debian.

`dwf setup` creates an SSH key and prepares the server for you when you give
it an admin login such as `root`. It creates a user that can **only** open
outbound connections. That user has no shell and no terminal, and it can't
listen on ports, so a leaked key can't be used to log in. To do it by hand,
run [`server/setup-server.sh`](server/setup-server.sh) on the server as root
with the public key `dwf setup` prints.

## Commands

```
dwf status            WARP exit IP, whether Discord uses the proxy, recent errors, service
dwf check             test the configured mode end to end
dwf launch            restart Discord through the proxy
dwf launch --direct   restart Discord without the proxy
dwf reroll            move WARP to a different exit IP right now
dwf logs              what dwf has been doing
dwf uninstall         stop dwf and remove it from login (--purge also deletes the config)
```

Settings live in `~/.config/discord-warp-fix/config`, or
`%APPDATA%\discord-warp-fix\config` on Windows. Run `dwf install` again
after editing it.

| Setting | Default | Meaning |
|---|---|---|
| `THRESHOLD` / `WINDOW` | `5` / `120` | errors within that many seconds that count as a rate-limit burst |
| `COOLDOWN` | `300` | seconds between automatic actions |
| `AUTO_RELAUNCH` | `1` | move a freshly started Discord onto the proxy |
| `FALLBACK_AFTER` | `120` | seconds of proxy failure before falling back to WARP (`0` = only notify) |
| `DISCORD_APP` | `Discord` | or `Discord PTB` / `Discord Canary` |
| `NOTIFY` | `1` | desktop notifications |

## Limitations

- **Voice isn't proxied.** Discord's voice engine ignores proxy settings, so calls go through WARP. That's fine: voice servers aren't what gets rate-limited.
- **Restarts take a few seconds.** `dwf launch`, auto-relaunch and fallback all quit and reopen Discord. Automatic restarts only touch a Discord that started in the last three minutes, or one whose proxy is dead.
- **`reroll` can't create new exits.** When every shared exit is overloaded, it can only tell you.
- **SOCKS5 proxies with a username and password** aren't supported, because Discord can't use them.

## How it was tested

- Unit tests cover config parsing (including configs from the old Bash version), burst detection, log rotation, the watchdog's rules, the SOCKS5 server, and the SSH tunnel against an in-process SSH server: reconnects, pinned host keys, and a refusal to ever open a shell.
- CI runs the tests on macOS, Linux and Windows. On Ubuntu it also prepares the runner as a real tunnel server with `dwf setup --admin`, and checks that the tunnel user can't get a shell or open listening ports.
- On macOS, against real WARP and Discord: setup, launching Discord through the tunnel, auto-relaunch, falling back when the server dies, and recovering when it returns.

## Uninstall

```sh
dwf launch --direct     # tunnel/proxy modes: put Discord back on plain WARP first
dwf uninstall --purge
rm ~/.local/bin/dwf     # Windows: Remove-Item -Recurse "$env:LOCALAPPDATA\Programs\discord-warp-fix"
```

---

## Türkçe

Bu bölüm tek başına yeterli: İngilizce kısmı okumanıza gerek yok.

### Sorun ne?

Discord Türkiye'de erişime kapatıldığından beri çoğu kişi Cloudflare WARP
(1.1.1.1 uygulaması) kullanıyor. WARP herkese ayrı bir IP adresi vermiyor.
İstanbul'dan bağlanan herkes aynı iki ortak çıkış adresini paylaşıyor.

Discord her IP adresinden gelen istek sayısını sınırlıyor. Binlerce kişi aynı
adresten bağlandığı için bu sınır sürekli aşılıyor ve Discord istekleri
reddediyor. Uygulamada bu **"Mesajlar yüklenemedi"** hatası olarak görünüyor.
Arada bir çalışmasının sebebi de bu: sınır, o anda o adreste kaç kişi olduğuna
göre dolup boşalıyor.

DNS değiştirmek, WARP'ı kapatıp açmak, WARP protokolünü değiştirmek ya da
WARP'ı silip yeniden kurmak sorunu kalıcı olarak çözmez.

### Hangi bilgisayarlarda çalışır?

| Sistem | Durum |
|---|---|
| macOS | Gerçek WARP ve Discord ile baştan sona denendi. |
| Linux | Sunucu tarafı otomatik testlerle denendi. Masaüstünde Discord'u başlatma kısmı henüz gerçek bir bilgisayarda denenmedi. |
| Windows | Program derleniyor ve otomatik testlerden geçiyor, ama **henüz gerçek bir Windows bilgisayarda denenmedi**. Sorun yaşarsanız GitHub'da bir [issue](../../issues) açın ya da düzeltip PR gönderin. |

### Hangi modu seçmeliyim?

| Mod | Ücret | Ne yapar? | Kalıcı çözüm mü? |
|---|---|---|---|
| `reroll` | Ücretsiz | Discord hata vermeye başlayınca WARP'ı diğer ortak adrese geçirir ve size bildirim gönderir. | Hayır. Diğer adres de doluysa yapabileceği bir şey yok. |
| `tunnel` | Aylık yaklaşık 4–5 $ | Discord'u yurt dışında kiraladığınız küçük bir sunucu üzerinden çalıştırır. Discord'a sadece size ait bir IP'den bağlanırsınız. | **Evet** |
| `proxy` | Proxy'nize bağlı | Elinizde zaten bir SOCKS5 veya HTTP proxy varsa Discord'u onun üzerinden çalıştırır. | Evet, proxy'nin IP'si başkalarıyla paylaşılmıyorsa |

**Önerimiz:** Önce ücretsiz `reroll` moduyla başlayın. Hatalar devam ederse
`tunnel` moduna geçin. Mod değiştirmek için her şeyi baştan kurmanız gerekmez,
sadece 3. adımı tekrarlarsınız.

### Gerekenler

- **Cloudflare WARP** kurulu ve bağlı olmalı. Kurulu değilse https://one.one.one.one adresinden indirin. Mac'te menü çubuğundaki, Windows'ta sağ alttaki bulut simgesinde "Connected" yazmalı.
- **Discord masaüstü uygulaması.** Tarayıcıdaki Discord ile çalışmaz.

### 1. Adım: Kurun

#### Mac veya Linux

1. **Terminal**'i açın. Mac'te `Cmd + Boşluk` tuşlarına basın, `Terminal` yazın ve Enter'a basın.
2. Aşağıdaki satırı kopyalayıp Terminal'e yapıştırın ve Enter'a basın:

```sh
curl -fsSL https://raw.githubusercontent.com/rockswe/discord-warp-fix/main/install.sh | sh
```

Ekranda `installed ... dwf` yazısını görmelisiniz. Altında
`isn't on your PATH yet` diye bir uyarı çıktıysa, uyarının hemen altında
gösterilen `echo ...` ile başlayan satırı kopyalayıp çalıştırın. Bunu sadece
bir kez yapmanız yeterli.

#### Windows

1. Başlat menüsüne `PowerShell` yazın ve **Windows PowerShell**'i açın. Yönetici olarak açmanıza gerek yok.
2. Aşağıdaki satırı kopyalayıp yapıştırın ve Enter'a basın:

```powershell
irm https://raw.githubusercontent.com/rockswe/discord-warp-fix/main/install.ps1 | iex
```

3. Kurulum bitince **PowerShell'i kapatıp yeniden açın.** Yoksa `dwf` komutu bulunamaz.

#### Kurulumu kontrol edin

Her sistemde şunu çalıştırın:

```sh
dwf version
```

Bir sürüm numarası görmelisiniz. `command not found` ya da
`is not recognized` yazıyorsa Terminal'i veya PowerShell'i kapatıp açın. Mac
ve Linux'ta PATH uyarısındaki satırı çalıştırdığınızdan emin olun.

### 2. Adım (A): Ücretsiz `reroll` modu

Şunu çalıştırın:

```sh
dwf setup
```

Ekranda modlar listelenir ve `Mode [reroll]:` sorusu çıkar. Hiçbir şey
yazmadan **Enter**'a basın. Ardından arka plan servisini başlatın:

```sh
dwf install
```

`installed and started` yazısını gördüyseniz kurulum bitti. Program arka
planda çalışır ve bilgisayar her açıldığında kendiliğinden başlar. Sizin
başka bir şey yapmanıza gerek yok.

Size gelebilecek bildirimler (İngilizce gelir):

| Bildirim | Anlamı |
|---|---|
| `Discord was rate-limited on WARP exit ... Moved to ...` | Discord hata vermeye başladı, WARP diğer adrese geçirildi. Discord birkaç saniye içinde düzelir. |
| `... no other exit was available. Will retry in 5 min.` | İki adres de dolu. 5 dakika sonra tekrar denenecek. Bu sık oluyorsa `tunnel` moduna geçin. |

Adresi hemen kendiniz değiştirmek isterseniz:

```sh
dwf reroll
```

### 2. Adım (B): Kalıcı çözüm, `tunnel` modu

#### Sunucu kiralayın

1. Yurt dışında (örneğin Almanya veya Hollanda'da) en küçük ve en ucuz Linux sunucuyu kiralayın. Hetzner, DigitalOcean gibi firmaların aylık 4–5 dolarlık paketleri yeterli. Sunucu yalnızca Discord'un yazı trafiğini taşıyacak.
2. İşletim sistemi olarak **Ubuntu 24.04** seçin.
3. Giriş yöntemi sorulursa **şifre (password)** seçin ve bir root şifresi belirleyin.
4. Sunucu hazır olunca size verilen **IP adresini** (örneğin `203.0.113.7`) ve **root şifresini** bir yere not edin.

#### Programı sunucuya bağlayın

Şunu çalıştırın:

```sh
dwf setup
```

Sorular İngilizce gelir. Her birine ne yazacağınız:

| Ekrandaki soru | Ne yazmalısınız? |
|---|---|
| `Mode [reroll]:` | `tunnel` yazıp Enter |
| `Server address (IP or hostname) []:` | Sunucunun IP adresi, örneğin `203.0.113.7` |
| `Server SSH port [22]:` | Sadece Enter |
| `Tunnel user on the server [dwf]:` | Sadece Enter |
| `Admin login on the server to prepare it now (e.g. root) ...:` | `root` |
| `Are you sure you want to continue connecting (yes/no/[fingerprint])?` | `yes` |
| `root@...'s password:` | Root şifreniz. Yazarken ekranda hiçbir şey görünmez, bu normal. Yazıp Enter'a basın. |

Bu adımda program sunucunuzu kendisi hazırlar. Sunucuda sadece bağlantı
aktarabilen, sunucuya giriş yapamayan özel bir kullanıcı oluşturur.

Her şey yolundaysa en sonda şuna benzer bir satır görürsünüz:

```
tunnel works. Discord would leave from 203.0.113.7 (WARP's shared exit is 104.28.x.x).
```

Buradaki ilk IP adresi sunucunuzun adresi olmalı. Ardından şu iki komutu
çalıştırın:

```sh
dwf install
dwf launch
```

`dwf launch` Discord'u kapatıp tünel üzerinden yeniden açar. Bundan sonra:

- Bilgisayar açıldığında tünel ve izleme servisi kendiliğinden başlar.
- Discord kendiliğinden açılırsa (açılışta ya da güncelleme sonrası) birkaç saniye içinde tünel üzerinden yeniden başlatılır.
- Sunucunuz çökerse Discord 2 dakika sonra normal WARP bağlantısıyla yeniden açılır, böylece Discord'suz kalmazsınız. Sunucu düzelince bildirim gelir, o zaman `dwf launch` yazmanız yeterli.
- Sesli görüşmeler tünelden değil WARP üzerinden gider. Bu bir sorun değil, çünkü sınırlama sesli görüşmeleri etkilemiyor.

**WARP'ı her zaman açık tutun.** Tünel modunda da Discord'un bazı bağlantıları
ve diğer tüm trafiğiniz WARP üzerinden geçer.

#### Elinizde zaten bir proxy varsa: `proxy` modu

`dwf setup` çalıştırın, `Mode` sorusuna `proxy` yazın. `Proxy URL` sorusuna
proxy adresinizi yazın, örneğin `socks5://203.0.113.7:1080`. Ardından
`dwf install` ve `dwf launch` çalıştırın. Kullanıcı adı ve parola isteyen
SOCKS5 proxy'ler desteklenmez.

### Çalışıyor mu? Kontrol edin

```sh
dwf status
```

Satırların anlamı:

| Satır | Olması gereken |
|---|---|
| `WARP` | `Connected` yazmalı |
| `Discord` | `tunnel` modunda `running through socks5://127.0.0.1:1080`, `reroll` modunda `running directly` |
| `Errors` | Son 10 dakikadaki hata sayısı. `0` ya da düşük olmalı. |
| `Proxy` | Sadece `tunnel`/`proxy` modunda görünür. `reaches Discord, exit IP <sunucunuzun IP'si>` yazmalı. |
| `Service` | `running` yazmalı |

Programın arka planda neler yaptığını görmek için:

```sh
dwf logs
```

### Sık karşılaşılan sorunlar

| Sorun | Çözüm |
|---|---|
| `command not found: dwf` ya da `is not recognized` | Terminal'i veya PowerShell'i kapatıp açın. Mac ve Linux'ta PATH uyarısındaki satırı çalıştırın. |
| `warp-cli not found` | Cloudflare WARP kurulu değil. https://one.one.one.one adresinden kurun. |
| `can't reach Cloudflare through WARP` | WARP bağlı değil. Bulut simgesinden bağlanın. |
| `tunnel test FAILED` | Sunucu IP'si ya da şifre yanlış olabilir, veya sunucu kapalı olabilir. Sunucunun açık olduğunu kontrol edip `dwf setup`'ı tekrar çalıştırın. |
| `warning: that's the same IP as WARP's shared exit` | Girdiğiniz sunucu yurt dışında değil ya da yanlış adres girdiniz. |
| `the host key for ... CHANGED` | Sunucuyu silip yeniden kurduysanız normaldir. Hata mesajında yazan `known_hosts` dosyasından o sunucunun satırını silin. |
| Mac'te bildirim gelmiyor | Sistem Ayarları > Bildirimler bölümünde **Script Editor** için bildirimleri açın. |
| Windows Defender programı engelledi | İmzasız programlarda bazen yanlış alarm veriyor. Programı [Releases](../../releases) sayfasındaki `checksums.txt` ile karşılaştırabilirsiniz. |
| Discord'u tünelsiz açmak istiyorum | `dwf launch --direct` |

### Ayarlar (isteğe bağlı)

Ayarlar Mac ve Linux'ta `~/.config/discord-warp-fix/config`, Windows'ta
`%APPDATA%\discord-warp-fix\config` dosyasındadır. Değiştirmeniz gerekmez, ama
isterseniz düzenleyip ardından `dwf install` çalıştırın.

| Ayar | Varsayılan | Anlamı |
|---|---|---|
| `THRESHOLD` / `WINDOW` | `5` / `120` | 120 saniye içinde 5 hata olursa müdahale edilir. |
| `COOLDOWN` | `300` | İki otomatik müdahale arasında en az kaç saniye bekleneceği. |
| `AUTO_RELAUNCH` | `1` | Kendiliğinden açılan Discord'u tünel üzerinden yeniden başlatır. Kapatmak için `0`. |
| `FALLBACK_AFTER` | `120` | Tünel kaç saniye çalışmazsa Discord'un normal WARP ile açılacağı. `0` yaparsanız sadece bildirim gelir. |
| `DISCORD_APP` | `Discord` | PTB veya Canary kullanıyorsanız `"Discord PTB"` ya da `"Discord Canary"`. |
| `NOTIFY` | `1` | Bildirimleri kapatmak için `0`. |

### Kaldırma

`tunnel` ya da `proxy` modunu kullandıysanız önce Discord'u tünelsiz açın.
Yoksa kaldırmadan sonra Discord kapanmış tünele bağlı kalır:

```sh
dwf launch --direct
```

Ardından programı kaldırın:

```sh
dwf uninstall --purge
```

Son olarak programın kendisini silin. Mac ve Linux'ta:

```sh
rm ~/.local/bin/dwf
```

Windows'ta PowerShell'de:

```powershell
Remove-Item -Recurse "$env:LOCALAPPDATA\Programs\discord-warp-fix"
```

Kiraladığınız sunucuyu artık kullanmayacaksanız sunucu firmasının panelinden
silmeyi unutmayın, yoksa ücretlendirilmeye devam eder.

## License

MIT
