package claude

import (
	"context"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/cfardev/all-usage/internal/core"
	"github.com/cfardev/all-usage/internal/httpx"
	"github.com/cfardev/all-usage/internal/jsonx"
	"github.com/cfardev/all-usage/internal/ui"
)

const loginHint = "Run `claude` (Claude Code) to refresh your login, or /login to sign in again."

// usagePayload is GET {api}/api/oauth/usage (the endpoint behind Claude Code's /usage).
type usagePayload struct {
	FiveHour       *window `json:"five_hour"`
	SevenDay       *window `json:"seven_day"`
	SevenDayOpus   *window `json:"seven_day_opus"`
	SevenDaySonnet *window `json:"seven_day_sonnet"`
	ExtraUsage     *struct {
		IsEnabled     bool         `json:"is_enabled"`
		MonthlyLimit  jsonx.Number `json:"monthly_limit"`
		UsedCredits   jsonx.Number `json:"used_credits"`
		Utilization   jsonx.Number `json:"utilization"`
		Currency      string       `json:"currency"`
		DecimalPlaces jsonx.Number `json:"decimal_places"`
	} `json:"extra_usage"`
}

type window struct {
	Utilization jsonx.Number `json:"utilization"`
	ResetsAt    jsonx.String `json:"resets_at"`
}

func (w *window) meter(id, label, short string, length time.Duration, headline bool) (core.Meter, bool) {
	if w == nil || !w.Utilization.Valid {
		return core.Meter{}, false
	}
	m := core.Meter{ID: id, Label: label, Short: short, Headline: headline,
		Percent: core.F64(w.Utilization.V), WindowSeconds: int64(length / time.Second)}
	if t, ok := jsonx.ParseTime(w.ResetsAt.V); ok {
		m.ResetsAt = core.TimePtr(t)
	}
	return m, true
}

func hostOf(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	return raw
}

// fetchWith calls the usage endpoint with one credential.
func (p *Provider) fetchWith(ctx context.Context, c *credential) (*core.Usage, error) {
	base := strings.TrimRight(p.cfg.APIBase, "/")
	host := hostOf(base)
	resp, err := p.env.HTTP.Do(ctx, httpx.Request{URL: base + "/api/oauth/usage", Headers: map[string]string{
		"Authorization":  "Bearer " + c.AccessToken,
		"anthropic-beta": "oauth-2025-04-20",
	}})
	switch {
	case err != nil:
		return nil, core.NetworkError(err, host)
	case resp.Status == 401 || resp.Status == 403:
		return nil, core.NewError(core.KindAuth, loginHint, "Claude rejected the login").At(c.label())
	case !resp.OK():
		return nil, core.HTTPError(resp.Status, resp.Body, host)
	}
	var pl usagePayload
	if err := resp.Decode(&pl); err != nil {
		return nil, core.Wrap(core.KindParse, err, "", "unexpected response from %s", host)
	}
	u := fromPayload(&pl, p.env.Time())
	if len(u.Meters) == 0 {
		return nil, core.NewError(core.KindParse, "", "Claude returned no usage data")
	}
	u.Plan = c.plan()
	u.Source = c.Source + " login"
	if c.Source == srcEnv {
		u.Source = "env token"
	}
	return u, nil
}

func fromPayload(pl *usagePayload, now time.Time) *core.Usage {
	u := &core.Usage{FetchedAt: now, Meters: []core.Meter{}}
	add := func(m core.Meter, ok bool) {
		if ok {
			u.Meters = append(u.Meters, m)
		}
	}
	const week = 7 * 24 * time.Hour
	add(pl.FiveHour.meter("five_hour", "5-hour limit", "5h", 5*time.Hour, true))
	add(pl.SevenDay.meter("seven_day", "Weekly limit", "wk", week, true))
	add(pl.SevenDayOpus.meter("seven_day_opus", "Weekly Opus", "opus", week, false))
	add(pl.SevenDaySonnet.meter("seven_day_sonnet", "Weekly Sonnet", "sonnet", week, false))
	if x := pl.ExtraUsage; x != nil && x.IsEnabled {
		// Amounts are in minor units (cents unless decimal_places says otherwise).
		scale := math.Pow10(int(x.DecimalPlaces.Or(2)))
		currency := x.Currency
		if currency == "" {
			currency = "USD"
		}
		m := core.Meter{ID: "extra_usage", Label: "Extra usage", Unit: strings.ToLower(currency)}
		used := x.UsedCredits.Or(0) / scale
		if limit := x.MonthlyLimit.Or(0) / scale; limit > 0 {
			m.Used, m.Limit = core.F64(used), core.F64(limit)
			m.Percent = core.F64(x.Utilization.Or(used / limit * 100))
		} else {
			m.Value = ui.FormatMoney(used, currency) + " (no limit)"
		}
		u.Meters = append(u.Meters, m)
	}
	return u
}
