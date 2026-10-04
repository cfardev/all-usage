package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/cfardev/all-usage/internal/core"
	"github.com/cfardev/all-usage/internal/ui"
)

const (
	gap          = 1  // columns between cards
	minCardWidth = 22 // narrowest card that still renders sensibly
)

// layout renders header, footer and body, sizing the viewport between them.
func (m *Model) layout() {
	if m.width == 0 {
		return
	}
	m.header = m.renderHeader()
	body := m.renderBody()
	m.footer = m.renderFooter(false)
	h := m.height - lipgloss.Height(m.header) - lipgloss.Height(m.footer)
	if lipgloss.Height(body) > h { // needs scrolling: show the scroll indicator
		m.footer = m.renderFooter(true)
		h = m.height - lipgloss.Height(m.header) - lipgloss.Height(m.footer)
	}
	m.vp.Width, m.vp.Height = m.width, max(1, h)
	m.vp.SetContent(body)
}

// spread places left and right on one line of width w, truncating as needed.
func spread(left, right string, w int) string {
	if w <= 0 {
		return ""
	}
	if right == "" {
		return ansi.Truncate(left, w, "…")
	}
	lw, rw := lipgloss.Width(left), lipgloss.Width(right)
	if rw >= w {
		return ansi.Truncate(right, w, "…")
	}
	if lw+1+rw > w {
		left = ansi.Truncate(left, w-rw-1, "…")
		lw = lipgloss.Width(left)
	}
	return left + strings.Repeat(" ", max(1, w-lw-rw)) + right
}

func (m *Model) renderHeader() string {
	w := max(1, m.width-2)
	left := m.st.app.Render("◆ all-usage")
	var status string
	switch {
	case m.anyLoading():
		status = m.spinner.View() + m.st.muted.Render(" refreshing")
	case m.interval > 0 && !m.nextRefresh.IsZero():
		status = m.st.muted.Render("↻ " + ui.FormatDuration(max(0, m.nextRefresh.Sub(m.now))))
	default:
		status = m.st.muted.Render("auto-refresh off")
	}
	right := status
	if !m.lastDone.IsZero() && !m.anyLoading() {
		withTime := status + m.st.muted.Render(" · updated "+m.opts.FormatClock(m.lastDone, m.now))
		if lipgloss.Width(left)+1+lipgloss.Width(withTime) <= w {
			right = withTime
		}
	}
	return " " + spread(left, right, w) + "\n"
}

func (m *Model) renderFooter(scrollable bool) string {
	w := max(1, m.width-1)
	var lines []string
	if m.flash != "" && m.now.Before(m.flashUntil) {
		lines = append(lines, m.st.warn.Render(ansi.Truncate(m.flash, w, "…")))
	}
	indicator := ""
	if scrollable {
		indicator = m.st.muted.Render(fmt.Sprintf("↕ %d%%", int(m.vp.ScrollPercent()*100)))
	}
	if m.cfg.UI.ShowHelp || m.help.ShowAll {
		m.help.Width = w
		if indicator != "" && !m.help.ShowAll {
			m.help.Width = max(1, w-lipgloss.Width(indicator)-2)
		}
		hl := strings.Split(m.help.View(m.keys), "\n")
		if indicator != "" && !m.help.ShowAll {
			hl[0] = spread(hl[0], indicator, w)
		}
		lines = append(lines, hl...)
	} else if scrollable {
		lines = append(lines, indicator+m.st.muted.Render(" j/k to scroll"))
	}
	for i, l := range lines {
		// Safety net: bubbles' help can overflow narrow terminals.
		lines[i] = " " + ansi.Truncate(l, w, "")
	}
	return strings.Join(lines, "\n")
}

func (m *Model) columns() int {
	n := len(m.cards)
	u := m.cfg.UI
	switch {
	case u.Layout == "rows":
		return 1
	case u.Layout == "columns":
		return n
	case u.Columns > 0:
		return min(u.Columns, n)
	}
	cols := (m.width + gap) / (u.CardWidth + gap)
	return max(1, min(cols, n))
}

