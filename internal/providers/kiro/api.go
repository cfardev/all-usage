package kiro

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/cfardev/all-usage/internal/core"
	"github.com/cfardev/all-usage/internal/httpx"
	"github.com/cfardev/all-usage/internal/jsonx"
	"github.com/cfardev/all-usage/internal/ui"
)

// usageLimits is the GetUsageLimits response (CodeWhisperer/Kiro service).
type usageLimits struct {
	NextDateReset    jsonx.String `json:"nextDateReset"`
	SubscriptionInfo *struct {
		SubscriptionTitle string `json:"subscriptionTitle"`
		Type              string `json:"type"`
	} `json:"subscriptionInfo"`
	OverageConfiguration *struct {
		OverageStatus string `json:"overageStatus"`
	} `json:"overageConfiguration"`
	UsageBreakdownList []breakdown `json:"usageBreakdownList"`
}

type breakdown struct {
	ResourceType      string       `json:"resourceType"`
	DisplayName       string       `json:"displayName"`
	DisplayNamePlural string       `json:"displayNamePlural"`
	Currency          string       `json:"currency"`
	CurrentUsage      jsonx.Number `json:"currentUsage"`
	CurrentUsageP     jsonx.Number `json:"currentUsageWithPrecision"`
	UsageLimit        jsonx.Number `json:"usageLimit"`
	UsageLimitP       jsonx.Number `json:"usageLimitWithPrecision"`
	CurrentOverages   jsonx.Number `json:"currentOverages"`
	CurrentOveragesP  jsonx.Number `json:"currentOveragesWithPrecision"`
	OverageCap        jsonx.Number `json:"overageCap"`
	OverageCapP       jsonx.Number `json:"overageCapWithPrecision"`
	OverageCharges    jsonx.Number `json:"overageCharges"`
	OverageRate       jsonx.Number `json:"overageRate"`
	NextDateReset     jsonx.String `json:"nextDateReset"`
	Bonuses           []struct {
		DisplayName   string       `json:"displayName"`
		Status        string       `json:"status"`
		ExpiresAt     jsonx.String `json:"expiresAt"`
		CurrentUsage  jsonx.Number `json:"currentUsage"`
		CurrentUsageP jsonx.Number `json:"currentUsageWithPrecision"`
		UsageLimit    jsonx.Number `json:"usageLimit"`
		UsageLimitP   jsonx.Number `json:"usageLimitWithPrecision"`
	} `json:"bonuses"`
	FreeTrialInfo *struct {
		Status        string       `json:"freeTrialStatus"`
		Expiry        jsonx.String `json:"freeTrialExpiry"`
		CurrentUsage  jsonx.Number `json:"currentUsage"`
		CurrentUsageP jsonx.Number `json:"currentUsageWithPrecision"`
		UsageLimit    jsonx.Number `json:"usageLimit"`
		UsageLimitP   jsonx.Number `json:"usageLimitWithPrecision"`
	} `json:"freeTrialInfo"`
}

func num(precise, plain jsonx.Number) float64 { return precise.Or(plain.Or(0)) }

func parseTime(s jsonx.String) time.Time {
	t, _ := jsonx.ParseTime(s.V)
	return t
}

// endpointFor mirrors kiro-cli: the service runs in us-east-1 and eu-central-1.
func endpointFor(region string) string {
	if region == "eu-central-1" {
		return "https://q.eu-central-1.amazonaws.com"
	}
	return "https://q.us-east-1.amazonaws.com"
}

func (p *Provider) fetchUsage(ctx context.Context, t *token) (*core.Usage, error) {
	arn := first(p.cfg.ProfileARN, t.ProfileARN)
	region := first(p.cfg.Region, regionFromARN(arn), t.Region, "us-east-1")
	endpoint := strings.TrimRight(first(p.cfg.Endpoint, endpointFor(region)), "/")
	host := endpoint
	if u, err := url.Parse(endpoint); err == nil && u.Host != "" {
		host = u.Host
	}
	q := url.Values{"origin": {"AI_EDITOR"}, "resourceType": {"AGENTIC_REQUEST"}}
	if arn != "" {
		q.Set("profileArn", arn)
	}
	resp, err := p.env.HTTP.Do(ctx, httpx.Request{
		URL:     endpoint + "/getUsageLimits?" + q.Encode(),
		Headers: map[string]string{"Authorization": "Bearer " + t.Access},
	})
	if err != nil {
		return nil, core.NetworkError(err, host)
	}
	switch {
	case resp.Status == 401 || resp.Status == 403:
		var body struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(resp.Body, &body)
		msg := "Kiro rejected the login"
		if strings.Contains(strings.ToLower(body.Message), "expired") {
			msg = "Kiro login expired"
		}
		return nil, core.NewError(core.KindAuth, loginHint(t.Source), "%s", msg).At(t.label())
	case !resp.OK():
		return nil, core.HTTPError(resp.Status, resp.Body, host)
	}
	u, err := parseUsage(resp.Body, p.env.Time())
	if err != nil {
		return nil, core.Wrap(core.KindParse, err, "", "unexpected response from %s", host)
	}
	u.Provider, u.Name = p.ID(), p.Name()
	u.Source = t.Source + " login"
	if t.Source == srcEnv {
		u.Source = "env token"
	}
	return u, nil
}

