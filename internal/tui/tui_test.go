package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/cfardev/all-usage/internal/config"
	"github.com/cfardev/all-usage/internal/core"
)

type stub struct {
	id, name string
	usage    *core.Usage
	err      error
}

func (s stub) ID() string   { return s.id }
func (s stub) Name() string { return s.name }
func (s stub) Fetch(context.Context) (*core.Usage, error) {
	return s.usage, s.err
}
func (s stub) Doctor(context.Context) []core.Check { return nil }

func providers() []core.Provider {
	now := time.Now()
	reset := now.Add(3*time.Hour + 5*time.Minute + 30*time.Second)
	asOf := now.Add(-5 * time.Hour)
	return []core.Provider{
		stub{id: "codex", name: "Codex", usage: &core.Usage{Provider: "codex", Name: "Codex", Plan: "Team", Source: "session log",
			AsOf: &asOf, Stale: true, FetchedAt: now,
			Warning: &core.Error{Kind: core.KindAuth, Msg: "Codex login expired 1d 16h ago (/mnt/c/Users/someone/.codex/auth.json)", Hint: "Run `codex login` (or open Codex) to sign in again."},
			Meters: []core.Meter{
				{ID: "primary", Label: "5-hour limit", Short: "5h", Headline: true, Percent: core.F64(95), ResetsAt: &reset},
				{ID: "secondary", Label: "Weekly limit", Short: "wk", Headline: true, Percent: core.F64(56), ResetsAt: &reset},
				{ID: "credits", Label: "Credits", Value: "unlimited"},
			}}},
		stub{id: "kiro", name: "Kiro", usage: &core.Usage{Provider: "kiro", Name: "Kiro", Plan: "Kiro Pro", Source: "kiro-cli login", FetchedAt: now,
			Meters: []core.Meter{{ID: "credit", Label: "Credits", Headline: true, Percent: core.F64(52.96), Used: core.F64(529.61),
				Limit: core.F64(1000), Unit: "credits", ResetsAt: &reset, Detail: "some extra detail that is long enough to wrap around"}}}},
		stub{id: "cursor", name: "Cursor", err: core.NewError(core.KindNotConfigured, "Sign in to the Cursor app, or run `cursor-agent login`.", "no Cursor login found")},
	}
}

func newModel(t *testing.T, mutate func(*config.Config)) *Model {
	t.Helper()
	lipgloss.SetColorProfile(termenv.TrueColor)
	cfg := config.Default()
	if mutate != nil {
		mutate(cfg)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	return New(cfg, providers())
}

// load runs the initial fetches synchronously.
func load(m *Model) {
	cmd := m.refreshAll()
	for _, msg := range collect(cmd) {
		m.Update(msg)
	}
}

// collect executes a command tree and returns the resulting result messages.
func collect(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	switch msg := msg.(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, collect(c)...)
		}
		return out
	case resultMsg:
		return []tea.Msg{msg}
	}
	return nil
}

func checkFits(t *testing.T, m *Model, w, h int) string {
	t.Helper()
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) > h {
		t.Errorf("%dx%d: view has %d lines", w, h, len(lines))
	}
	for i, l := range lines {
		if lw := lipgloss.Width(l); lw > w {
			t.Errorf("%dx%d: line %d is %d cells wide: %q", w, h, i, lw, l)
		}
	}
	return view
}

func TestViewFitsTerminal(t *testing.T) {
	sizes := [][2]int{{20, 8}, {40, 15}, {60, 24}, {80, 24}, {100, 30}, {120, 40}, {160, 50}, {250, 60}}
	variants := map[string]func(*config.Config){
		"default":   nil,
		"compact":   func(c *config.Config) { c.UI.Compact = true },
		"rows":      func(c *config.Config) { c.UI.Layout = "rows" },
		"columns":   func(c *config.Config) { c.UI.Layout = "columns" },
		"remaining": func(c *config.Config) { c.UI.Percent = "remaining"; c.UI.ResetFormat = "both"; c.UI.Clock = "12h" },
		"nohelp":    func(c *config.Config) { c.UI.ShowHelp = false; c.UI.ShowSource = false; c.UI.BarStyle = "dots" },
		"3cols":     func(c *config.Config) { c.UI.Columns = 3; c.UI.Theme = "mono" },
	}
	for name, mutate := range variants {
		t.Run(name, func(t *testing.T) {
			m := newModel(t, mutate)
			load(m)
			for _, s := range sizes {
				checkFits(t, m, s[0], s[1])
			}
		})
	}
}

