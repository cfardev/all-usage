package ui

import (
	"math"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/cfardev/all-usage/internal/config"
)

// Theme is a resolved color palette.
type Theme struct {
	Name     string
	Accent   lipgloss.TerminalColor
	Text     lipgloss.TerminalColor
	Muted    lipgloss.TerminalColor
	Border   lipgloss.TerminalColor
	OK       lipgloss.TerminalColor
	Warn     lipgloss.TerminalColor
	Critical lipgloss.TerminalColor
	BarEmpty lipgloss.TerminalColor
	// Mono is true when colors are unavailable (mono theme, NO_COLOR, pipes);
	// bars then use distinct glyphs instead of colors.
	Mono bool
}

type palette struct{ accent, text, muted, border, ok, warn, critical, barEmpty string }

var palettes = map[string]palette{
	"dark":       {"#A78BFA", "#E5E7EB", "#9CA3AF", "#4B5563", "#34D399", "#FBBF24", "#F87171", "#374151"},
	"light":      {"#6D28D9", "#111827", "#6B7280", "#D1D5DB", "#059669", "#B45309", "#DC2626", "#E5E7EB"},
	"dracula":    {"#BD93F9", "#F8F8F2", "#6272A4", "#44475A", "#50FA7B", "#F1FA8C", "#FF5555", "#44475A"},
	"nord":       {"#88C0D0", "#ECEFF4", "#7B88A1", "#4C566A", "#A3BE8C", "#EBCB8B", "#BF616A", "#3B4252"},
	"catppuccin": {"#CBA6F7", "#CDD6F4", "#9399B2", "#45475A", "#A6E3A1", "#F9E2AF", "#F38BA8", "#313244"},
	"gruvbox":    {"#FE8019", "#EBDBB2", "#A89984", "#504945", "#B8BB26", "#FABD2F", "#FB4934", "#3C3836"},
	"tokyonight": {"#7AA2F7", "#C0CAF5", "#737AA2", "#3B4261", "#9ECE6A", "#E0AF68", "#F7768E", "#292E42"},
}

// SetColorMode configures color output globally: "auto" detects the
// terminal, "always" forces colors, "never" disables them.
func SetColorMode(mode string) {
	switch mode {
	case "never":
		lipgloss.SetColorProfile(termenv.Ascii)
	case "always":
		ct := strings.ToLower(os.Getenv("COLORTERM"))
		if ct == "truecolor" || ct == "24bit" {
			lipgloss.SetColorProfile(termenv.TrueColor)
		} else {
			lipgloss.SetColorProfile(termenv.ANSI256)
		}
	}
}

// ColorsEnabled reports whether the active color profile renders colors.
func ColorsEnabled() bool { return lipgloss.ColorProfile() != termenv.Ascii }

// NewTheme resolves a theme by name, applying per-color overrides.
func NewTheme(name string, ov config.Colors) Theme {
	t := Theme{Name: name}
	switch name {
	case "mono":
		none := lipgloss.NoColor{}
		t.Accent, t.Text, t.Muted, t.Border, t.OK, t.Warn, t.Critical, t.BarEmpty = none, none, none, none, none, none, none, none
		t.Mono = true
	case "auto", "":
		t.Name = "auto"
		d, l := palettes["dark"], palettes["light"]
		ad := func(light, dark string) lipgloss.TerminalColor {
			return lipgloss.AdaptiveColor{Light: light, Dark: dark}
		}
		t.Accent, t.Text, t.Muted, t.Border = ad(l.accent, d.accent), ad(l.text, d.text), ad(l.muted, d.muted), ad(l.border, d.border)
		t.OK, t.Warn, t.Critical, t.BarEmpty = ad(l.ok, d.ok), ad(l.warn, d.warn), ad(l.critical, d.critical), ad(l.barEmpty, d.barEmpty)
	default:
		p, ok := palettes[name]
		if !ok {
			return NewTheme("auto", ov)
		}
		c := func(s string) lipgloss.TerminalColor { return lipgloss.Color(s) }
		t.Accent, t.Text, t.Muted, t.Border = c(p.accent), c(p.text), c(p.muted), c(p.border)
		t.OK, t.Warn, t.Critical, t.BarEmpty = c(p.ok), c(p.warn), c(p.critical), c(p.barEmpty)
	}
	set := func(dst *lipgloss.TerminalColor, v string) {
		if v = strings.TrimSpace(v); v != "" {
			*dst = lipgloss.Color(v)
		}
	}
	set(&t.Accent, ov.Accent)
	set(&t.Text, ov.Text)
	set(&t.Muted, ov.Muted)
	set(&t.Border, ov.Border)
	set(&t.OK, ov.OK)
	set(&t.Warn, ov.Warn)
	set(&t.Critical, ov.Critical)
	set(&t.BarEmpty, ov.BarEmpty)
	if !ColorsEnabled() {
		t.Mono = true
	}
	return t
}

// LevelColor returns the color of a severity level.
func (t Theme) LevelColor(l Level) lipgloss.TerminalColor {
	switch l {
	case LevelCritical:
		return t.Critical
	case LevelWarn:
		return t.Warn
	default:
		return t.OK
	}
}

// ProviderColor returns a provider's brand color (the accent when unset).
func (t Theme) ProviderColor(hex string) lipgloss.TerminalColor {
	if t.Name == "mono" || hex == "" {
		return t.Accent
	}
	return lipgloss.Color(hex)
}

// Bar renders a horizontal gauge of width cells for pct (0-100).
func (t Theme) Bar(pct float64, width int, style string, fill lipgloss.TerminalColor) string {
	if width <= 0 {
		return ""
	}
	if math.IsNaN(pct) {
		pct = 0
	}
	pct = math.Max(0, math.Min(100, pct))
	fs := lipgloss.NewStyle().Foreground(fill)
	ts := lipgloss.NewStyle().Foreground(t.BarEmpty)
	cells := func() int { return int(math.Round(pct / 100 * float64(width))) }
	switch style {
	case "ascii":
		n := cells()
		return fs.Render(strings.Repeat("#", n)) + ts.Render(strings.Repeat("-", width-n))
	case "line":
		n := cells()
		track := "━"
		if t.Mono {
			track = "─"
		}
		return fs.Render(strings.Repeat("━", n)) + ts.Render(strings.Repeat(track, width-n))
	case "dots":
		n := cells()
		return fs.Render(strings.Repeat("⣿", n)) + ts.Render(strings.Repeat("⣀", width-n))
	default: // blocks, with 1/8-cell precision
		eighths := int(math.Round(pct / 100 * float64(width*8)))
		full, rem := eighths/8, eighths%8
		var b strings.Builder
		b.WriteString(fs.Render(strings.Repeat("█", full)))
		usedCells := full
		if rem > 0 && full < width {
			partial := string([]rune(" ▏▎▍▌▋▊▉")[rem])
			if t.Mono {
				b.WriteString(partial)
			} else {
				b.WriteString(fs.Background(t.BarEmpty).Render(partial))
			}
			usedCells++
		}
		track := "█"
		if t.Mono {
			track = "░"
		}
		b.WriteString(ts.Render(strings.Repeat(track, width-usedCells)))
		return b.String()
	}
}