func loginHint(source string) string {
	if source == srcIDE {
		return "Sign in again in the Kiro IDE, or run `kiro-cli login`."
	}
	return "Run `kiro-cli login` to sign in again."
}

// parseUsage converts GetUsageLimits into meters. currentUsage includes
// overage (and bonus spend), so the plan meter subtracts both.
func parseUsage(body []byte, now time.Time) (*core.Usage, error) {
	var r usageLimits
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	if len(r.UsageBreakdownList) == 0 {
		return nil, errors.New("no usage breakdown in response")
	}
	u := &core.Usage{FetchedAt: now, Meters: []core.Meter{}}
	if r.SubscriptionInfo != nil {
		u.Plan = ui.TitleCase(r.SubscriptionInfo.SubscriptionTitle)
	}
	topReset := parseTime(r.NextDateReset)
	overageOn := r.OverageConfiguration != nil && strings.EqualFold(r.OverageConfiguration.OverageStatus, "ENABLED")

	for i, b := range r.UsageBreakdownList {
		id := strings.ToLower(b.ResourceType)
		if id == "" {
			id = fmt.Sprintf("resource%d", i+1)
		}
		label := first(b.DisplayNamePlural, b.DisplayName, ui.TitleCase(id))
		unit := strings.ToLower(label)
		reset := parseTime(b.NextDateReset)
		if reset.IsZero() {
			reset = topReset
		}
		used := num(b.CurrentUsageP, b.CurrentUsage)
		limit := num(b.UsageLimitP, b.UsageLimit)
		over := num(b.CurrentOveragesP, b.CurrentOverages)
		var bonusUsed float64
		for _, bo := range b.Bonuses {
			bonusUsed += num(bo.CurrentUsageP, bo.CurrentUsage)
		}
		planUsed := used
		if over > 0 && over <= planUsed {
			planUsed -= over
		}
		if bonusUsed > 0 && bonusUsed <= planUsed {
			planUsed -= bonusUsed
		}

		m := core.Meter{ID: id, Label: label, Headline: true, Unit: unit, ResetsAt: core.TimePtr(reset)}
		if limit > 0 {
			m.Used, m.Limit, m.Percent = core.F64(planUsed), core.F64(limit), core.F64(planUsed/limit*100)
		} else {
			m.Value = ui.FormatNumber(planUsed) + " " + unit
		}
		u.Meters = append(u.Meters, m)

		for _, bo := range b.Bonuses {
			if bo.Status != "" && !strings.EqualFold(bo.Status, "ACTIVE") {
				continue
			}
			bu, bl := num(bo.CurrentUsageP, bo.CurrentUsage), num(bo.UsageLimitP, bo.UsageLimit)
			bm := core.Meter{ID: id + ".bonus", Label: first(bo.DisplayName, "Bonus "+unit), Unit: unit}
			if bl > 0 {
				bm.Used, bm.Limit, bm.Percent = core.F64(bu), core.F64(bl), core.F64(bu/bl*100)
			} else {
				bm.Value = ui.FormatNumber(bu) + " " + unit
			}
			if exp := parseTime(bo.ExpiresAt); !exp.IsZero() {
				bm.Detail = "expires in " + ui.FormatDuration(exp.Sub(now))
			}
			u.Meters = append(u.Meters, bm)
		}

		if ft := b.FreeTrialInfo; ft != nil && strings.EqualFold(ft.Status, "ACTIVE") {
			fu, fl := num(ft.CurrentUsageP, ft.CurrentUsage), num(ft.UsageLimitP, ft.UsageLimit)
			fm := core.Meter{ID: id + ".free_trial", Label: "Free trial " + unit, Unit: unit}
			if fl > 0 {
				fm.Used, fm.Limit, fm.Percent = core.F64(fu), core.F64(fl), core.F64(fu/fl*100)
			} else {
				fm.Value = ui.FormatNumber(fu) + " " + unit
			}
			if exp := parseTime(ft.Expiry); !exp.IsZero() {
				fm.Detail = "trial ends in " + ui.FormatDuration(exp.Sub(now))
			}
			u.Meters = append(u.Meters, fm)
		}

		if over > 0 || overageOn {
			om := core.Meter{ID: id + ".overage", Label: "Overage", Unit: unit, ResetsAt: core.TimePtr(reset)}
			if capv := num(b.OverageCapP, b.OverageCap); overageOn && capv > 0 {
				om.Used, om.Limit, om.Percent = core.F64(over), core.F64(capv), core.F64(over/capv*100)
			} else {
				om.Value = ui.FormatNumber(over) + " " + unit
			}
			if ch := b.OverageCharges.Or(0); ch > 0 || over > 0 {
				om.Detail = ui.FormatMoney(ch, b.Currency) + " billed"
				if rate := b.OverageRate.Or(0); rate > 0 {
					om.Detail += fmt.Sprintf(" at %s/%s", ui.FormatMoney(rate, b.Currency), strings.ToLower(first(b.DisplayName, "unit")))
				}
			}
			u.Meters = append(u.Meters, om)
		}
	}
	return u, nil
}
