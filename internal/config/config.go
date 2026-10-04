// Package config loads all-usage settings: built-in defaults, then the TOML
// config file, then environment variables (CLI flags are applied by the
// caller on top).
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/cfardev/all-usage/internal/sys"
)

// Provider identifiers.
const (
	Codex  = "codex"
	Kiro   = "kiro"
	Cursor = "cursor"
)

// AllProviders lists every supported provider in default display order.
var AllProviders = []string{Codex, Kiro, Cursor}

// Allowed values of enumerated settings.
var (
	Themes        = []string{"auto", "dark", "light", "dracula", "nord", "catppuccin", "gruvbox", "tokyonight", "material-ocean", "mono"}
	Layouts       = []string{"auto", "columns", "rows"}
	BarStyles     = []string{"blocks", "line", "ascii", "dots"}
	PercentModes  = []string{"used", "remaining"}
	ResetFormats  = []string{"relative", "absolute", "both"}
	Clocks        = []string{"24h", "12h"}
	ColorModes    = []string{"auto", "always", "never"}
	CodexSources  = []string{"auto", "api", "app-server", "sessions"}
	KiroSources   = []string{"auto", "cli", "ide"}
	CursorSources = []string{"auto", "ide", "cli"}
)

// Config is the complete configuration.
type Config struct {
	RefreshInterval Duration `toml:"refresh_interval"`
	Timeout         Duration `toml:"timeout"`
	Order           []string `toml:"order"`
	WindowsHome     string   `toml:"windows_home"`
	ScanWindows     bool     `toml:"scan_windows"`

	UI     UI           `toml:"ui"`
	Codex  CodexConfig  `toml:"codex"`
	Kiro   KiroConfig   `toml:"kiro"`
	Cursor CursorConfig `toml:"cursor"`

	// Path is the file the configuration was loaded from ("" when none).
	Path string `toml:"-"`
	// Warnings collects non-fatal problems such as unknown keys.
	Warnings []string `toml:"-"`
}

// UI holds presentation settings.
type UI struct {
	Theme       string  `toml:"theme"`
	Layout      string  `toml:"layout"`
	Columns     int     `toml:"columns"`
	CardWidth   int     `toml:"card_width"`
	Compact     bool    `toml:"compact"`
	BarStyle    string  `toml:"bar_style"`
	Percent     string  `toml:"percent"`
	ResetFormat string  `toml:"reset_format"`
	Clock       string  `toml:"clock"`
	WarnAt      float64 `toml:"warn_at"`
	CriticalAt  float64 `toml:"critical_at"`
	ShowSource  bool    `toml:"show_source"`
	ShowAccount bool    `toml:"show_account"`
	ShowHelp    bool    `toml:"show_help"`
	Color       string  `toml:"color"`
	Colors      Colors  `toml:"colors"`
}

// Colors overrides individual theme colors ("#RRGGBB", "#RGB" or an ANSI 0-255 index).
type Colors struct {
	Accent   string `toml:"accent"`
	Text     string `toml:"text"`
	Muted    string `toml:"muted"`
	Border   string `toml:"border"`
	OK       string `toml:"ok"`
	Warn     string `toml:"warn"`
	Critical string `toml:"critical"`
	BarEmpty string `toml:"bar_empty"`
}

// CodexConfig configures the OpenAI Codex provider.
type CodexConfig struct {
	Enabled          bool     `toml:"enabled"`
	DisplayName      string   `toml:"display_name"`
	Color            string   `toml:"color"`
	Meters           []string `toml:"meters"`
	Source           string   `toml:"source"`
	Home             string   `toml:"home"`
	Binary           string   `toml:"binary"`
	UseAppServer     bool     `toml:"use_app_server"`
	SessionsFallback bool     `toml:"sessions_fallback"`
	BaseURL          string   `toml:"base_url"`
}

// KiroConfig configures the Kiro provider.
type KiroConfig struct {
	Enabled        bool     `toml:"enabled"`
	DisplayName    string   `toml:"display_name"`
	Color          string   `toml:"color"`
	Meters         []string `toml:"meters"`
	Source         string   `toml:"source"`
	DBPath         string   `toml:"db_path"`
	IDETokenFile   string   `toml:"ide_token_file"`
	Binary         string   `toml:"binary"`
	RefreshWithCLI bool     `toml:"refresh_with_cli"`
	Region         string   `toml:"region"`
	Endpoint       string   `toml:"endpoint"`
	ProfileARN     string   `toml:"profile_arn"`
}

// CursorConfig configures the Cursor provider.
type CursorConfig struct {
	Enabled     bool     `toml:"enabled"`
	DisplayName string   `toml:"display_name"`
	Color       string   `toml:"color"`
	Meters      []string `toml:"meters"`
	Source      string   `toml:"source"`
	StateDB     string   `toml:"state_db"`
	AuthFile    string   `toml:"auth_file"`
	APIBase     string   `toml:"api_base"`
	WebBase     string   `toml:"web_base"`
}

