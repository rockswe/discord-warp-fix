# discord-warp-fix

Keeps Discord working on macOS when you reach it through Cloudflare WARP and
keep getting **"Messages failed to load"**.

**Türkçe kurulum rehberi:** [aşağıda](#türkçe). İngilizce kısmı okumanıza gerek yok.

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

- Bir Mac (bu araç şimdilik yalnızca macOS'ta çalışır).
- **Cloudflare WARP** kurulu ve bağlı olmalı. Menü çubuğundaki bulut simgesinde "Connected" yazmalı. Kurulu değilse https://one.one.one.one adresinden indirin.
- **Discord masaüstü uygulaması.** Tarayıcıdaki Discord ile çalışmaz.

### 1. Adım: Aracı indirin

1. Bu sayfanın üstündeki yeşil **Code** düğmesine, ardından **Download ZIP**'e tıklayın.
2. İndirilen `discord-warp-fix-main.zip` dosyasına çift tıklayın. İndirilenler klasöründe `discord-warp-fix-main` adında bir klasör oluşur.

Git kullanmayı biliyorsanız bunun yerine şunu da çalıştırabilirsiniz:

```sh
git clone https://github.com/rockswe/discord-warp-fix.git ~/Downloads/discord-warp-fix-main
```

### 2. Adım: Kurun

1. **Terminal**'i açın: `Cmd + Boşluk` tuşlarına basın, `Terminal` yazın ve Enter'a basın.
2. Aşağıdaki iki satırı kopyalayıp Terminal'e yapıştırın ve Enter'a basın:

```sh
cd ~/Downloads/discord-warp-fix-main
bash install.sh
```

Ekranda `installed /Users/<adınız>/.local/bin/dwf` yazısını görmelisiniz.

Eğer altında `note: add ... to your PATH` diye bir satır da çıktıysa, şu iki
satırı da çalıştırın. Bunu sadece bir kez yapmanız yeterli:

```sh
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

Kurulumun çalıştığını kontrol edin:

```sh
dwf version
```

Ekranda `0.1.0` gibi bir sürüm numarası görmelisiniz. `command not found: dwf`
yazıyorsa yukarıdaki PATH adımını yapın ve Terminal'i kapatıp yeniden açın.

### 3. Adım (A): Ücretsiz `reroll` modu

Şunu çalıştırın:

```sh
dwf setup
```

Ekranda modlar listelenir ve `Mode [reroll]:` sorusu çıkar. Hiçbir şey
yazmadan **Enter**'a basın. Ardından arka plan servisini başlatın:

```sh
dwf install
```

`watchdog installed` yazısını gördüyseniz kurulum bitti. Bu servis arka planda
çalışır, Mac her açıldığında kendiliğinden başlar ve kapanırsa kendini yeniden
başlatır. Sizin başka bir şey yapmanıza gerek yok.

Size gelebilecek bildirimler (İngilizce gelir):

| Bildirim | Anlamı |
|---|---|
| `Discord was rate-limited on WARP exit ... Moved to ...` | Discord hata vermeye başladı, WARP diğer adrese geçirildi. Discord birkaç saniye içinde düzelir. |
| `... no other exit was available. Will retry in 5 min.` | İki adres de dolu. 5 dakika sonra tekrar denenecek. Bu sık oluyorsa `tunnel` moduna geçin. |

Adresi hemen kendiniz değiştirmek isterseniz:

```sh
dwf reroll
```

### 3. Adım (B): Kalıcı çözüm, `tunnel` modu

#### Sunucu kiralayın

1. Yurt dışında (örneğin Almanya veya Hollanda'da) en küçük ve en ucuz Linux sunucuyu kiralayın. Hetzner, DigitalOcean gibi firmaların aylık 4–5 dolarlık paketleri yeterli. Sunucu yalnızca Discord'un yazı trafiğini taşıyacak.
2. İşletim sistemi olarak **Ubuntu 24.04** seçin.
3. Giriş yöntemi sorulursa **şifre (password)** seçin ve bir root şifresi belirleyin.
4. Sunucu hazır olunca size verilen **IP adresini** (örneğin `203.0.113.7`) ve **root şifresini** bir yere not edin.

#### Aracı sunucuya bağlayın

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
| `root@...'s password:` | Root şifreniz. Yazarken ekranda hiçbir şey görünmez, bu normal. Yazıp Enter'a basın. Bu soru iki kez gelebilir. |

Bu adımda araç sunucunuzu kendisi hazırlar. Sunucuda sadece bağlantı
aktarabilen, sunucuya giriş yapamayan özel bir kullanıcı oluşturur.

Her şey yolundaysa en sonda şuna benzer bir satır görürsünüz:

```
tunnel works. Discord would leave from 203.0.113.7 (WARP's shared exit is 104.28.x.x).
```

Buradaki ilk IP adresi sunucunuzun adresi olmalı. Ardından şu iki komutu çalıştırın:

```sh
dwf install
dwf launch
```

`dwf launch` Discord'u kapatıp tünel üzerinden yeniden açar. Bundan sonra:

- Mac açıldığında tünel ve izleme servisi kendiliğinden başlar.
- Discord kendiliğinden açılırsa (açılışta ya da güncelleme sonrası) birkaç saniye içinde tünel üzerinden yeniden başlatılır.
- Sunucunuz çökerse Discord 2 dakika sonra normal WARP bağlantısıyla yeniden açılır, böylece Discord'suz kalmazsınız. Sunucu düzelince bildirim gelir, o zaman `dwf launch` yazmanız yeterli.
- Sesli görüşmeler tünelden değil WARP üzerinden gider. Bu bir sorun değil, çünkü sınırlama sesli görüşmeleri etkilemiyor.

**WARP'ı her zaman açık tutun.** Tünel modunda da Discord'un bazı bağlantıları
ve diğer tüm trafiğiniz WARP üzerinden geçer.

#### Elinizde zaten bir proxy varsa: `proxy` modu

`dwf setup` çalıştırın, `Mode` sorusuna `proxy` yazın. `Proxy URL` sorusuna
proxy adresinizi yazın, örneğin `socks5://203.0.113.7:1080`. Ardından
`dwf install` ve `dwf launch` çalıştırın. Şifreli (kullanıcı adı/parola
isteyen) SOCKS5 proxy'ler desteklenmez.

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
| `Watchdog` ve `Tunnel` | `running` yazmalı |

Aracın arka planda neler yaptığını görmek için:

```sh
dwf logs
```

### Sık karşılaşılan sorunlar

| Sorun | Çözüm |
|---|---|
| `command not found: dwf` | 2. Adım'daki PATH satırlarını çalıştırın ve Terminal'i kapatıp açın. |
| `warp-cli not found` | Cloudflare WARP kurulu değil. https://one.one.one.one adresinden kurun. |
| `can't reach Cloudflare through WARP` | WARP bağlı değil. Menü çubuğundaki bulut simgesinden bağlanın. |
| `tunnel test FAILED` | Sunucu IP'si ya da şifre yanlış olabilir, veya sunucu kapalı olabilir. Sunucunun açık olduğunu kontrol edip `dwf setup`'ı tekrar çalıştırın. |
| `warning: that's the same IP as WARP's shared exit` | Girdiğiniz sunucu yurt dışında değil ya da yanlış adres girdiniz. |
| Bildirim gelmiyor | Sistem Ayarları > Bildirimler bölümünde **Script Editor** için bildirimleri açın. |
| Discord'u tünelsiz açmak istiyorum | `dwf launch --direct` |

### Ayarlar (isteğe bağlı)

Ayarlar `~/.config/discord-warp-fix/config` dosyasındadır. Değiştirmeniz
gerekmez, ama isterseniz düzenleyip ardından `dwf install` çalıştırın.

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

Ardından aracı kaldırın:

```sh
dwf uninstall --purge
rm ~/.local/bin/dwf
rm -r ~/.local/share/discord-warp-fix
```

Kiraladığınız sunucuyu artık kullanmayacaksanız sunucu firmasının panelinden
silmeyi unutmayın, yoksa ücretlendirilmeye devam eder.

## License

MIT
