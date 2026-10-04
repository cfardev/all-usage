// Package render prints usage results without the TUI: an aligned table, a
// one-line summary for status bars, JSON, or a user-supplied Go template.
package render

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/template"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/cfardev/all-usage/internal/config"
	"github.com/cfardev/all-usage/internal/core"
	"github.com/cfardev/all-usage/internal/ui"
)

// Options controls plain rendering.
type Options struct {
	UI         ui.Options
	Theme      ui.Theme
	Now        time.Time
	ShowSource bool
	// Colors maps provider IDs to brand colors.
	Colors map[string]string
}

const barWidth = 20

// Table prints every provider with aligned meters.
func Table(w io.Writer, results []core.Result, o Options) {
	t := o.Theme
	bold := lipgloss.NewStyle().Bold(true).Foreground(t.Text)
	muted := lipgloss.NewStyle().Foreground(t.Muted)
	if t.Mono {
		muted = muted.Faint(true)
	}
	warn := lipgloss.NewStyle().Foreground(t.Warn)
	crit := lipgloss.NewStyle().Foreground(t.Critical)
	accent := lipgloss.NewStyle().Foreground(t.Accent)

	labelW := 0
	for _, r := range results {
		if r.Usage != nil {
			for _, m := range r.Usage.Meters {
				labelW = max(labelW, lipgloss.Width(m.Label))
			}
		}
	}
	labelW = min(labelW, 28)
	pctW := 6
	if o.UI.Remaining {
		pctW = 10
	}

	for i, r := range results {
		if i > 0 {
			fmt.Fprintln(w)
		}
		dot := lipgloss.NewStyle().Foreground(t.ProviderColor(o.Colors[r.ID])).Render("●")
		head := dot + " " + bold.Render(r.Name)
		if r.Usage != nil && r.Usage.Plan != "" {
			head += "  " + accent.Render(r.Usage.Plan)
		}
		if r.Usage != nil && r.Usage.Account != "" {
			head += "  " + muted.Render(r.Usage.Account)
		}
		if o.ShowSource && r.Usage != nil {
			src := r.Usage.Source
			if r.Usage.AsOf != nil {
				head += "  " + warn.Render(src+" · as of "+ui.FormatAgo(*r.Usage.AsOf, o.Now))
			} else {
				head += "  " + muted.Render(fmt.Sprintf("%s · %.1fs", src, r.Duration.Seconds()))
			}
		}
		fmt.Fprintln(w, head)

		if r.Usage == nil {
			e := r.Err
			if e == nil {
				e = &core.Error{Kind: core.KindInternal, Msg: "no data"}
			}
			style, icon := crit, "✗"
			if e.Kind == core.KindNotConfigured {
				style, icon = warn, "○"
			}
			fmt.Fprintf(w, "  %s %s\n", style.Render(icon+" "+e.Title()+":"), e.Full())
			if e.Hint != "" {
				fmt.Fprintf(w, "    %s\n", muted.Render("→ "+e.Hint))
			}
			continue
		}
		if len(r.Usage.Meters) == 0 {
			fmt.Fprintf(w, "  %s\n", muted.Render("no usage data"))
		}
		for _, m := range r.Usage.Meters {
			label := m.Label + strings.Repeat(" ", max(0, labelW-lipgloss.Width(m.Label)))
			if !m.HasPercent() {
				line := "  " + label + "  " + bold.Render(m.Value)
				if m.Detail != "" {
					line += "  " + muted.Render(m.Detail)
				}
				fmt.Fprintln(w, line)
				continue
			}
			used := m.Pct()
			color := t.LevelColor(o.UI.LevelFor(used))
			pct := lipgloss.NewStyle().Foreground(color).Bold(true).Render(fmt.Sprintf("%*s", pctW, o.UI.PctText(used)))
			var info []string
			if s := ui.UsedOfLimit(m); s != "" {
				info = append(info, s)
			}
			if s := resetText(m, o); s != "" {
				info = append(info, s)
			}
			if m.Detail != "" {
				info = append(info, m.Detail)
			}
			fmt.Fprintf(w, "  %s  %s %s  %s\n", label, t.Bar(o.UI.DisplayPct(used), barWidth, o.UI.BarStyle, color), pct,
				muted.Render(strings.Join(info, " · ")))
		}
		for _, n := range r.Usage.Notes {
			fmt.Fprintf(w, "  %s\n", warn.Render("! "+n))
		}
		if wr := r.Usage.Warning; wr != nil {
			fmt.Fprintf(w, "  %s\n", warn.Render("⚠ Live data unavailable: "+wr.Full()))
			if wr.Hint != "" {
				fmt.Fprintf(w, "    %s\n", muted.Render("→ "+wr.Hint))
			}
		}
	}
}

func resetText(m core.Meter, o Options) string {
	if m.ResetsAt == nil {
		return ""
	}
	if !m.ResetsAt.After(o.Now) {
		return "resetting now"
	}
	return "resets " + o.UI.FormatReset(*m.ResetsAt, o.Now)
}

