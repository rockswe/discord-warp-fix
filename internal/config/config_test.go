package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnquote(t *testing.T) {
	cases := map[string]string{
		`tunnel`:               "tunnel",
		`''`:                   "",
		`""`:                   "",
		`Discord\ PTB`:         "Discord PTB",
		`'Discord PTB'`:        "Discord PTB",
		`"a \"b\" \$c"`:        `a "b" $c`,
		`$'a\nb'`:              "a\nb",
		`/Users/x/.ssh/key`:    "/Users/x/.ssh/key",
		`5 # trailing comment`: "5",
	}
	for in, want := range cases {
		got, err := Unquote(in)
		if err != nil || got != want {
			t.Errorf("Unquote(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{`'open`, `"open`, `a b`} {
		if _, err := Unquote(bad); err == nil {
			t.Errorf("Unquote(%q) should fail", bad)
		}
	}
}

// A config exactly as the Bash version (printf %q) wrote it.
const bashConfig = `# discord-warp-fix config. Edit, then run: dwf install
MODE=tunnel
SSH_HOST=203.0.113.7
SSH_USER=dwf
SSH_PORT=2222
SSH_KEY=/Users/someone/.ssh/discord-warp-fix_ed25519
SOCKS_PORT=1080
PROXY_URL=''
THRESHOLD=7
WINDOW=120
COOLDOWN=300
POLL=5
REROLL_ATTEMPTS=6
AUTO_RELAUNCH=0
FALLBACK_AFTER=90
NOTIFY=1
DISCORD_APP=Discord\ PTB
`

func TestLoadBashConfig(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(p, []byte(bashConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Mode != "tunnel" || c.SSHHost != "203.0.113.7" || c.SSHPort != 2222 || c.Threshold != 7 ||
		c.AutoRelaunch || c.FallbackAfter != 90 || c.DiscordApp != "Discord PTB" || c.ProxyURL != "" {
		t.Fatalf("unexpected config: %+v", c)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "config")
	c := Default()
	c.Mode, c.ProxyURL, c.DiscordApp, c.Notify = ModeProxy, "socks5://h:1080", `Weird "App" $x`, false
	if err := c.Save(p); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if got != c {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", got, c)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/' {
		t.Fatalf("config should be private, got %v", fi.Mode().Perm())
	}
}

func TestMissingFileGivesDefaults(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "nope"))
	if err != nil || c != Default() {
		t.Fatalf("got %+v, %v", c, err)
	}
}

func TestValidate(t *testing.T) {
	c := Default()
	c.Mode = ModeTunnel
	if c.Validate() == nil {
		t.Error("tunnel without host should fail")
	}
	c.Mode = "bogus"
	if c.Validate() == nil {
		t.Error("unknown mode should fail")
	}
	c = Default()
	c.Threshold = 0
	if c.Validate() == nil {
		t.Error("zero threshold should fail")
	}
}

func TestProxyFor(t *testing.T) {
	c := Default()
	if c.ProxyFor() != "" {
		t.Error("reroll has no proxy")
	}
	c.Mode, c.SocksPort = ModeTunnel, 1999
	if c.ProxyFor() != "socks5://127.0.0.1:1999" {
		t.Error(c.ProxyFor())
	}
}
