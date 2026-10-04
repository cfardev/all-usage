// Package tui implements the interactive dashboard: one card per provider,
// auto-refresh, live theme switching and a responsive grid layout.
package tui

import (
	"context"
	"slices"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cfardev/all-usage/internal/config"
	"github.com/cfardev/all-usage/internal/core"
	"github.com/cfardev/all-usage/internal/ui"
)

// card is the state of one provider.
type card struct {
	provider core.Provider
	color    string
	loading  bool
	usage    *core.Usage // last successful fetch
	err      *core.Error // last error (usage may still hold older data)
	errAt    time.Time
}

// Model is the bubbletea model of the dashboard.
type Model struct {
	cfg      *config.Config
	opts     ui.Options
	theme    ui.Theme
	st       styles
	cards    []card
	interval time.Duration
	timeout  time.Duration

	width, height int
	compact       bool
	keys          keyMap
	help          help.Model
	spinner       spinner.Model
	spinning      bool
	vp            viewport.Model

	now         time.Time
	nextRefresh time.Time
	lastDone    time.Time
	flash       string
	flashUntil  time.Time

	header, footer string
}

type tickMsg time.Time

type resultMsg struct {
	idx int
	res core.Result
}

// New builds the dashboard model.
func New(cfg *config.Config, ps []core.Provider) *Model {
	m := &Model{
		cfg:      cfg,
		interval: cfg.RefreshInterval.D(),
		timeout:  cfg.Timeout.D(),
		compact:  cfg.UI.Compact,
		keys:     newKeyMap(),
		now:      time.Now(),
		opts: ui.Options{
			Remaining:   cfg.UI.Percent == "remaining",
			ResetFormat: cfg.UI.ResetFormat,
			Clock:       cfg.UI.Clock,
			WarnAt:      cfg.UI.WarnAt,
			CriticalAt:  cfg.UI.CriticalAt,
			BarStyle:    cfg.UI.BarStyle,
		},
	}
	for _, p := range ps {
		m.cards = append(m.cards, card{provider: p, color: cfg.Common(p.ID()).Color})
	}
	m.vp = viewport.New(0, 0)
	m.vp.KeyMap = viewportKeys()
	m.vp.MouseWheelEnabled = false
	m.setTheme(cfg.UI.Theme)
	if len(cfg.Warnings) > 0 {
		m.flash = cfg.Warnings[0]
		m.flashUntil = m.now.Add(8 * time.Second)
	}
	return m
}

func (m *Model) setTheme(name string) {
	m.theme = ui.NewTheme(name, m.cfg.UI.Colors)
	m.st = newStyles(m.theme)
	m.help = newHelp(m.theme)
	m.help.Width = m.width
	m.spinner = spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(m.st.accent))
	m.spinning = false
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(tea.SetWindowTitle("all-usage"), m.refreshAll(), tick())
}

func (m *Model) anyLoading() bool {
	return slices.ContainsFunc(m.cards, func(c card) bool { return c.loading })
}

// refreshAll starts a fetch for every provider that is not already loading.
func (m *Model) refreshAll() tea.Cmd {
	var cmds []tea.Cmd
	for i := range m.cards {
		c := &m.cards[i]
		if c.loading {
			continue
		}
		c.loading = true
		p, timeout, idx := c.provider, m.timeout, i
		cmds = append(cmds, func() tea.Msg {
			return resultMsg{idx: idx, res: core.Fetch(context.Background(), p, timeout)}
		})
	}
	if m.interval > 0 {
		m.nextRefresh = m.now.Add(m.interval)
	}
	if len(cmds) > 0 && !m.spinning {
		m.spinning = true
		cmds = append(cmds, m.spinner.Tick)
	}
	return tea.Batch(cmds...)
}

func (m *Model) apply(r resultMsg) {
	if r.idx < 0 || r.idx >= len(m.cards) {
		return
	}
	c := &m.cards[r.idx]
	c.loading = false
	if r.res.OK() {
		c.usage, c.err = r.res.Usage, nil
	} else {
		c.err, c.errAt = r.res.Err, m.now
	}
	if !m.anyLoading() {
		m.lastDone = m.now
	}
}

func (m *Model) cycleTheme() {
	i := slices.Index(config.Themes, m.theme.Name)
	name := config.Themes[(i+1)%len(config.Themes)]
	m.setTheme(name)
	m.setFlash("theme: " + name + "  (set ui.theme = \"" + name + "\" to keep it)")
}

func (m *Model) setFlash(s string) {
	m.flash = s
	m.flashUntil = m.now.Add(4 * time.Second)
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.Width = msg.Width
	case tea.KeyMsg:
		// Fast typing (or key repeat) can deliver several runes in one
		// message ("jjj"); handle them one by one so bindings still match.
		keys := []tea.KeyMsg{msg}
		if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 && !msg.Paste {
			keys = keys[:0]
			for _, r := range msg.Runes {
				keys = append(keys, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: msg.Alt})
			}
		}
		for _, k := range keys {
			cmd, quit := m.handleKey(k)
			if quit {
				return m, tea.Quit
			}
			cmds = append(cmds, cmd)
		}
	case tickMsg:
		m.now = time.Time(msg)
		if m.interval > 0 && !m.nextRefresh.IsZero() && !m.now.Before(m.nextRefresh) {
			cmds = append(cmds, m.refreshAll())
		}
		cmds = append(cmds, tick())
	case resultMsg:
		m.now = time.Now()
		m.apply(msg)
	case spinner.TickMsg:
		if m.anyLoading() {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		} else {
			m.spinning = false
		}
	}
	m.layout()
	return m, tea.Batch(cmds...)
}

// handleKey applies one key press; quit reports whether to exit.
func (m *Model) handleKey(msg tea.KeyMsg) (cmd tea.Cmd, quit bool) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		return nil, true
	case key.Matches(msg, m.keys.Refresh):
		return m.refreshAll(), false
	case key.Matches(msg, m.keys.Compact):
		m.compact = !m.compact
	case key.Matches(msg, m.keys.Theme):
		m.cycleTheme()
		if m.anyLoading() {
			m.spinning = true
			return m.spinner.Tick, false
		}
	case key.Matches(msg, m.keys.Percent):
		m.opts.Remaining = !m.opts.Remaining
		if m.opts.Remaining {
			m.setFlash("showing remaining")
		} else {
			m.setFlash("showing used")
		}
	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
	default:
		m.vp, cmd = m.vp.Update(msg)
	}
	return cmd, false
}

// View implements tea.Model.
func (m *Model) View() string {
	if m.width == 0 {
		return ""
	}
	return m.header + "\n" + m.vp.View() + "\n" + m.footer
}
