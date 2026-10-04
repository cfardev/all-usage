package cursor

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/cfardev/all-usage/internal/core"
	"github.com/cfardev/all-usage/internal/httpx"
	"github.com/cfardev/all-usage/internal/jsonx"
	"github.com/cfardev/all-usage/internal/ui"
)

// usageSummary is GET {web}/api/usage-summary (amounts in cents).
type usageSummary struct {
	BillingCycleEnd jsonx.String `json:"billingCycleEnd"`
	MembershipType  string       `json:"membershipType"`
	IsUnlimited     bool         `json:"isUnlimited"`
	IndividualUsage *struct {
		Plan     *planUsage `json:"plan"`
		OnDemand *onDemand  `json:"onDemand"`
	} `json:"individualUsage"`
	TeamUsage *struct {
		OnDemand *onDemand `json:"onDemand"`
	} `json:"teamUsage"`
}

type planUsage struct {
	Enabled   *bool        `json:"enabled"`
	Used      jsonx.Number `json:"used"`
	Limit     jsonx.Number `json:"limit"`
	Breakdown *struct {
		Included jsonx.Number `json:"included"`
		Bonus    jsonx.Number `json:"bonus"`
		Total    jsonx.Number `json:"total"`
	} `json:"breakdown"`
	AutoPercentUsed  jsonx.Number `json:"autoPercentUsed"`
	APIPercentUsed   jsonx.Number `json:"apiPercentUsed"`
	TotalPercentUsed jsonx.Number `json:"totalPercentUsed"`
}

type onDemand struct {
	Enabled bool         `json:"enabled"`
	Used    jsonx.Number `json:"used"`
	Limit   jsonx.Number `json:"limit"`
}

// periodUsage is DashboardService/GetCurrentPeriodUsage (fallback; cents).
type periodUsage struct {
	BillingCycleEnd jsonx.String `json:"billingCycleEnd"`
	PlanUsage       *struct {
		TotalSpend       jsonx.Number `json:"totalSpend"`
		IncludedSpend    jsonx.Number `json:"includedSpend"`
		BonusSpend       jsonx.Number `json:"bonusSpend"`
		Limit            jsonx.Number `json:"limit"`
		AutoPercentUsed  jsonx.Number `json:"autoPercentUsed"`
		APIPercentUsed   jsonx.Number `json:"apiPercentUsed"`
		TotalPercentUsed jsonx.Number `json:"totalPercentUsed"`
	} `json:"planUsage"`
	SpendLimitUsage *struct {
		IndividualLimit jsonx.Number `json:"individualLimit"`
		IndividualUsed  jsonx.Number `json:"individualUsed"`
	} `json:"spendLimitUsage"`
}

// planInfo is DashboardService/GetPlanInfo.
type planInfo struct {
	PlanInfo *struct {
		PlanName string `json:"planName"`
		Price    string `json:"price"`
	} `json:"planInfo"`
}

// legacyUsage is GET {api}/auth/usage (request-based plans).
type legacyUsage struct {
	GPT4 *struct {
		NumRequests     jsonx.Number `json:"numRequests"`
		MaxRequestUsage jsonx.Number `json:"maxRequestUsage"`
	} `json:"gpt-4"`
	StartOfMonth jsonx.String `json:"startOfMonth"`
}

const loginHint = "Open Cursor and sign in again, or run `cursor-agent login`."

type call struct {
	resp *httpx.Response
	err  error
}

func (p *Provider) do(ctx context.Context, t *token, method, rawURL string, cookie bool) call {
	h := map[string]string{}
	var body any
	if cookie {
		h["Cookie"] = "WorkosCursorSessionToken=" + t.userID() + "%3A%3A" + t.Value
		h["Origin"] = strings.TrimRight(p.cfg.WebBase, "/")
		h["Referer"] = strings.TrimRight(p.cfg.WebBase, "/") + "/dashboard"
	} else {
		h["Authorization"] = "Bearer " + t.Value
	}
	if method == http.MethodPost {
		body = map[string]any{}
	}
	resp, err := p.env.HTTP.Do(ctx, httpx.Request{Method: method, URL: rawURL, Headers: h, JSON: body})
	return call{resp: resp, err: err}
}

func hostOf(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	return raw
}