func TestViewContent(t *testing.T) {
	m := newModel(t, nil)
	load(m)
	view := checkFits(t, m, 140, 50)
	for _, want := range []string{"all-usage", "Codex", "Team", "5-hour limit", "95%", "resets in 3h 5m", "unlimited",
		"Kiro Pro", "529.61 / 1,000 credits", "Not signed in", "cursor-agent login", "Live data unavailable", "as of 5h ago"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q", want)
		}
	}
	// Three cards side by side at this width.
	if !strings.Contains(firstLineWith(view, "Codex"), "Kiro") {
		t.Errorf("expected cards in one row:\n%s", view)
	}
}

func firstLineWith(view, s string) string {
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(l, s) {
			return l
		}
	}
	return ""
}

func TestKeys(t *testing.T) {
	m := newModel(t, nil)
	load(m)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	if !m.opts.Remaining || !strings.Contains(m.View(), "5% left") {
		t.Error("u should toggle remaining")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if !m.compact {
		t.Error("c should toggle compact")
	}
	before := m.theme.Name
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	if m.theme.Name == before {
		t.Error("t should cycle the theme")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if !m.anyLoading() || cmd == nil {
		t.Error("r should start a refresh")
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil || cmd() != tea.Quit() {
		t.Error("q should quit")
	}
}

func TestAutoRefreshAndKeepsDataOnError(t *testing.T) {
	m := newModel(t, func(c *config.Config) { c.RefreshInterval = config.Duration(10 * time.Second) })
	load(m)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.anyLoading() {
		t.Fatal("should be idle after load")
	}
	// A later failure keeps the previous data and shows the error.
	m.Update(resultMsg{idx: 1, res: core.Result{ID: "kiro", Name: "Kiro", Err: core.NewError(core.KindNetwork, "", "cannot reach q.us-east-1.amazonaws.com")}})
	if m.cards[1].usage == nil || !strings.Contains(m.View(), "Refresh failed") {
		t.Error("previous data must be kept on refresh errors")
	}
	m.Update(tickMsg(m.nextRefresh.Add(time.Second)))
	if !m.anyLoading() {
		t.Error("tick past nextRefresh should start a refresh")
	}
}

func TestScrollInSmallTerminal(t *testing.T) {
	m := newModel(t, nil)
	load(m)
	m.Update(tea.WindowSizeMsg{Width: 58, Height: 20})
	top := m.View()
	for range 5 {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	}
	if m.vp.YOffset != 5 || m.View() == top {
		t.Fatalf("j should scroll down: offset=%d", m.vp.YOffset)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.vp.YOffset != 4 {
		t.Fatalf("up should scroll up: offset=%d", m.vp.YOffset)
	}
	// Re-rendering on ticks must keep the scroll position.
	m.Update(tickMsg(time.Now()))
	if m.vp.YOffset != 4 {
		t.Fatalf("tick reset the scroll position: offset=%d", m.vp.YOffset)
	}
	// Fast typing arrives as one multi-rune message.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("jjj")})
	if m.vp.YOffset != 7 {
		t.Fatalf("coalesced keys should scroll 3 lines: offset=%d", m.vp.YOffset)
	}
}

func TestNoProviders(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii)
	m := New(config.Default(), nil)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	if !strings.Contains(m.View(), "No providers enabled") {
		t.Error("expected empty-state message")
	}
}
