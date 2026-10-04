package codex

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/cfardev/all-usage/internal/core"
	"github.com/cfardev/all-usage/internal/httpx"
	"github.com/cfardev/all-usage/internal/jsonx"
)

const loginHint = "Run `codex login` (or open Codex) to sign in again."

// Payload of GET {base}/wham/usage (see codex-rs codex-backend-openapi-models).
type whamPayload struct {
	PlanType   string         `json:"plan_type"`
	RateLimit  *whamRateLimit `json:"rate_limit"`
	Credits    *whamCredits   `json:"credits"`
	Additional []struct {
		LimitName      string         `json:"limit_name"`
		MeteredFeature string         `json:"metered_feature"`
		RateLimit      *whamRateLimit `json:"rate_limit"`
	} `json:"additional_rate_limits"`
	Reached json.RawMessage `json:"rate_limit_reached_type"`
}

type whamRateLimit struct {
	Primary   *whamWindow `json:"primary_window"`
	Secondary *whamWindow `json:"secondary_window"`
}

type whamWindow struct {
	UsedPercent        float64      `json:"used_percent"`
	LimitWindowSeconds jsonx.Number `json:"limit_window_seconds"`
	ResetAfterSeconds  jsonx.Number `json:"reset_after_seconds"`
	ResetAt            jsonx.Number `json:"reset_at"`
}

type whamCredits struct {
	HasCredits bool         `json:"has_credits"`
	Unlimited  bool         `json:"unlimited"`
	Balance    jsonx.String `json:"balance"`
}

func (w *whamWindow) toWindow(now time.Time) *window {
	if w == nil {
		return nil
	}
	out := &window{UsedPercent: w.UsedPercent, Length: time.Duration(w.LimitWindowSeconds.Or(0)) * time.Second}
	if w.ResetAt.Or(0) > 0 {
		out.ResetsAt = jsonx.UnixTime(w.ResetAt.V)
	} else if w.ResetAfterSeconds.Valid {
		out.ResetsAt = now.Add(time.Duration(w.ResetAfterSeconds.V) * time.Second)
	}
	return out
}

func (c *whamCredits) toCredits() *credits {
	if c == nil {
		return nil
	}
	return &credits{HasCredits: c.HasCredits, Unlimited: c.Unlimited, Balance: c.Balance.V}
}

func parseWham(body []byte, now time.Time) (*snapshot, error) {
	var p whamPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	s := &snapshot{Plan: p.PlanType, Credits: p.Credits.toCredits(), Reached: parseReached(p.Reached)}
	main := limitGroup{ID: "codex"}
	if p.RateLimit != nil {
		main.Primary = p.RateLimit.Primary.toWindow(now)
		main.Secondary = p.RateLimit.Secondary.toWindow(now)
	}
	s.Groups = append(s.Groups, main)
	for _, a := range p.Additional {
		if a.RateLimit == nil {
			continue
		}
		g := limitGroup{ID: a.MeteredFeature, Name: a.LimitName,
			Primary: a.RateLimit.Primary.toWindow(now), Secondary: a.RateLimit.Secondary.toWindow(now)}
		if g.ID == "" {
			g.ID = slug(a.LimitName)
		}
		if g.Name == "" {
			g.Name = g.ID
		}
		if g.Primary != nil || g.Secondary != nil {
			s.Groups = append(s.Groups, g)
		}
	}
	if !s.hasData() && p.RateLimit == nil {
		return nil, errors.New("response has no rate limits")
	}
	return s, nil
}

// fetchAPI calls the ChatGPT usage endpoint with one credential.
func (p *Provider) fetchAPI(ctx context.Context, c credential) (*snapshot, error) {
	base := strings.TrimRight(p.cfg.BaseURL, "/")
	host := hostOf(base)
	headers := map[string]string{"Authorization": "Bearer " + c.AccessToken}
	if c.AccountID != "" {
		headers["ChatGPT-Account-Id"] = c.AccountID
	}
	resp, err := p.env.HTTP.Do(ctx, httpx.Request{URL: base + "/wham/usage", Headers: headers})
	if err != nil {
		return nil, core.NetworkError(err, host)
	}
	switch {
	case resp.Status == 401 || resp.Status == 403:
		msg := "Codex login was rejected"
		if strings.Contains(string(resp.Body), "token_expired") {
			msg = "Codex login expired"
		}
		return nil, core.NewError(core.KindAuth, loginHint, "%s", msg).At(c.label())
	case !resp.OK():
		return nil, core.HTTPError(resp.Status, resp.Body, host)
	}
	snap, err := parseWham(resp.Body, p.env.Time())
	if err != nil {
		return nil, core.Wrap(core.KindParse, err, "", "unexpected response from %s", host)
	}
	snap.Source = host + " API"
	if snap.Plan == "" {
		snap.Plan = c.Plan
	}
	snap.Email = c.Email
	return snap, nil
}

func hostOf(base string) string {
	if u, err := url.Parse(base); err == nil && u.Host != "" {
		return u.Host
	}
	return base
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, s)
}