// check converts a failed call into a provider error (nil when OK).
func check(c call, host string, t *token) error {
	switch {
	case c.err != nil:
		return core.NetworkError(c.err, host)
	case c.resp.Status == 401 || c.resp.Status == 403:
		return core.NewError(core.KindAuth, loginHint, "Cursor rejected the login").At(t.label())
	case !c.resp.OK():
		return core.HTTPError(c.resp.Status, c.resp.Body, host)
	}
	return nil
}

// fetchWith queries the usage endpoints concurrently with one token.
func (p *Provider) fetchWith(ctx context.Context, t *token) (*core.Usage, error) {
	api := strings.TrimRight(p.cfg.APIBase, "/")
	web := strings.TrimRight(p.cfg.WebBase, "/")
	var summary, plan, legacy call
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); summary = p.do(ctx, t, http.MethodGet, web+"/api/usage-summary", true) }()
	go func() {
		defer wg.Done()
		plan = p.do(ctx, t, http.MethodPost, api+"/aiserver.v1.DashboardService/GetPlanInfo", false)
	}()
	go func() { defer wg.Done(); legacy = p.do(ctx, t, http.MethodGet, api+"/auth/usage", false) }()
	wg.Wait()

	now := p.env.Time()
	var u *core.Usage
	if err := check(summary, hostOf(web), t); err == nil {
		var s usageSummary
		if derr := summary.resp.Decode(&s); derr != nil {
			return nil, core.Wrap(core.KindParse, derr, "", "unexpected response from %s", hostOf(web))
		}
		u = fromSummary(&s, now)
		u.Source = t.Source + " login"
	} else {
		// Fall back to the IDE's own RPC (bearer auth).
		pc := p.do(ctx, t, http.MethodPost, api+"/aiserver.v1.DashboardService/GetCurrentPeriodUsage", false)
		if perr := check(pc, hostOf(api), t); perr != nil {
			if core.IsAuth(perr) || !core.IsAuth(err) {
				return nil, perr
			}
			return nil, err
		}
		var pu periodUsage
		if derr := pc.resp.Decode(&pu); derr != nil {
			return nil, core.Wrap(core.KindParse, derr, "", "unexpected response from %s", hostOf(api))
		}
		u = fromPeriod(&pu, now)
		u.Source = t.Source + " login"
	}
	if t.Source == srcEnv {
		u.Source = "env token"
	}

	if check(plan, hostOf(api), t) == nil {
		var pi planInfo
		if plan.resp.Decode(&pi) == nil && pi.PlanInfo != nil && pi.PlanInfo.PlanName != "" {
			u.Plan = pi.PlanInfo.PlanName
		}
	}
	if u.Plan == "" {
		u.Plan = membershipName(t.Membership)
	}
	if check(legacy, hostOf(api), t) == nil {
		var l legacyUsage
		if legacy.resp.Decode(&l) == nil && l.GPT4 != nil && l.GPT4.MaxRequestUsage.Or(0) > 0 {
			used, limit := l.GPT4.NumRequests.Or(0), l.GPT4.MaxRequestUsage.V
			m := core.Meter{ID: "requests", Label: "Premium requests", Unit: "requests",
				Used: core.F64(used), Limit: core.F64(limit), Percent: core.F64(used / limit * 100)}
			if start, ok := jsonx.ParseTime(l.StartOfMonth.V); ok {
				m.ResetsAt = core.TimePtr(start.AddDate(0, 1, 0))
			}
			m.Headline = len(u.Meters) == 0
			u.Meters = append(u.Meters, m)
		}
	}
	if len(u.Meters) == 0 {
		return nil, core.NewError(core.KindParse, "", "Cursor returned no usage data")
	}
	if p.env.ShowAccount {
		u.Account = t.Email
	}
	return u, nil
}

func cents(n jsonx.Number) float64 { return n.Or(0) / 100 }

func spendDetail(total, bonus float64) string {
	if total <= 0 {
		return ""
	}
	s := ui.FormatMoney(total, "USD") + " spent"
	if bonus > 0 {
		s += " · incl. " + ui.FormatMoney(bonus, "USD") + " bonus"
	}
	return s
}

func percentMeters(total, auto, api jsonx.Number, reset time.Time) []core.Meter {
	var ms []core.Meter
	add := func(n jsonx.Number, id, label, short string, headline bool) {
		if n.Valid {
			ms = append(ms, core.Meter{ID: id, Label: label, Short: short, Percent: core.F64(n.V), Headline: headline, ResetsAt: core.TimePtr(reset)})
		}
	}
	add(total, "total", "Total usage", "Total", true)
	add(auto, "auto", "Auto + Composer", "Auto", false)
	add(api, "api", "API models", "API", false)
	return ms
}

