// Package ui holds presentation helpers shared by the TUI and the plain
// renderers: themes, progress bars and human-friendly formatting.
package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/cfardev/all-usage/internal/core"
)

// Options are the formatting preferences derived from the config.
type Options struct {
	Remaining   bool   // show remaining instead of used percentages
	ResetFormat string // relative | absolute | both
	Clock       string // 24h | 12h
	WarnAt      float64
	CriticalAt  float64
	BarStyle    string
}

// Level is a usage severity.
type Level int

// Usage severities.
const (
	LevelOK Level = iota
	LevelWarn
	LevelCritical
)

// LevelFor returns the severity of a used percentage.
func (o Options) LevelFor(usedPct float64) Level {
	switch {
	case usedPct >= o.CriticalAt:
		return LevelCritical
	case usedPct >= o.WarnAt:
		return LevelWarn
	default:
		return LevelOK
	}
}

// DisplayPct converts a used percentage into the configured display mode.
func (o Options) DisplayPct(usedPct float64) float64 {
	if o.Remaining {
		return math.Max(0, 100-usedPct)
	}
	return usedPct
}

// PctText renders a meter percentage, e.g. "22%" or "78% left".
func (o Options) PctText(usedPct float64) string {
	s := FormatPercent(o.DisplayPct(usedPct))
	if o.Remaining {
		return s + " left"
	}
	return s
}

// FormatPercent renders 22 -> "22%", 41.611 -> "41.6%", 0.4 -> "0.4%".
func FormatPercent(p float64) string {
	if math.IsNaN(p) || math.IsInf(p, 0) {
		return "–"
	}
	r := math.Round(p*10) / 10
	if r == math.Trunc(r) || math.Abs(r) >= 100 {
		return strconv.FormatFloat(math.Round(p), 'f', 0, 64) + "%"
	}
	return strconv.FormatFloat(r, 'f', 1, 64) + "%"
}

// FormatNumber renders a number with thousands separators and at most two
// decimals: 1000 -> "1,000", 416.114 -> "416.11".
func FormatNumber(v float64) string {
	neg := v < 0
	v = math.Abs(v)
	s := strconv.FormatFloat(math.Round(v*100)/100, 'f', 2, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	intPart, frac, _ := strings.Cut(s, ".")
	var b strings.Builder
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if frac != "" {
		b.WriteByte('.')
		b.WriteString(frac)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// FormatMoney renders an amount: ("USD", 148.66) -> "$148.66".
func FormatMoney(v float64, currency string) string {
	s := strconv.FormatFloat(math.Abs(v), 'f', 2, 64)
	intPart, frac, _ := strings.Cut(s, ".")
	n, _ := strconv.ParseFloat(intPart, 64)
	s = FormatNumber(n) + "." + frac
	sign := ""
	if v < 0 {
		sign = "-"
	}
	switch strings.ToUpper(currency) {
	case "", "USD":
		return sign + "$" + s
	case "EUR":
		return sign + "€" + s
	case "GBP":
		return sign + "£" + s
	default:
		return sign + s + " " + strings.ToUpper(currency)
	}
}

// FormatAmount renders a meter quantity in its unit.
func FormatAmount(v float64, unit string) string {
	switch unit {
	case "usd":
		return FormatMoney(v, "USD")
	case "":
		return FormatNumber(v)
	default:
		return FormatNumber(v) + " " + unit
	}
}

// UsedOfLimit renders "416.11 / 1,000 credits" or "$12.00 / $50.00".
func UsedOfLimit(m core.Meter) string {
	if m.Used == nil || m.Limit == nil {
		return ""
	}
	if m.Unit == "usd" {
		return FormatMoney(*m.Used, "USD") + " / " + FormatMoney(*m.Limit, "USD")
	}
	s := FormatNumber(*m.Used) + " / " + FormatNumber(*m.Limit)
	if m.Unit != "" {
		s += " " + m.Unit
	}
	return s
}

// FormatDuration renders a duration coarsely: "45s", "12m", "2h 14m", "3d 4h".
func FormatDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		if m == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh %dm", h, m)
	default:
		days := int(d.Hours()) / 24
		h := int(d.Hours()) % 24
		if h == 0 || days >= 10 {
			return fmt.Sprintf("%dd", days)
		}
		return fmt.Sprintf("%dd %dh", days, h)
	}
}

// CompactDuration is FormatDuration without spaces ("2h14m").
func CompactDuration(d time.Duration) string {
	return strings.ReplaceAll(FormatDuration(d), " ", "")
}

// FormatAgo renders how long ago t was: "just now", "12s ago", "3h ago".
func FormatAgo(t, now time.Time) string {
	d := now.Sub(t)
	if d < 5*time.Second {
		return "just now"
	}
	return FormatDuration(d) + " ago"
}

// FormatClock renders an absolute time relative to now: "14:05" today,
// "Tue 14:05" within a week, otherwise "Nov 1".
func (o Options) FormatClock(t, now time.Time) string {
	t = t.Local()
	now = now.Local()
	hm := "15:04"
	if o.Clock == "12h" {
		hm = "3:04pm"
	}
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()
	switch {
	case y1 == y2 && m1 == m2 && d1 == d2:
		return t.Format(hm)
	case t.After(now) && t.Sub(now) < 6*24*time.Hour:
		return t.Format("Mon " + hm)
	case y1 == y2:
		return t.Format("Jan 2")
	default:
		return t.Format("Jan 2 2006")
	}
}

// FormatReset renders when a meter resets, honoring ResetFormat.
func (o Options) FormatReset(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	if !t.After(now) {
		return "resetting now"
	}
	rel := "in " + FormatDuration(t.Sub(now))
	switch o.ResetFormat {
	case "absolute":
		return o.FormatClock(t, now)
	case "both":
		return rel + " · " + o.FormatClock(t, now)
	default:
		return rel
	}
}

// ShortReset renders a reset time for compact layouts ("2h14m", "Tue 14:05").
func (o Options) ShortReset(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	if !t.After(now) {
		return "now"
	}
	if o.ResetFormat == "absolute" {
		return o.FormatClock(t, now)
	}
	return CompactDuration(t.Sub(now))
}

// WindowLabel names a rate-limit window by its length: 5h -> "5-hour", 7d -> "Weekly".
func WindowLabel(d time.Duration) (label, short string) {
	switch {
	case d <= 0:
		return "Usage", ""
	case d == 7*24*time.Hour:
		return "Weekly", "wk"
	case d == 24*time.Hour:
		return "Daily", "day"
	case d >= 28*24*time.Hour && d <= 31*24*time.Hour:
		return "Monthly", "mo"
	case d%(24*time.Hour) == 0:
		n := int(d.Hours()) / 24
		return fmt.Sprintf("%d-day", n), fmt.Sprintf("%dd", n)
	case d%time.Hour == 0:
		n := int(d.Hours())
		return fmt.Sprintf("%d-hour", n), fmt.Sprintf("%dh", n)
	default:
		n := int(d.Minutes())
		return fmt.Sprintf("%d-minute", n), fmt.Sprintf("%dm", n)
	}
}

// TitleCase converts "KIRO PRO" or "pro_plus" into "Kiro Pro" / "Pro Plus".
func TitleCase(s string) string {
	s = strings.NewReplacer("_", " ", "-", " ").Replace(strings.TrimSpace(s))
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}