func (m *Model) renderBody() string {
	if len(m.cards) == 0 {
		return "\n " + m.st.muted.Render("No providers enabled. Run `all-usage config edit` to enable some.")
	}
	cols := m.columns()
	cardW := max(minCardWidth, (m.width-gap*(cols-1))/cols)
	if m.cfg.UI.Layout == "auto" && m.cfg.UI.Columns == 0 {
		cardW = min(cardW, max(64, m.cfg.UI.CardWidth))
	}
	var rows []string
	for start := 0; start < len(m.cards); start += cols {
		end := min(start+cols, len(m.cards))
		contents := make([]string, 0, end-start)
		height := 0
		for i := start; i < end; i++ {
			c := m.cardContent(&m.cards[i], cardW-4)
			contents = append(contents, c)
			height = max(height, lipgloss.Height(c))
		}
		rendered := make([]string, 0, len(contents))
		for j, c := range contents {
			if j > 0 {
				rendered = append(rendered, strings.Repeat(" ", gap))
			}
			rendered = append(rendered, m.frame(&m.cards[start+j], cardW).Height(height).Render(c))
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, rendered...))
	}
	return lipgloss.PlaceHorizontal(m.width, lipgloss.Center, lipgloss.JoinVertical(lipgloss.Left, rows...))
}

// frame is the bordered card style; the border color reflects the worst state.
func (m *Model) frame(c *card, width int) lipgloss.Style {
	border := m.theme.Border
	switch {
	case c.usage == nil && c.err != nil && c.err.Kind != core.KindNotConfigured:
		border = m.theme.Critical
	case c.usage != nil:
		worst := -1.0
		for _, mt := range c.usage.Meters {
			if mt.HasPercent() && mt.Pct() > worst {
				worst = mt.Pct()
			}
		}
		if worst >= 0 && m.opts.LevelFor(worst) != ui.LevelOK {
			border = m.theme.LevelColor(m.opts.LevelFor(worst))
		}
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).Padding(0, 1).Width(width - 2)
}

func (m *Model) wrap(s lipgloss.Style, text string, w int) string {
	return s.Width(w).Render(text)
}

