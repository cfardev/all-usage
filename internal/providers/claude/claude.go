// Package claude reports Claude subscription usage (Pro/Max): the 5-hour
// session window, the weekly windows and extra usage.
//
// The OAuth token is read (read-only) from the Claude Code login
// (~/.claude/.credentials.json, the Windows one under WSL, or the macOS
// Keychain). all-usage never refreshes or writes Claude credentials: refresh
// tokens rotate, so refreshing here would sign Claude Code out.
package claude

import (
	"context"
	"errors"
	"fmt"

	"github.com/cfardev/all-usage/internal/config"
	"github.com/cfardev/all-usage/internal/core"
	"github.com/cfardev/all-usage/internal/ui"
)

// Provider implements core.Provider for Claude.
type Provider struct {
	cfg config.ClaudeConfig
	env *core.Env
}

// New returns a Claude provider.
func New(cfg config.ClaudeConfig, env *core.Env) *Provider {
	return &Provider{cfg: cfg, env: env}
}

// ID implements core.Provider.
func (p *Provider) ID() string { return config.Claude }

// Name implements core.Provider.
func (p *Provider) Name() string { return p.cfg.DisplayName }

var errNoLogin = errors.New("no Claude login found")

// Fetch implements core.Provider.
func (p *Provider) Fetch(ctx context.Context) (*core.Usage, error) {
	creds := p.credentials(ctx)
	now := p.env.Time()
	var lastErr error
	tried := false
	for _, c := range creds {
		if !c.usable(now) {
			continue
		}
		tried = true
		u, err := p.fetchWith(ctx, c)
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
	for _, c := range creds {
		if c.Err == nil && c.expired(now) {
			return nil, core.NewError(core.KindAuth, loginHint, "Claude login expired %s ago", ui.FormatDuration(now.Sub(c.Expiry))).At(c.label())
		}
	}
	return nil, core.Wrap(core.KindNotConfigured, errNoLogin, "Sign in with Claude Code: run `claude` and use /login.", "no Claude login found")
}

// Doctor implements core.Provider.
func (p *Provider) Doctor(ctx context.Context) []core.Check {
	now := p.env.Time()
	var checks []core.Check
	creds := p.credentials(ctx)
	for _, c := range creds {
		chk := core.Check{Label: c.Source + " login", Detail: c.label()}
		switch {
		case c.Err != nil:
			chk.Status, chk.Detail = core.CheckWarn, chk.Detail+": "+c.Err.Error()
		case c.expired(now):
			chk.Status, chk.Detail = core.CheckWarn, fmt.Sprintf("%s: token expired %s ago", chk.Detail, ui.FormatDuration(now.Sub(c.Expiry)))
		default:
			chk.Status = core.CheckOK
			chk.Detail += ": token valid"
			if !c.Expiry.IsZero() {
				chk.Detail += " for " + ui.FormatDuration(c.Expiry.Sub(now))
			}
			if plan := c.plan(); plan != "" {
				chk.Detail += " (" + plan + ")"
			}
		}
		checks = append(checks, chk)
	}
	if len(creds) == 0 {
		checks = append(checks, core.Check{Status: core.CheckWarn, Label: "login", Detail: "no Claude Code login found"})
	}
	return checks
}
