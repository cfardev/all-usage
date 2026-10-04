package render

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/cfardev/all-usage/internal/config"
	"github.com/cfardev/all-usage/internal/core"
	"github.com/cfardev/all-usage/internal/ui"
)

var now = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

func results() []core.Result {
	reset := now.Add(2*time.Hour + 14*time.Minute)
	asOf := now.Add(-26 * time.Hour)
	return []core.Result{
		{ID: "codex", Name: "Codex", Duration: 300 * time.Millisecond, Usage: &core.Usage{
			Provider: "codex", Name: "Codex", Plan: "Team", Source: "session log", AsOf: &asOf, Stale: true,
			Meters: []core.Meter{
				{ID: "primary", Label: "5-hour limit", Short: "5h", Headline: true, Percent: core.F64(22), ResetsAt: &reset},
				{ID: "secondary", Label: "Weekly limit", Short: "wk", Headline: true, Percent: core.F64(56.4)},
				{ID: "credits", Label: "Credits", Value: "12.5"},
			},
			Warning: &core.Error{Kind: core.KindAuth, Msg: "Codex login expired", Hint: "Run `codex login`."},
		}},
		{ID: "kiro", Name: "Kiro", Usage: &core.Usage{
			Provider: "kiro", Name: "Kiro", Plan: "Kiro Pro", Source: "kiro-cli login",
			Meters: []core.Meter{{ID: "credit", Label: "Credits", Headline: true, Percent: core.F64(41.6),
				Used: core.F64(416.11), Limit: core.F64(1000), Unit: "credits"}},
		}},
		{ID: "cursor", Name: "Cursor", Err: &core.Error{Kind: core.KindNotConfigured, Msg: "no Cursor login found", Hint: "Sign in."}},
	}
}

func opts() Options {
	lipgloss.SetColorProfile(termenv.Ascii)
	return Options{
		UI:    ui.Options{ResetFormat: "relative", Clock: "24h", WarnAt: 70, CriticalAt: 90, BarStyle: "blocks"},
		Theme: ui.NewTheme("dark", config.Colors{}), Now: now, ShowSource: true,
	}
}

func TestTable(t *testing.T) {
	var b bytes.Buffer
	Table(&b, results(), opts())
	out := b.String()
	for _, want := range []string{
		"● Codex  Team  session log · as of 1d 2h ago",
		"5-hour limit", "22%", "resets in 2h 14m",
		"Credits       12.5",
		"⚠ Live data unavailable: Codex login expired", "→ Run `codex login`.",
		"● Kiro  Kiro Pro", "416.11 / 1,000 credits",
		"○ Not signed in: no Cursor login found", "→ Sign in.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q:\n%s", want, out)
		}
	}
}

func TestShort(t *testing.T) {
	var b bytes.Buffer
	Short(&b, results(), opts())
	if got := b.String(); got != "Codex 5h 22% · wk 56.4%* | Kiro 41.6% | Cursor ✗\n" {
		t.Errorf("short = %q", got)
	}
	o := opts()
	o.UI.Remaining = true
	b.Reset()
	Short(&b, results()[1:2], o)
	if got := b.String(); got != "Kiro 58.4%\n" {
		t.Errorf("remaining short = %q", got)
	}
}

func TestJSON(t *testing.T) {
	var b bytes.Buffer
	if err := JSON(&b, results(), now); err != nil {
		t.Fatal(err)
	}
	var out struct {
		Providers []struct {
			ID    string `json:"id"`
			OK    bool   `json:"ok"`
			Usage *struct {
				Meters []struct {
					ID      string   `json:"id"`
					Percent *float64 `json:"percent"`
				} `json:"meters"`
				Warning *struct{ Kind string } `json:"warning"`
			} `json:"usage"`
			Error *struct {
				Kind string `json:"kind"`
				Hint string `json:"hint"`
			} `json:"error"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(b.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, b.String())
	}
	if len(out.Providers) != 3 || !out.Providers[0].OK || *out.Providers[0].Usage.Meters[0].Percent != 22 ||
		out.Providers[0].Usage.Warning.Kind != "auth" || out.Providers[2].OK || out.Providers[2].Error.Kind != "not_configured" {
		t.Errorf("unexpected JSON:\n%s", b.String())
	}
}

func TestTemplate(t *testing.T) {
	var b bytes.Buffer
	tmpl := `{{range .Providers}}{{.Name}}:{{if .OK}}{{range .Headline}} {{pct .}}({{reset .}}){{end}}{{else}} {{.Error}}{{end}};{{end}}`
	if err := Template(&b, tmpl, results(), opts()); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != "Codex: 22%(2h14m) 56.4%();Kiro: 41.6%();Cursor: no Cursor login found;\n" {
		t.Errorf("template = %q", got)
	}
	if err := Template(&b, "{{.Nope", results(), opts()); err == nil {
		t.Error("expected parse error")
	}
	b.Reset()
	if err := Template(&b, `{{range .Providers}}{{range .Headline}}[{{bar . 10}}]{{end}}{{end}}`, results()[1:2], opts()); err != nil {
		t.Fatal(err)
	}
	if lipgloss.Width(strings.TrimSpace(b.String())) != 12 {
		t.Errorf("bar = %q", b.String())
	}
}