func (m *Model) cardContent(c *card, w int) string {
	st := m.st
	dot := lipgloss.NewStyle().Foreground(m.theme.ProviderColor(c.color)).Render("●")
	left := dot + " " + st.title.Render(c.provider.Name())
	if c.loading {
		left += " " + m.spinner.View()
	}
	plan := ""
	if c.usage != nil && c.usage.Plan != "" {
		plan = st.plan.Render(c.usage.Plan)
	}
	lines := []string{spread(left, plan, w)}
	if c.usage != nil && c.usage.Account != "" {
		lines = append(lines, st.muted.Render(ansi.Truncate(c.usage.Account, w, "…")))
	}

	switch {
	case c.usage == nil && c.err == nil:
		lines = append(lines, "", st.muted.Render("Fetching usage…"))
	case c.usage == nil:
		lines = append(lines, "")
		lines = append(lines, m.errorLines(c.err, w)...)
	default:
		u := c.usage
		if !m.compact {
			lines = append(lines, "")
		}
		if len(u.Meters) == 0 {
			lines = append(lines, st.muted.Render("No usage data"))
		}
		labelCap := max(6, w*2/5)
		labelW := 0
		for _, mt := range u.Meters {
			labelW = max(labelW, lipgloss.Width(compactLabel(mt, labelCap)))
		}
		for i, mt := range u.Meters {
			if i > 0 && !m.compact {
				lines = append(lines, "")
			}
			if m.compact {
				lines = append(lines, m.compactMeter(mt, w, labelW, labelCap))
			} else {
				lines = append(lines, m.meterLines(mt, w)...)
			}
		}
		for _, n := range u.Notes {
			lines = append(lines, "", m.wrap(st.warn, "! "+n, w))
		}
		if u.Warning != nil {
			lines = append(lines, "", m.wrap(st.warn, "⚠ Live data unavailable: "+u.Warning.Msg, w))
			if u.Warning.Hint != "" {
				lines = append(lines, m.wrap(st.muted, "→ "+u.Warning.Hint, w))
			}
		}
		if c.err != nil {
			lines = append(lines, "", m.wrap(st.warn, "⚠ Refresh failed: "+c.err.Msg, w))
		}
	}
	if src := m.sourceLine(c, w); src != "" {
		lines = append(lines, "", src)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) sourceLine(c *card, w int) string {
	if !m.cfg.UI.ShowSource || c.usage == nil {
		return ""
	}
	u := c.usage
	if u.AsOf != nil {
		return m.st.warn.Render(ansi.Truncate(u.Source+" · as of "+ui.FormatAgo(*u.AsOf, m.now), w, "…"))
	}
	return m.st.muted.Render(ansi.Truncate(u.Source+" · "+ui.FormatAgo(u.FetchedAt, m.now), w, "…"))
}

func (m *Model) errorLines(e *core.Error, w int) []string {
	style := m.st.crit
	icon := "✗ "
	if e.Kind == core.KindNotConfigured {
		style, icon = m.st.warn, "○ "
	}
	out := []string{style.Bold(true).Render(icon + e.Title()), m.wrap(m.st.text, e.Msg, w)}
	if e.Hint != "" {
		out = append(out, m.wrap(m.st.muted, "→ "+e.Hint, w))
	}
	return out
}

func (m *Model) resetText(mt core.Meter) string {
	if mt.ResetsAt == nil {
		return ""
	}
	if !mt.ResetsAt.After(m.now) {
		return "resetting now"
	}
	return "resets " + m.opts.FormatReset(*mt.ResetsAt, m.now)
}

func (m *Model) meterLines(mt core.Meter, w int) []string {
	st := m.st
	if !mt.HasPercent() {
		out := []string{spread(st.label.Render(mt.Label), st.title.Render(mt.Value), w)}
		if mt.Detail != "" {
			out = append(out, m.wrap(st.muted, mt.Detail, w))
		}
		return out
	}
	used := mt.Pct()
	color := m.theme.LevelColor(m.opts.LevelFor(used))
	pct := lipgloss.NewStyle().Foreground(color).Bold(true).Render(m.opts.PctText(used))
	out := []string{
		spread(st.label.Render(mt.Label), pct, w),
		m.theme.Bar(m.opts.DisplayPct(used), w, m.opts.BarStyle, color),
	}
	var info []string
	if s := ui.UsedOfLimit(mt); s != "" {
		info = append(info, s)
	}
	if s := m.resetText(mt); s != "" {
		info = append(info, s)
	}
	if joined := strings.Join(info, " · "); lipgloss.Width(joined) <= w {
		if joined != "" {
			out = append(out, st.muted.Render(joined))
		}
	} else {
		for _, s := range info {
			out = append(out, st.muted.Render(ansi.Truncate(s, w, "…")))
		}
	}
	if mt.Detail != "" {
		out = append(out, m.wrap(st.muted, mt.Detail, w))
	}
	return out
}

// compactLabel picks the label shown in compact mode: the full label when it
// fits in limit cells, otherwise the short alias, otherwise a truncation.
func compactLabel(mt core.Meter, limit int) string {
	switch {
	case lipgloss.Width(mt.Label) <= limit:
		return mt.Label
	case mt.Short != "" && lipgloss.Width(mt.Short) <= limit:
		return mt.Short
	default:
		return ansi.Truncate(mt.Label, limit, "…")
	}
}

// compactMeter renders one line: label, bar, percentage and time to reset.
func (m *Model) compactMeter(mt core.Meter, w, labelW, labelCap int) string {
	st := m.st
	label := compactLabel(mt, labelCap)
	label += strings.Repeat(" ", max(0, labelW-lipgloss.Width(label)))
	if !mt.HasPercent() {
		return spread(st.label.Render(label), st.title.Render(mt.Value), w)
	}
	used := mt.Pct()
	color := m.theme.LevelColor(m.opts.LevelFor(used))
	pctTxt := m.opts.PctText(used)
	pctW := 5
	if m.opts.Remaining {
		pctW = 10
	}
	pct := lipgloss.NewStyle().Foreground(color).Bold(true).Render(fmt.Sprintf("%*s", pctW, pctTxt))
	reset := ""
	if mt.ResetsAt != nil {
		reset = m.opts.ShortReset(*mt.ResetsAt, m.now)
	}
	resetW := 0
	if reset != "" {
		resetW = max(6, lipgloss.Width(reset))
	}
	barW := w - labelW - 1 - pctW - 1
	if resetW > 0 {
		barW -= resetW + 1
	}
	if barW < 4 && resetW > 0 { // drop the reset time before the bar
		barW += resetW + 1
		reset, resetW = "", 0
	}
	parts := []string{st.label.Render(label)}
	if barW >= 4 {
		parts = append(parts, m.theme.Bar(m.opts.DisplayPct(used), barW, m.opts.BarStyle, color))
	}
	parts = append(parts, pct)
	if reset != "" {
		parts = append(parts, st.muted.Render(fmt.Sprintf("%-*s", resetW, reset)))
	}
	return strings.Join(parts, " ")
}
