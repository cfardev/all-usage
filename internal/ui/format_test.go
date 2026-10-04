package ui

import (
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/cfardev/all-usage/internal/config"
	"github.com/cfardev/all-usage/internal/core"
)

func TestFormatPercent(t *testing.T) {
	for in, want := range map[float64]string{0: "0%", 22: "22%", 41.611: "41.6%", 0.04: "0%", 0.4: "0.4%", 99.96: "100%", 112.4: "112%", 53.0001: "53%"} {
		if got := FormatPercent(in); got != want {
			t.Errorf("FormatPercent(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatNumberAndMoney(t *testing.T) {
	for in, want := range map[float64]string{0: "0", 1000: "1,000", 1234567.891: "1,234,567.89", 416.11: "416.11", 10.5: "10.5", -2500: "-2,500"} {
		if got := FormatNumber(in); got != want {
			t.Errorf("FormatNumber(%v) = %q, want %q", in, got, want)
		}
	}
	cases := []struct {
		v    float64
		cur  string
		want string
	}{{148.66, "USD", "$148.66"}, {1500, "", "$1,500.00"}, {0.04, "USD", "$0.04"}, {3, "EUR", "€3.00"}, {7, "MXN", "7.00 MXN"}, {-1.5, "USD", "-$1.50"}}
	for _, c := range cases {
		if got := FormatMoney(c.v, c.cur); got != c.want {
			t.Errorf("FormatMoney(%v, %q) = %q, want %q", c.v, c.cur, got, c.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	for in, want := range map[time.Duration]string{
		30 * time.Second: "30s", 12 * time.Minute: "12m", 2*time.Hour + 14*time.Minute: "2h 14m", 5 * time.Hour: "5h",
		3*24*time.Hour + 4*time.Hour: "3d 4h", 28 * 24 * time.Hour: "28d", 12*24*time.Hour + 5*time.Hour: "12d",
	} {
		if got := FormatDuration(in); got != want {
			t.Errorf("FormatDuration(%v) = %q, want %q", in, got, want)
		}
	}
	if got := CompactDuration(2*time.Hour + 14*time.Minute); got != "2h14m" {
		t.Errorf("CompactDuration = %q", got)
	}
}

func TestWindowLabel(t *testing.T) {
	cases := map[time.Duration][2]string{
		5 * time.Hour: {"5-hour", "5h"}, 7 * 24 * time.Hour: {"Weekly", "wk"}, 24 * time.Hour: {"Daily", "day"},
		30 * 24 * time.Hour: {"Monthly", "mo"}, 90 * time.Minute: {"90-minute", "90m"}, 0: {"Usage", ""},
	}
	for d, want := range cases {
		if l, s := WindowLabel(d); l != want[0] || s != want[1] {
			t.Errorf("WindowLabel(%v) = %q,%q; want %q,%q", d, l, s, want[0], want[1])
		}
	}
}

func TestTitleCase(t *testing.T) {
	for in, want := range map[string]string{"KIRO PRO": "Kiro Pro", "pro_plus": "Pro Plus", "": "", "  team ": "Team", "ünïcode": "Ünïcode"} {
		if got := TitleCase(in); got != want {
			t.Errorf("TitleCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatReset(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.Local)
	o := Options{ResetFormat: "relative", Clock: "24h"}
	if got := o.FormatReset(now.Add(2*time.Hour+5*time.Minute), now); got != "in 2h 5m" {
		t.Errorf("relative = %q", got)
	}
	if got := o.FormatReset(now.Add(-time.Minute), now); got != "resetting now" {
		t.Errorf("past = %q", got)
	}
	o.ResetFormat = "absolute"
	if got := o.FormatReset(now.Add(3*time.Hour), now); got != "15:00" {
		t.Errorf("absolute today = %q", got)
	}
	o.Clock = "12h"
	if got := o.FormatReset(now.Add(3*time.Hour), now); got != "3:00pm" {
		t.Errorf("absolute 12h = %q", got)
	}
	if got := o.FormatReset(now.Add(48*time.Hour), now); got != "Mon 12:00pm" {
		t.Errorf("absolute weekday = %q", got)
	}
	if got := o.FormatReset(now.AddDate(0, 1, 0), now); got != "Nov 3" {
		t.Errorf("absolute date = %q", got)
	}
	o.ResetFormat, o.Clock = "both", "24h"
	if got := o.FormatReset(now.Add(time.Hour), now); got != "in 1h · 13:00" {
		t.Errorf("both = %q", got)
	}
}

func TestOptionsPercentModes(t *testing.T) {
	o := Options{WarnAt: 70, CriticalAt: 90}
	if o.LevelFor(10) != LevelOK || o.LevelFor(70) != LevelWarn || o.LevelFor(95) != LevelCritical {
		t.Error("wrong levels")
	}
	if got := o.PctText(22); got != "22%" {
		t.Errorf("used = %q", got)
	}
	o.Remaining = true
	if got := o.PctText(22); got != "78% left" {
		t.Errorf("remaining = %q", got)
	}
	if got := o.DisplayPct(130); got != 0 {
		t.Errorf("remaining must not go negative: %v", got)
	}
}

func TestUsedOfLimit(t *testing.T) {
	m := core.Meter{Used: core.F64(416.11), Limit: core.F64(1000), Unit: "credits"}
	if got := UsedOfLimit(m); got != "416.11 / 1,000 credits" {
		t.Errorf("got %q", got)
	}
	m = core.Meter{Used: core.F64(12), Limit: core.F64(50), Unit: "usd"}
	if got := UsedOfLimit(m); got != "$12.00 / $50.00" {
		t.Errorf("got %q", got)
	}
	if got := UsedOfLimit(core.Meter{Percent: core.F64(3)}); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestBarWidth(t *testing.T) {
	for _, theme := range config.Themes {
		th := NewTheme(theme, config.Colors{})
		for _, style := range config.BarStyles {
			for _, pct := range []float64{-5, 0, 0.5, 12.3, 50, 99.9, 100, 250} {
				for _, w := range []int{1, 7, 20} {
					if got := lipgloss.Width(th.Bar(pct, w, style, th.OK)); got != w {
						t.Fatalf("theme %s style %s pct %v width %d: rendered width %d", theme, style, pct, w, got)
					}
				}
			}
		}
	}
	if NewTheme("dark", config.Colors{}).Bar(50, 0, "blocks", nil) != "" {
		t.Error("zero width must render nothing")
	}
}

func TestThemeOverridesAndFallback(t *testing.T) {
	th := NewTheme("dracula", config.Colors{Accent: "#123456"})
	if th.Accent != lipgloss.Color("#123456") {
		t.Errorf("override not applied: %v", th.Accent)
	}
	if NewTheme("does-not-exist", config.Colors{}).Name != "auto" {
		t.Error("unknown theme should fall back to auto")
	}
}