func fromSummary(s *usageSummary, now time.Time) *core.Usage {
	u := &core.Usage{FetchedAt: now, Meters: []core.Meter{}, Plan: membershipName(s.MembershipType)}
	reset, _ := jsonx.ParseTime(s.BillingCycleEnd.V)
	if s.IndividualUsage != nil && s.IndividualUsage.Plan != nil {
		pl := s.IndividualUsage.Plan
		u.Meters = percentMeters(pl.TotalPercentUsed, pl.AutoPercentUsed, pl.APIPercentUsed, reset)
		if len(u.Meters) == 0 && pl.Limit.Or(0) > 0 {
			used, limit := cents(pl.Used), cents(pl.Limit)
			u.Meters = append(u.Meters, core.Meter{ID: "total", Label: "Included usage", Headline: true, Unit: "usd",
				Used: core.F64(used), Limit: core.F64(limit), Percent: core.F64(used / limit * 100), ResetsAt: core.TimePtr(reset)})
		}
		if len(u.Meters) > 0 && pl.Breakdown != nil {
			u.Meters[0].Detail = spendDetail(cents(pl.Breakdown.Total), cents(pl.Breakdown.Bonus))
		}
	}
	if s.IndividualUsage != nil {
		if m, ok := onDemandMeter(s.IndividualUsage.OnDemand, "on_demand", "On-demand", reset); ok {
			u.Meters = append(u.Meters, m)
		}
	}
	if s.TeamUsage != nil {
		if m, ok := onDemandMeter(s.TeamUsage.OnDemand, "team_on_demand", "Team on-demand", reset); ok {
			u.Meters = append(u.Meters, m)
		}
	}
	if s.IsUnlimited {
		u.Notes = append(u.Notes, "Unlimited plan")
	}
	return u
}

func onDemandMeter(od *onDemand, id, label string, reset time.Time) (core.Meter, bool) {
	if od == nil || !od.Enabled {
		return core.Meter{}, false
	}
	m := core.Meter{ID: id, Label: label, Unit: "usd", ResetsAt: core.TimePtr(reset)}
	used := cents(od.Used)
	if limit := cents(od.Limit); limit > 0 {
		m.Used, m.Limit, m.Percent = core.F64(used), core.F64(limit), core.F64(used/limit*100)
	} else {
		m.Value = ui.FormatMoney(used, "USD") + " (no limit)"
	}
	return m, true
}

func fromPeriod(pu *periodUsage, now time.Time) *core.Usage {
	u := &core.Usage{FetchedAt: now, Meters: []core.Meter{}}
	reset, _ := jsonx.ParseTime(pu.BillingCycleEnd.V)
	if pl := pu.PlanUsage; pl != nil {
		u.Meters = percentMeters(pl.TotalPercentUsed, pl.AutoPercentUsed, pl.APIPercentUsed, reset)
		if len(u.Meters) == 0 && pl.Limit.Or(0) > 0 {
			used, limit := cents(pl.IncludedSpend), cents(pl.Limit)
			u.Meters = append(u.Meters, core.Meter{ID: "total", Label: "Included usage", Headline: true, Unit: "usd",
				Used: core.F64(used), Limit: core.F64(limit), Percent: core.F64(used / limit * 100), ResetsAt: core.TimePtr(reset)})
		}
		if len(u.Meters) > 0 {
			u.Meters[0].Detail = spendDetail(cents(pl.TotalSpend), cents(pl.BonusSpend))
		}
	}
	if sl := pu.SpendLimitUsage; sl != nil && sl.IndividualLimit.Or(0) > 0 {
		used, limit := cents(sl.IndividualUsed), cents(sl.IndividualLimit)
		u.Meters = append(u.Meters, core.Meter{ID: "on_demand", Label: "On-demand", Unit: "usd",
			Used: core.F64(used), Limit: core.F64(limit), Percent: core.F64(used / limit * 100), ResetsAt: core.TimePtr(reset)})
	}
	return u
}

var membershipNames = map[string]string{
	"free": "Free", "free_trial": "Free trial", "pro": "Pro", "pro_plus": "Pro+", "ultra": "Ultra",
	"team": "Teams", "business": "Business", "enterprise": "Enterprise",
}

func membershipName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if n, ok := membershipNames[s]; ok {
		return n
	}
	return ui.TitleCase(s)
}

var errNoLogin = errors.New("no Cursor login found")