// Common holds the settings every provider shares.
type Common struct {
	ID          string
	Enabled     bool
	DisplayName string
	Color       string
	Meters      []string
}

// Default returns the built-in configuration.
func Default() *Config {
	return &Config{
		RefreshInterval: Duration(60 * time.Second),
		Timeout:         Duration(20 * time.Second),
		Order:           slices.Clone(AllProviders),
		ScanWindows:     true,
		UI: UI{
			Theme:       "auto",
			Layout:      "auto",
			CardWidth:   36,
			BarStyle:    "blocks",
			Percent:     "used",
			ResetFormat: "relative",
			Clock:       "24h",
			WarnAt:      70,
			CriticalAt:  90,
			ShowSource:  true,
			ShowHelp:    true,
			Color:       "auto",
		},
		Codex: CodexConfig{
			Enabled:          true,
			DisplayName:      "Codex",
			Color:            "#10A37F",
			Meters:           []string{},
			Source:           "auto",
			Binary:           "codex",
			UseAppServer:     true,
			SessionsFallback: true,
			BaseURL:          "https://chatgpt.com/backend-api",
		},
		Kiro: KiroConfig{
			Enabled:        true,
			DisplayName:    "Kiro",
			Color:          "#9D6CFF",
			Meters:         []string{},
			Source:         "auto",
			Binary:         "kiro-cli",
			RefreshWithCLI: true,
		},
		Cursor: CursorConfig{
			Enabled:     true,
			DisplayName: "Cursor",
			Color:       "#4C9AFF",
			Meters:      []string{},
			Source:      "auto",
			APIBase:     "https://api2.cursor.sh",
			WebBase:     "https://cursor.com",
		},
	}
}

// DefaultPath returns the default config file location:
// $XDG_CONFIG_HOME/all-usage/config.toml (~/.config/... on Linux and macOS),
// or %APPDATA%\all-usage\config.toml on Windows.
func DefaultPath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(sys.WindowsAppData(), "all-usage", "config.toml")
	}
	return filepath.Join(sys.XDGConfigHome(), "all-usage", "config.toml")
}

// ResolvePath returns the config path to use and whether it was set
// explicitly (flag or ALL_USAGE_CONFIG), in which case it must exist.
func ResolvePath(flagPath string) (string, bool) {
	if flagPath != "" {
		return sys.ExpandPath(flagPath), true
	}
	if v := os.Getenv("ALL_USAGE_CONFIG"); v != "" {
		return sys.ExpandPath(v), true
	}
	return DefaultPath(), false
}

