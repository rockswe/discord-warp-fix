package discord

import (
	"strings"
	"testing"
	"time"
)

func TestProxyArg(t *testing.T) {
	p := &Process{Cmdline: "/Applications/Discord.app/Contents/MacOS/Discord --proxy-server=socks5://127.0.0.1:1080"}
	if !p.Proxied() || p.ProxyArg() != "socks5://127.0.0.1:1080" {
		t.Fatalf("got %q", p.ProxyArg())
	}
	p = &Process{Cmdline: `Discord.exe "--proxy-server=socks5://127.0.0.1:1080"`}
	if p.ProxyArg() != "socks5://127.0.0.1:1080" {
		t.Fatalf("quoted Windows arg: got %q", p.ProxyArg())
	}
	if (&Process{Cmdline: "/usr/share/discord/Discord"}).Proxied() {
		t.Fatal("no flag means not proxied")
	}
}

func TestAge(t *testing.T) {
	now := time.Now()
	p := &Process{Started: now.Add(-90 * time.Second)}
	if p.Age(now) != 90*time.Second {
		t.Fatal(p.Age(now))
	}
}

func TestFlavors(t *testing.T) {
	if f := flavorOf("Discord PTB"); f.binary != "DiscordPTB" || f.dataDir != "discordptb" {
		t.Fatalf("%+v", f)
	}
	if f := flavorOf("Discord"); f.command != "discord" || f.appDir != "Discord" {
		t.Fatalf("%+v", f)
	}
}

func TestLogPath(t *testing.T) {
	p := LogPath("Discord Canary")
	if !strings.Contains(p, "discordcanary") || !strings.HasSuffix(p, "renderer_js.log") {
		t.Fatal(p)
	}
}

func TestFindDoesNotFail(t *testing.T) {
	if _, err := Find("Discord"); err != nil {
		t.Fatal(err)
	}
}
