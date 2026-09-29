// Package config reads and writes dwf's KEY=value config file.
//
// The format is a small subset of shell assignments so that configs written
// by the original Bash version (printf %q output) keep working.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rockswe/discord-warp-fix/internal/paths"
)

// Modes.
const (
	ModeReroll = "reroll"
	ModeTunnel = "tunnel"
	ModeProxy  = "proxy"
)

// Config is everything dwf can be told. Durations are in seconds.
type Config struct {
	Mode           string
	SSHHost        string
	SSHUser        string
	SSHPort        int
	SSHKey         string
	SocksPort      int
	ProxyURL       string
	Threshold      int
	Window         int
	Cooldown       int
	Poll           int
	RerollAttempts int
	AutoRelaunch   bool
	FallbackAfter  int
	Notify         bool
	DiscordApp     string
	DiscordLog     string // empty = derived from DiscordApp
}

// Default returns the built-in settings.
func Default() Config {
	return Config{
		Mode:           ModeReroll,
		SSHUser:        "dwf",
		SSHPort:        22,
		SSHKey:         paths.DefaultSSHKey(),
		SocksPort:      1080,
		Threshold:      5,
		Window:         120,
		Cooldown:       300,
		Poll:           5,
		RerollAttempts: 6,
		AutoRelaunch:   true,
		FallbackAfter:  120,
		Notify:         true,
		DiscordApp:     "Discord",
	}
}

// ProxyURL returns the proxy Discord should use, or "" in reroll mode.
func (c Config) ProxyFor() string {
	switch c.Mode {
	case ModeTunnel:
		return fmt.Sprintf("socks5://127.0.0.1:%d", c.SocksPort)
	case ModeProxy:
		return c.ProxyURL
	}
	return ""
}

// Validate reports settings that can't work.
func (c Config) Validate() error {
	switch c.Mode {
	case ModeReroll:
	case ModeTunnel:
		if c.SSHHost == "" {
			return errors.New("tunnel mode needs SSH_HOST. Run: dwf setup")
		}
	case ModeProxy:
		if c.ProxyURL == "" {
			return errors.New("proxy mode needs PROXY_URL. Run: dwf setup")
		}
	default:
		return fmt.Errorf("MODE must be reroll, tunnel or proxy, not %q", c.Mode)
	}
	for name, v := range map[string]int{"SSH_PORT": c.SSHPort, "SOCKS_PORT": c.SocksPort,
		"THRESHOLD": c.Threshold, "WINDOW": c.Window, "POLL": c.Poll, "REROLL_ATTEMPTS": c.RerollAttempts} {
		if v <= 0 {
			return fmt.Errorf("%s must be a positive number", name)
		}
	}
	if c.Cooldown < 0 || c.FallbackAfter < 0 {
		return errors.New("COOLDOWN and FALLBACK_AFTER can't be negative")
	}
	return nil
}

// Load reads the config file on top of the defaults. A missing file is fine.
func Load(path string) (Config, error) {
	c := Default()
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, raw, ok := strings.Cut(line, "=")
		if !ok {
			return c, fmt.Errorf("%s:%d: expected KEY=value", path, n)
		}
		val, err := Unquote(raw)
		if err != nil {
			return c, fmt.Errorf("%s:%d: %v", path, n, err)
		}
		if err := c.set(strings.TrimSpace(key), val); err != nil {
			return c, fmt.Errorf("%s:%d: %v", path, n, err)
		}
	}
	return c, sc.Err()
}

func (c *Config) set(key, val string) error {
	atoi := func(dst *int) error {
		v, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("%s must be a number, got %q", key, val)
		}
		*dst = v
		return nil
	}
	boolean := func(dst *bool) error {
		switch strings.ToLower(val) {
		case "1", "true", "yes", "on":
			*dst = true
		case "0", "false", "no", "off", "":
			*dst = false
		default:
			return fmt.Errorf("%s must be 1 or 0, got %q", key, val)
		}
		return nil
	}
	switch key {
	case "MODE":
		c.Mode = val
	case "SSH_HOST":
		c.SSHHost = val
	case "SSH_USER":
		c.SSHUser = val
	case "SSH_PORT":
		return atoi(&c.SSHPort)
	case "SSH_KEY":
		c.SSHKey = val
	case "SOCKS_PORT":
		return atoi(&c.SocksPort)
	case "PROXY_URL":
		c.ProxyURL = val
	case "THRESHOLD":
		return atoi(&c.Threshold)
	case "WINDOW":
		return atoi(&c.Window)
	case "COOLDOWN":
		return atoi(&c.Cooldown)
	case "POLL":
		return atoi(&c.Poll)
	case "REROLL_ATTEMPTS":
		return atoi(&c.RerollAttempts)
	case "AUTO_RELAUNCH":
		return boolean(&c.AutoRelaunch)
	case "FALLBACK_AFTER":
		return atoi(&c.FallbackAfter)
	case "NOTIFY":
		return boolean(&c.Notify)
	case "DISCORD_APP":
		c.DiscordApp = val
	case "DISCORD_LOG":
		c.DiscordLog = val
	}
	return nil // unknown keys are ignored so older binaries accept newer configs
}