// Load builds the effective configuration from defaults, the config file and
// environment variables.
func Load(flagPath string) (*Config, error) {
	cfg := Default()
	path, explicit := ResolvePath(flagPath)
	md, err := toml.DecodeFile(path, cfg)
	switch {
	case err == nil:
		cfg.Path = path
		for _, k := range md.Undecoded() {
			cfg.Warnings = append(cfg.Warnings, fmt.Sprintf("%s: unknown setting %q (ignored)", sys.ShortPath(path), k.String()))
		}
	case errors.Is(err, fs.ErrNotExist) && !explicit:
		// No config file: defaults it is.
	default:
		var perr toml.ParseError
		if errors.As(err, &perr) {
			return nil, fmt.Errorf("config %s:\n%s", sys.ShortPath(path), perr.ErrorWithPosition())
		}
		return nil, fmt.Errorf("config %s: %w", sys.ShortPath(path), err)
	}
	cfg.applyEnv()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// applyEnv applies ALL_USAGE_* environment overrides.
func (c *Config) applyEnv() {
	if v := os.Getenv("ALL_USAGE_REFRESH"); v != "" {
		if d, err := ParseDuration(v); err == nil {
			c.RefreshInterval = Duration(d)
		} else {
			c.Warnings = append(c.Warnings, fmt.Sprintf("ALL_USAGE_REFRESH: %v", err))
		}
	}
	if v := os.Getenv("ALL_USAGE_TIMEOUT"); v != "" {
		if d, err := ParseDuration(v); err == nil {
			c.Timeout = Duration(d)
		} else {
			c.Warnings = append(c.Warnings, fmt.Sprintf("ALL_USAGE_TIMEOUT: %v", err))
		}
	}
	if v := os.Getenv("ALL_USAGE_THEME"); v != "" {
		c.UI.Theme = strings.ToLower(strings.TrimSpace(v))
	}
	if v := os.Getenv("ALL_USAGE_PROVIDERS"); v != "" {
		if err := c.SelectProviders(SplitList(v)); err != nil {
			c.Warnings = append(c.Warnings, fmt.Sprintf("ALL_USAGE_PROVIDERS: %v", err))
		}
	}
	if v := os.Getenv("ALL_USAGE_WINDOWS_HOME"); v != "" {
		c.WindowsHome = v
	}
	if os.Getenv("NO_COLOR") != "" {
		c.UI.Color = "never"
	}
}

// SelectProviders enables exactly the given providers, in that order.
func (c *Config) SelectProviders(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	var order []string
	for _, id := range ids {
		id = strings.ToLower(strings.TrimSpace(id))
		if !slices.Contains(AllProviders, id) {
			return fmt.Errorf("unknown provider %q (available: %s)", id, strings.Join(AllProviders, ", "))
		}
		if !slices.Contains(order, id) {
			order = append(order, id)
		}
	}
	c.Order = order
	c.Codex.Enabled = slices.Contains(order, Codex)
	c.Kiro.Enabled = slices.Contains(order, Kiro)
	c.Cursor.Enabled = slices.Contains(order, Cursor)
	return nil
}

// EnabledProviders returns enabled provider IDs in display order: the `order`
// list first, then any remaining enabled providers in default order.
func (c *Config) EnabledProviders() []string {
	var out []string
	add := func(id string) {
		if c.Common(id).Enabled && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	for _, id := range c.Order {
		add(id)
	}
	for _, id := range AllProviders {
		add(id)
	}
	return out
}

// Common returns the shared settings of a provider.
func (c *Config) Common(id string) Common {
	switch id {
	case Codex:
		return Common{ID: id, Enabled: c.Codex.Enabled, DisplayName: c.Codex.DisplayName, Color: c.Codex.Color, Meters: c.Codex.Meters}
	case Kiro:
		return Common{ID: id, Enabled: c.Kiro.Enabled, DisplayName: c.Kiro.DisplayName, Color: c.Kiro.Color, Meters: c.Kiro.Meters}
	case Cursor:
		return Common{ID: id, Enabled: c.Cursor.Enabled, DisplayName: c.Cursor.DisplayName, Color: c.Cursor.Color, Meters: c.Cursor.Meters}
	}
	return Common{ID: id}
}

var colorRe = regexp.MustCompile(`^(#[0-9A-Fa-f]{3}|#[0-9A-Fa-f]{6}|[0-9]{1,3})$`)

// Validate normalizes values and reports invalid settings.
func (c *Config) Validate() error {
	var errs []string
	bad := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }
	enum := func(name string, v *string, allowed []string) {
		*v = strings.ToLower(strings.TrimSpace(*v))
		if !slices.Contains(allowed, *v) {
			bad("%s: invalid value %q (expected one of: %s)", name, *v, strings.Join(allowed, ", "))
		}
	}
	color := func(name, v string) {
		if v != "" && !colorRe.MatchString(strings.TrimSpace(v)) {
			bad("%s: invalid color %q (use \"#RRGGBB\", \"#RGB\" or an ANSI index 0-255)", name, v)
		}
	}

	if c.RefreshInterval < 0 {
		bad("refresh_interval: must be >= 0 (0 disables auto-refresh)")
	} else if c.RefreshInterval > 0 && c.RefreshInterval.D() < 5*time.Second {
		c.Warnings = append(c.Warnings, "refresh_interval below 5s; using 5s")
		c.RefreshInterval = Duration(5 * time.Second)
	}
	if c.Timeout.D() < time.Second {
		bad("timeout: must be at least 1s")
	}
	var order []string
	for _, id := range c.Order {
		id = strings.ToLower(strings.TrimSpace(id))
		if !slices.Contains(AllProviders, id) {
			bad("order: unknown provider %q (available: %s)", id, strings.Join(AllProviders, ", "))
			continue
		}
		if !slices.Contains(order, id) {
			order = append(order, id)
		}
	}
	c.Order = order

	if !slices.Contains(Themes, strings.ToLower(strings.TrimSpace(c.UI.Theme))) {
		c.Warnings = append(c.Warnings, fmt.Sprintf("ui.theme: unknown theme %q, using \"auto\" (available: %s)", c.UI.Theme, strings.Join(Themes, ", ")))
		c.UI.Theme = "auto"
	}
	c.UI.Theme = strings.ToLower(strings.TrimSpace(c.UI.Theme))
	enum("ui.layout", &c.UI.Layout, Layouts)
	enum("ui.bar_style", &c.UI.BarStyle, BarStyles)
	enum("ui.percent", &c.UI.Percent, PercentModes)
	enum("ui.reset_format", &c.UI.ResetFormat, ResetFormats)
	enum("ui.clock", &c.UI.Clock, Clocks)
	enum("ui.color", &c.UI.Color, ColorModes)
	if c.UI.Columns < 0 || c.UI.Columns > 12 {
		bad("ui.columns: must be between 0 (auto) and 12")
	}
	if c.UI.CardWidth < 24 || c.UI.CardWidth > 200 {
		bad("ui.card_width: must be between 24 and 200")
	}
	if c.UI.WarnAt < 0 || c.UI.CriticalAt < 0 || c.UI.WarnAt > c.UI.CriticalAt {
		bad("ui.warn_at/ui.critical_at: need 0 <= warn_at <= critical_at (got %g, %g)", c.UI.WarnAt, c.UI.CriticalAt)
	}
	cs := c.UI.Colors
	for name, v := range map[string]string{"accent": cs.Accent, "text": cs.Text, "muted": cs.Muted, "border": cs.Border, "ok": cs.OK, "warn": cs.Warn, "critical": cs.Critical, "bar_empty": cs.BarEmpty} {
		color("ui.colors."+name, v)
	}

	enum("codex.source", &c.Codex.Source, CodexSources)
	enum("kiro.source", &c.Kiro.Source, KiroSources)
	enum("cursor.source", &c.Cursor.Source, CursorSources)
	color("codex.color", c.Codex.Color)
	color("kiro.color", c.Kiro.Color)
	color("cursor.color", c.Cursor.Color)
	for _, u := range []struct{ name, v string }{
		{"codex.base_url", c.Codex.BaseURL}, {"kiro.endpoint", c.Kiro.Endpoint},
		{"cursor.api_base", c.Cursor.APIBase}, {"cursor.web_base", c.Cursor.WebBase},
	} {
		if u.v != "" && !strings.HasPrefix(u.v, "https://") && !strings.HasPrefix(u.v, "http://") {
			bad("%s: must be an http(s) URL", u.name)
		}
	}
	if c.Codex.BaseURL == "" {
		c.Codex.BaseURL = Default().Codex.BaseURL
	}
	if c.Cursor.APIBase == "" {
		c.Cursor.APIBase = Default().Cursor.APIBase
	}
	if c.Cursor.WebBase == "" {
		c.Cursor.WebBase = Default().Cursor.WebBase
	}
	for _, p := range []*string{&c.Codex.DisplayName, &c.Kiro.DisplayName, &c.Cursor.DisplayName} {
		*p = strings.TrimSpace(*p)
	}
	if c.Codex.DisplayName == "" {
		c.Codex.DisplayName = "Codex"
	}
	if c.Kiro.DisplayName == "" {
		c.Kiro.DisplayName = "Kiro"
	}
	if c.Cursor.DisplayName == "" {
		c.Cursor.DisplayName = "Cursor"
	}

	if len(errs) > 0 {
		where := "configuration"
		if c.Path != "" {
			where = sys.ShortPath(c.Path)
		}
		return fmt.Errorf("invalid %s:\n  - %s", where, strings.Join(errs, "\n  - "))
	}
	return nil
}

// SplitList splits a comma/space separated list.
func SplitList(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == ';' })
}

