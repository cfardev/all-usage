package tui

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"github.com/cfardev/all-usage/internal/ui"
)

type keyMap struct {
	Refresh, Compact, Theme, Percent, Up, Down, Help, Quit key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		Refresh: key.NewBinding(key.WithKeys("r", "f5"), key.WithHelp("r", "refresh")),
		Compact: key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "compact")),
		Theme:   key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
		Percent: key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "used/left")),
		Up:      key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "scroll up")),
		Down:    key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "scroll down")),
		Help:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "more keys")),
		Quit:    key.NewBinding(key.WithKeys("q", "esc", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

// ShortHelp implements help.KeyMap.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Refresh, k.Compact, k.Theme, k.Percent, k.Help, k.Quit}
}

// FullHelp implements help.KeyMap.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Refresh, k.Compact, k.Theme, k.Percent}, {k.Up, k.Down, k.Help, k.Quit}}
}

// viewportKeys limits viewport scrolling to keys that don't clash with ours.
func viewportKeys() viewport.KeyMap {
	none := key.NewBinding(key.WithDisabled())
	return viewport.KeyMap{
		PageDown:     key.NewBinding(key.WithKeys("pgdown", " ")),
		PageUp:       key.NewBinding(key.WithKeys("pgup")),
		HalfPageDown: key.NewBinding(key.WithKeys("ctrl+d")),
		HalfPageUp:   key.NewBinding(key.WithKeys("ctrl+u")),
		Down:         key.NewBinding(key.WithKeys("down", "j")),
		Up:           key.NewBinding(key.WithKeys("up", "k")),
		Left:         none,
		Right:        none,
	}
}

type styles struct {
	app, title, plan, label, text, muted, warn, crit, accent lipgloss.Style
}

func newStyles(t ui.Theme) styles {
	s := styles{
		app:    lipgloss.NewStyle().Bold(true).Foreground(t.Accent),
		title:  lipgloss.NewStyle().Bold(true).Foreground(t.Text),
		plan:   lipgloss.NewStyle().Foreground(t.Accent),
		label:  lipgloss.NewStyle().Foreground(t.Text),
		text:   lipgloss.NewStyle().Foreground(t.Text),
		muted:  lipgloss.NewStyle().Foreground(t.Muted),
		warn:   lipgloss.NewStyle().Foreground(t.Warn),
		crit:   lipgloss.NewStyle().Foreground(t.Critical),
		accent: lipgloss.NewStyle().Foreground(t.Accent),
	}
	if t.Mono {
		s.muted = s.muted.Faint(true)
		s.plan = s.plan.Italic(true)
	}
	return s
}

func newHelp(t ui.Theme) help.Model {
	h := help.New()
	keyStyle := lipgloss.NewStyle().Foreground(t.Accent)
	descStyle := lipgloss.NewStyle().Foreground(t.Muted)
	sep := lipgloss.NewStyle().Foreground(t.Border)
	h.Styles.ShortKey, h.Styles.ShortDesc, h.Styles.ShortSeparator = keyStyle, descStyle, sep
	h.Styles.FullKey, h.Styles.FullDesc, h.Styles.FullSeparator = keyStyle, descStyle, sep
	h.Styles.Ellipsis = sep
	return h
}