// Save writes the config atomically with owner-only permissions.
func (c Config) Save(path string) error {
	b01 := func(b bool) string {
		if b {
			return "1"
		}
		return "0"
	}
	var sb strings.Builder
	sb.WriteString("# discord-warp-fix config. Edit, then run: dwf install\n")
	kv := [][2]string{
		{"MODE", c.Mode}, {"SSH_HOST", c.SSHHost}, {"SSH_USER", c.SSHUser},
		{"SSH_PORT", strconv.Itoa(c.SSHPort)}, {"SSH_KEY", c.SSHKey},
		{"SOCKS_PORT", strconv.Itoa(c.SocksPort)}, {"PROXY_URL", c.ProxyURL},
		{"THRESHOLD", strconv.Itoa(c.Threshold)}, {"WINDOW", strconv.Itoa(c.Window)},
		{"COOLDOWN", strconv.Itoa(c.Cooldown)}, {"POLL", strconv.Itoa(c.Poll)},
		{"REROLL_ATTEMPTS", strconv.Itoa(c.RerollAttempts)},
		{"AUTO_RELAUNCH", b01(c.AutoRelaunch)}, {"FALLBACK_AFTER", strconv.Itoa(c.FallbackAfter)},
		{"NOTIFY", b01(c.Notify)}, {"DISCORD_APP", c.DiscordApp},
	}
	if c.DiscordLog != "" {
		kv = append(kv, [2]string{"DISCORD_LOG", c.DiscordLog})
	}
	for _, p := range kv {
		fmt.Fprintf(&sb, "%s=%s\n", p[0], Quote(p[1]))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(sb.String()), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Quote wraps a value in double quotes, escaping what the shell would expand.
func Quote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "$", `\$`, "`", "\\`")
	return `"` + r.Replace(s) + `"`
}

// Unquote parses one shell-style value: bare words with backslash escapes,
// '...', "..." and $'...' segments, as produced by Bash's printf %q.
func Unquote(v string) (string, error) {
	v = strings.TrimSpace(v)
	var b strings.Builder
	for i := 0; i < len(v); {
		switch c := v[i]; {
		case c == '\'':
			j := strings.IndexByte(v[i+1:], '\'')
			if j < 0 {
				return "", errors.New("unterminated '")
			}
			b.WriteString(v[i+1 : i+1+j])
			i += j + 2
		case c == '"':
			i++
			for ; i < len(v) && v[i] != '"'; i++ {
				if v[i] == '\\' && i+1 < len(v) && strings.IndexByte("\\\"$`", v[i+1]) >= 0 {
					i++
				}
				b.WriteByte(v[i])
			}
			if i >= len(v) {
				return "", errors.New(`unterminated "`)
			}
			i++
		case c == '$' && i+1 < len(v) && v[i+1] == '\'':
			i += 2
			for ; i < len(v) && v[i] != '\''; i++ {
				if v[i] == '\\' && i+1 < len(v) {
					i++
					switch v[i] {
					case 'n':
						b.WriteByte('\n')
					case 't':
						b.WriteByte('\t')
					default:
						b.WriteByte(v[i])
					}
					continue
				}
				b.WriteByte(v[i])
			}
			if i >= len(v) {
				return "", errors.New("unterminated $'")
			}
			i++
		case c == '\\' && i+1 < len(v):
			b.WriteByte(v[i+1])
			i += 2
		case c == ' ' || c == '\t':
			rest := strings.TrimSpace(v[i:])
			if rest != "" && !strings.HasPrefix(rest, "#") {
				return "", fmt.Errorf("unexpected text after value: %q", rest)
			}
			return b.String(), nil
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String(), nil
}