// Duration is a time.Duration that reads from TOML as "90s"/"5m" (or a plain
// number of seconds) and writes back as a compact string.
type Duration time.Duration

// D returns the value as time.Duration.
func (d Duration) D() time.Duration { return time.Duration(d) }

// UnmarshalTOML implements toml.Unmarshaler.
func (d *Duration) UnmarshalTOML(v any) error {
	switch t := v.(type) {
	case string:
		pd, err := ParseDuration(t)
		if err != nil {
			return err
		}
		*d = Duration(pd)
	case int64:
		*d = Duration(time.Duration(t) * time.Second)
	case float64:
		*d = Duration(time.Duration(t * float64(time.Second)))
	default:
		return fmt.Errorf("invalid duration %v (use a string like \"60s\" or \"5m\")", v)
	}
	return nil
}

// MarshalText implements encoding.TextMarshaler.
func (d Duration) MarshalText() ([]byte, error) { return []byte(FormatDuration(d.D())), nil }

// ParseDuration parses "90s", "5m", "1h30m" or a bare number of seconds.
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return 0, nil
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		return time.Duration(n * float64(time.Second)), nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q (examples: \"30s\", \"5m\", \"1h\")", s)
	}
	return d, nil
}

// FormatDuration renders d compactly ("90s" -> "1m30s", "60s" -> "1m").
func FormatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	if d%time.Second != 0 {
		return d.String()
	}
	var b strings.Builder
	if h := d / time.Hour; h > 0 {
		fmt.Fprintf(&b, "%dh", h)
		d -= h * time.Hour
	}
	if m := d / time.Minute; m > 0 {
		fmt.Fprintf(&b, "%dm", m)
		d -= m * time.Minute
	}
	if s := d / time.Second; s > 0 {
		fmt.Fprintf(&b, "%ds", s)
	}
	return b.String()
}
