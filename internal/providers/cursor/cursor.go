// Package cursor reports Cursor subscription usage for the current billing
// cycle (total/Auto/API usage percentages, on-demand spend, legacy request
// quotas).
//
// The session token is read (read-only) from the Cursor IDE state database
// (including the Windows install under WSL) or the cursor-agent CLI login.
// all-usage never refreshes or writes Cursor credentials.
package cursor

import (
	"context"
	"fmt"

	"github.com/cfardev/all-usage/internal/config"
	"github.com/cfardev/all-usage/internal/core"
	"github.com/cfardev/all-usage/internal/ui"
)

// Provider implements core.Provider for Cursor.
type Provider struct {
	cfg config.CursorConfig
	env *core.Env
}

// New returns a Cursor provider.
func New(cfg config.CursorConfig, env *core.Env) *Provider {
	return &Provider{cfg: cfg, env: env}
}

// ID implements core.Provider.
func (p *Provider) ID() string { return config.Cursor }

// Name implements core.Provider.
func (p *Provider) Name() string { return p.cfg.DisplayName }

// Fetch implements core.Provider.
func (p *Provider) Fetch(ctx context.Context) (*core.Usage, error) {
	toks := p.tokens(ctx)
	now := p.env.Time()
	var lastErr error
	tried := false
	for _, t := range toks {
		if !t.usable(now) {
			continue
		}
		tried = true
		u, err := p.fetchWith(ctx, t)
		if err == nil {
			u.Provider, u.Name = p.ID(), p.Name()
			return u, nil
		}
		lastErr = err
		if !core.IsAuth(err) {
			return nil, err // service/network problem: don't try other logins
		}
	}
	if tried {
		return nil, lastErr
	}
	for _, t := range toks {
		if t.Err == nil && t.expired(now) {
			return nil, core.NewError(core.KindAuth, loginHint, "Cursor login expired %s ago", ui.FormatDuration(now.Sub(t.Expiry))).At(t.label())
		}
	}
	return nil, core.Wrap(core.KindNotConfigured, errNoLogin, "Sign in to the Cursor app, or run `cursor-agent login`.", "no Cursor login found")
}

// Doctor implements core.Provider.
func (p *Provider) Doctor(ctx context.Context) []core.Check {
	now := p.env.Time()
	checks := []core.Check{{Status: core.CheckInfo, Label: "source", Detail: p.cfg.Source}}
	toks := p.tokens(ctx)
	for _, t := range toks {
		chk := core.Check{Label: t.Source + " login", Detail: t.label()}
		switch {
		case t.Err != nil:
			chk.Status, chk.Detail = core.CheckWarn, chk.Detail+": "+t.Err.Error()
		case t.expired(now):
			chk.Status, chk.Detail = core.CheckWarn, fmt.Sprintf("%s: token expired %s ago", chk.Detail, ui.FormatDuration(now.Sub(t.Expiry)))
		default:
			chk.Status = core.CheckOK
			chk.Detail += ": token valid"
			if !t.Expiry.IsZero() {
				chk.Detail += " for " + ui.FormatDuration(t.Expiry.Sub(now))
			}
			if t.Membership != "" {
				chk.Detail += " (" + membershipName(t.Membership) + ")"
			}
		}
		checks = append(checks, chk)
	}
	if len(toks) == 0 {
		checks = append(checks, core.Check{Status: core.CheckWarn, Label: "login", Detail: "no Cursor IDE database or cursor-agent login found"})
	}
	return checks
}
