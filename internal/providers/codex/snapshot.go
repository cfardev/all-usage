package codex

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/cfardev/all-usage/internal/core"
	"github.com/cfardev/all-usage/internal/ui"
)

// snapshot is the normalized form of Codex rate limits, whatever the source.
type snapshot struct {
	Plan    string
	Groups  []limitGroup // Groups[0] is the main "codex" limit
	Credits *credits
	Reached string
	Source  string
	Email   string
	// AsOf is set for data read from session logs (not live).
	AsOf time.Time
}

type limitGroup struct {
	ID        string
	Name      string
	Primary   *window
	Secondary *window
}

type window struct {
	UsedPercent float64
	Length      time.Duration
	ResetsAt    time.Time
}

type credits struct {
	HasCredits bool
	Unlimited  bool
	Balance    string
}

func (s *snapshot) hasData() bool {
	for _, g := range s.Groups {
		if g.Primary != nil || g.Secondary != nil {
			return true
		}
	}
	return s.Credits != nil && (s.Credits.Unlimited || s.Credits.HasCredits)
}

// toUsage converts the snapshot into dashboard meters.
func (s *snapshot) toUsage(id, name string, now time.Time, showAccount bool) *core.Usage {
	u := &core.Usage{
		Provider:  id,
		Name:      name,
		Plan:      planName(s.Plan),
		Source:    s.Source,
		FetchedAt: now,
		Meters:    []core.Meter{},
	}
	if showAccount {
		u.Account = s.Email
	}
	stale := !s.AsOf.IsZero()
	if stale {
		asOf := s.AsOf
		u.AsOf = &asOf
		u.Stale = true
	}
	for gi, g := range s.Groups {
		main := gi == 0
		for wi, w := range []*window{g.Primary, g.Secondary} {
			if w == nil {
				continue
			}
			slot := "primary"
			if wi == 1 {
				slot = "secondary"
			}
			label, short := ui.WindowLabel(w.Length)
			m := core.Meter{
				ID:            slot,
				Label:         label + " limit",
				Short:         short,
				Headline:      main,
				WindowSeconds: int64(w.Length / time.Second),
			}
			if !main {
				m.ID = g.ID + "." + slot
				m.Label = g.Name + " · " + strings.ToLower(label)
				m.Short = ""
			}
			pct, reset := w.UsedPercent, w.ResetsAt
			if stale && !reset.IsZero() && !reset.After(now) {
				// The window rolled over after the snapshot was recorded.
				pct, reset = 0, time.Time{}
				m.Detail = "reset since last session"
			}
			m.Percent = core.F64(pct)
			m.ResetsAt = core.TimePtr(reset)
			u.Meters = append(u.Meters, m)
		}
	}
	if c := s.Credits; c != nil {
		switch {
		case c.Unlimited:
			u.Meters = append(u.Meters, core.Meter{ID: "credits", Label: "Credits", Value: "unlimited"})
		case c.HasCredits || (c.Balance != "" && c.Balance != "0"):
			v := c.Balance
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				v = ui.FormatNumber(f)
			}
			if v == "" {
				v = "available"
			}
			u.Meters = append(u.Meters, core.Meter{ID: "credits", Label: "Credits", Value: v})
		}
	}
	if s.Reached != "" {
		u.Notes = append(u.Notes, reachedText(s.Reached))
	}
	return u
}

var planNames = map[string]string{
	"free": "Free", "go": "Go", "plus": "Plus", "pro": "Pro", "prolite": "Pro Lite", "promax": "Pro Max",
	"team": "Team", "business": "Business", "enterprise": "Enterprise", "edu": "Edu", "education": "Edu",
	"self_serve_business_usage_based": "Business (usage-based)",
	"enterprise_cbp_usage_based":      "Enterprise (usage-based)",
	"free_workspace":                  "Free workspace",
	"guest":                           "Guest",
	"unknown":                         "",
}

func planName(p string) string {
	p = strings.ToLower(strings.TrimSpace(p))
	if n, ok := planNames[p]; ok {
		return n
	}
	return ui.TitleCase(p)
}

func reachedText(kind string) string {
	switch kind {
	case "rate_limit_reached":
		return "Rate limit reached"
	case "workspace_owner_credits_depleted", "workspace_member_credits_depleted":
		return "Workspace credits depleted"
	case "workspace_owner_usage_limit_reached", "workspace_member_usage_limit_reached":
		return "Workspace usage limit reached"
	}
	s := strings.ReplaceAll(kind, "_", " ")
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// parseReached accepts null, "kind" or {"type": "kind"}.
func parseReached(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var obj struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(raw, &obj) == nil && obj.Type != "unknown" {
		return obj.Type
	}
	return ""
}