// headline returns the meters that summarize a provider.
func headline(u *core.Usage) []core.Meter {
	var out []core.Meter
	for _, m := range u.Meters {
		if m.Headline && m.HasPercent() {
			out = append(out, m)
		}
	}
	if len(out) == 0 {
		for _, m := range u.Meters {
			if m.HasPercent() {
				return []core.Meter{m}
			}
		}
	}
	return out
}

// Short prints a single line such as "Codex 5h 22% · wk 56% | Kiro 42% | Cursor 12%".
func Short(w io.Writer, results []core.Result, o Options) {
	parts := make([]string, 0, len(results))
	for _, r := range results {
		if r.Usage == nil {
			parts = append(parts, r.Name+" ✗")
			continue
		}
		hs := headline(r.Usage)
		var ms []string
		for _, m := range hs {
			s := ui.FormatPercent(o.UI.DisplayPct(m.Pct()))
			if len(hs) > 1 && m.Short != "" {
				s = m.Short + " " + s
			}
			ms = append(ms, s)
		}
		if len(ms) == 0 {
			ms = append(ms, "–")
		}
		s := r.Name + " " + strings.Join(ms, " · ")
		if r.Usage.Stale {
			s += "*"
		}
		parts = append(parts, s)
	}
	fmt.Fprintln(w, strings.Join(parts, " | "))
}

// JSONResult is the JSON shape of one provider.
type JSONResult struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	OK         bool        `json:"ok"`
	Usage      *core.Usage `json:"usage,omitempty"`
	Error      *core.Error `json:"error,omitempty"`
	DurationMS int64       `json:"duration_ms"`
}

// JSON prints machine-readable results.
func JSON(w io.Writer, results []core.Result, now time.Time) error {
	out := struct {
		GeneratedAt time.Time    `json:"generated_at"`
		Providers   []JSONResult `json:"providers"`
	}{GeneratedAt: now, Providers: make([]JSONResult, 0, len(results))}
	for _, r := range results {
		out.Providers = append(out.Providers, JSONResult{
			ID: r.ID, Name: r.Name, OK: r.OK(), Usage: r.Usage, Error: r.Err, DurationMS: r.Duration.Milliseconds(),
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// TemplateProvider is the per-provider data available to --template.
type TemplateProvider struct {
	ID, Name, Plan, Account, Source string
	OK, Stale                       bool
	Error, Hint                     string
	Meters                          []core.Meter
	Headline                        []core.Meter
}

// Template renders results with a Go text/template. The data is
// {.Now, .Providers}; see TemplateFuncs for helpers.
func Template(w io.Writer, text string, results []core.Result, o Options) error {
	t, err := template.New("all-usage").Funcs(TemplateFuncs(o)).Parse(text)
	if err != nil {
		return fmt.Errorf("template: %w", err)
	}
	data := struct {
		Now       time.Time
		Providers []TemplateProvider
	}{Now: o.Now}
	for _, r := range results {
		p := TemplateProvider{ID: r.ID, Name: r.Name, OK: r.OK()}
		if r.Usage != nil {
			p.Plan, p.Account, p.Source, p.Stale = r.Usage.Plan, r.Usage.Account, r.Usage.Source, r.Usage.Stale
			p.Meters, p.Headline = r.Usage.Meters, headline(r.Usage)
		}
		if r.Err != nil {
			p.Error, p.Hint = r.Err.Msg, r.Err.Hint
		}
		data.Providers = append(data.Providers, p)
	}
	if err := t.Execute(w, data); err != nil {
		return fmt.Errorf("template: %w", err)
	}
	if !strings.HasSuffix(text, "\n") {
		fmt.Fprintln(w)
	}
	return nil
}

// TemplateFuncs are the helpers available in --template.
func TemplateFuncs(o Options) template.FuncMap {
	plain := o
	plain.Theme = ui.NewTheme("mono", config.Colors{})
	return template.FuncMap{
		"pct": func(m core.Meter) string {
			if !m.HasPercent() {
				return m.Value
			}
			return ui.FormatPercent(o.UI.DisplayPct(m.Pct()))
		},
		"used": func(m core.Meter) float64 { return m.Pct() },
		"left": func(m core.Meter) float64 { return max(0, 100-m.Pct()) },
		"reset": func(m core.Meter) string {
			if m.ResetsAt == nil {
				return ""
			}
			return ui.CompactDuration(m.ResetsAt.Sub(o.Now))
		},
		"bar": func(m core.Meter, width int) string {
			return plain.Theme.Bar(o.UI.DisplayPct(m.Pct()), width, o.UI.BarStyle, plain.Theme.Text)
		},
		"money": func(v float64) string { return ui.FormatMoney(v, "USD") },
		"num":   ui.FormatNumber,
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"join":  strings.Join,
	}
}
