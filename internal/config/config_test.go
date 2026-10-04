package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
)

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"ALL_USAGE_CONFIG", "ALL_USAGE_REFRESH", "ALL_USAGE_TIMEOUT", "ALL_USAGE_THEME",
		"ALL_USAGE_PROVIDERS", "ALL_USAGE_WINDOWS_HOME", "NO_COLOR"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
}

// The commented template must describe exactly the built-in defaults.
func TestTemplateMatchesDefaults(t *testing.T) {
	got := Default()
	md, err := toml.Decode(Template, got)
	if err != nil {
		t.Fatalf("template does not parse: %v", err)
	}
	if u := md.Undecoded(); len(u) > 0 {
		t.Fatalf("template has keys unknown to Config: %v", u)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("template does not validate: %v", err)
	}
	want := Default()
	if err := want.Validate(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("template differs from defaults:\n got %+v\nwant %+v", got, want)
	}
}

func TestLoadMissingDefaultFileUsesDefaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Path != "" || cfg.RefreshInterval.D() != time.Minute || len(cfg.EnabledProviders()) != 3 {
		t.Fatalf("unexpected config: path=%q refresh=%v providers=%v", cfg.Path, cfg.RefreshInterval.D(), cfg.EnabledProviders())
	}
}

func TestLoadExplicitMissingFileFails(t *testing.T) {
	clearEnv(t)
	if _, err := Load(filepath.Join(t.TempDir(), "nope.toml")); err == nil {
		t.Fatal("expected an error for a missing explicit config file")
	}
}

func writeFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadFileAndEnvPrecedence(t *testing.T) {
	clearEnv(t)
	p := writeFile(t, `
refresh_interval = "2m"
order = ["cursor", "codex"]
bogus = 1

[ui]
theme = "nord"
percent = "remaining"

[kiro]
enabled = false
`)
	t.Setenv("ALL_USAGE_REFRESH", "45s")
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.RefreshInterval.D(); got != 45*time.Second {
		t.Errorf("env should override file: refresh = %v", got)
	}
	if cfg.UI.Theme != "nord" || cfg.UI.Percent != "remaining" {
		t.Errorf("file values not applied: theme=%q percent=%q", cfg.UI.Theme, cfg.UI.Percent)
	}
	if got := strings.Join(cfg.EnabledProviders(), ","); got != "cursor,codex" {
		t.Errorf("enabled providers = %q, want cursor,codex", got)
	}
	if len(cfg.Warnings) != 1 || !strings.Contains(cfg.Warnings[0], "bogus") {
		t.Errorf("expected a warning for the unknown key, got %v", cfg.Warnings)
	}
}

func TestEnvProvidersAndTheme(t *testing.T) {
	clearEnv(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("ALL_USAGE_PROVIDERS", "kiro, codex")
	t.Setenv("ALL_USAGE_THEME", "Dracula")
	t.Setenv("NO_COLOR", "1")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(cfg.EnabledProviders(), ","); got != "kiro,codex" {
		t.Errorf("providers = %q", got)
	}
	if cfg.Cursor.Enabled {
		t.Error("cursor should be disabled")
	}
	if cfg.UI.Theme != "dracula" || cfg.UI.Color != "never" {
		t.Errorf("theme=%q color=%q", cfg.UI.Theme, cfg.UI.Color)
	}
}

func TestValidateErrors(t *testing.T) {
	clearEnv(t)
	p := writeFile(t, `
timeout = "10ms"
order = ["codex", "copilot"]
[ui]
layout = "diagonal"
warn_at = 95.0
critical_at = 90.0
[ui.colors]
accent = "blue"
[cursor]
api_base = "ftp://example.com"
`)
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected validation errors")
	}
	for _, want := range []string{"timeout", "copilot", "ui.layout", "warn_at", "ui.colors.accent", "cursor.api_base"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q:\n%v", want, err)
		}
	}
}

func TestUnknownThemeFallsBack(t *testing.T) {
	cfg := Default()
	cfg.UI.Theme = "neon"
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.UI.Theme != "auto" || len(cfg.Warnings) == 0 {
		t.Fatalf("theme=%q warnings=%v", cfg.UI.Theme, cfg.Warnings)
	}
}

func TestRefreshClamp(t *testing.T) {
	cfg := Default()
	cfg.RefreshInterval = Duration(time.Second)
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.RefreshInterval.D() != 5*time.Second {
		t.Fatalf("refresh = %v, want 5s", cfg.RefreshInterval.D())
	}
	cfg.RefreshInterval = 0
	if err := cfg.Validate(); err != nil || cfg.RefreshInterval != 0 {
		t.Fatalf("0 must disable refresh: %v %v", cfg.RefreshInterval, err)
	}
}

func TestSelectProvidersRejectsUnknown(t *testing.T) {
	cfg := Default()
	if err := cfg.SelectProviders([]string{"codex", "nope"}); err == nil {
		t.Fatal("expected error")
	}
	if err := cfg.SelectProviders([]string{"CURSOR", "cursor"}); err != nil {
		t.Fatal(err)
	}
	if got := cfg.EnabledProviders(); !reflect.DeepEqual(got, []string{"cursor"}) {
		t.Fatalf("got %v", got)
	}
}

func TestDurations(t *testing.T) {
	for in, want := range map[string]time.Duration{"": 0, "0": 0, "90": 90 * time.Second, "1.5": 1500 * time.Millisecond,
		"30s": 30 * time.Second, "5m": 5 * time.Minute, "1h30m": 90 * time.Minute} {
		got, err := ParseDuration(in)
		if err != nil || got != want {
			t.Errorf("ParseDuration(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseDuration("soon"); err == nil {
		t.Error("expected error for an invalid duration")
	}
	for d, want := range map[time.Duration]string{0: "0s", time.Minute: "1m", 90 * time.Second: "1m30s", 2 * time.Hour: "2h", 1500 * time.Millisecond: "1.5s"} {
		if got := FormatDuration(d); got != want {
			t.Errorf("FormatDuration(%v) = %q, want %q", d, got, want)
		}
	}
	var cfg struct {
		A Duration `toml:"a"`
		B Duration `toml:"b"`
	}
	if _, err := toml.Decode("a = 30\nb = \"2m\"", &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.A.D() != 30*time.Second || cfg.B.D() != 2*time.Minute {
		t.Fatalf("decoded %v %v", cfg.A.D(), cfg.B.D())
	}
}

func TestEncodeRoundTrip(t *testing.T) {
	cfg := Default()
	cfg.UI.Theme = "gruvbox"
	cfg.RefreshInterval = Duration(90 * time.Second)
	s, err := cfg.Encode()
	if err != nil {
		t.Fatal(err)
	}
	back := Default()
	if _, err := toml.Decode(s, back); err != nil {
		t.Fatalf("encoded config does not parse: %v\n%s", err, s)
	}
	if back.UI.Theme != "gruvbox" || back.RefreshInterval.D() != 90*time.Second {
		t.Fatalf("round trip lost values: %+v", back.UI)
	}
}

func TestWriteTemplate(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "config.toml")
	if err := WriteTemplate(p, false); err != nil {
		t.Fatal(err)
	}
	if err := WriteTemplate(p, false); err == nil {
		t.Fatal("must refuse to overwrite without force")
	}
	if err := WriteTemplate(p, true); err != nil {
		t.Fatal(err)
	}
}
