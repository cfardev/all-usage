// Package kiro reports Kiro subscription usage (monthly credits, bonus and
// free-trial credits, overage) via the GetUsageLimits API that the Kiro IDE
// and kiro-cli use.
//
// Credentials are read (read-only) from kiro-cli's SQLite database or the Kiro
// IDE token cache. When the kiro-cli token has expired, all-usage can run
// `kiro-cli whoami`, which makes the official client refresh it; all-usage
// itself never refreshes or writes credentials.
package kiro

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/cfardev/all-usage/internal/config"
	"github.com/cfardev/all-usage/internal/core"
	"github.com/cfardev/all-usage/internal/sys"
	"github.com/cfardev/all-usage/internal/ui"
)

// Provider implements core.Provider for Kiro.
type Provider struct {
	cfg config.KiroConfig
	env *core.Env

	mu sync.Mutex
	// refreshFailed remembers the DB mtime when `kiro-cli whoami` failed to
	// produce a valid token, so it is not re-run until the login changes.
	refreshFailed map[string]time.Time
}

// New returns a Kiro provider.
func New(cfg config.KiroConfig, env *core.Env) *Provider {
	return &Provider{cfg: cfg, env: env, refreshFailed: map[string]time.Time{}}
}

// ID implements core.Provider.
func (p *Provider) ID() string { return config.Kiro }

// Name implements core.Provider.
func (p *Provider) Name() string { return p.cfg.DisplayName }

// Fetch implements core.Provider.
func (p *Provider) Fetch(ctx context.Context) (*core.Usage, error) {
	toks := p.tokens(ctx)
	now := p.env.Time()
	var valid []*token
	for _, t := range toks {
		if t.usable(now) {
			valid = append(valid, t)
		}
	}
	if len(valid) == 0 {
		if t := p.refreshViaCLI(ctx, toks); t != nil {
			valid = append(valid, t)
		}
	}
	if len(valid) == 0 {
		return nil, p.noLoginError(toks, now)
	}
	var lastErr error
	for i, t := range valid {
		u, err := p.fetchUsage(ctx, t)
		if err == nil {
			return u, nil
		}
		lastErr = err
		if !core.IsAuth(err) {
			return nil, err
		}
		// The API rejected a token that looked valid: let kiro-cli refresh it once.
		if t.Source == srcCLI && i == 0 {
			if r := p.refreshViaCLI(ctx, []*token{t}); r != nil {
				if u, err := p.fetchUsage(ctx, r); err == nil {
					return u, nil
				}
			}
		}
	}
	return nil, lastErr
}

// refreshViaCLI runs `kiro-cli whoami` (which refreshes an expired kiro-cli
// login) and re-reads the token from the native kiro-cli database.
func (p *Provider) refreshViaCLI(ctx context.Context, toks []*token) *token {
	if !p.cfg.RefreshWithCLI || p.cfg.Source == "ide" {
		return nil
	}
	native := p.dbPaths()[0]
	var cur *token
	for _, t := range toks {
		if t.Source == srcCLI && t.Path == native && t.Err == nil {
			cur = t
		}
	}
	if cur == nil {
		return nil // not signed in to kiro-cli: nothing to refresh
	}
	p.mu.Lock()
	failedAt, failed := p.refreshFailed[native]
	p.mu.Unlock()
	if failed && failedAt.Equal(cur.ModTime) {
		return nil
	}
	bin, ok := sys.FindBinary(p.cfg.Binary)
	if !ok {
		return nil
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, bin, "whoami", "--format", "json")
	cmd.Dir = sys.Home()
	cmd.Env = os.Environ()
	_ = cmd.Run() // the outcome is judged by re-reading the token
	t := readCLIToken(ctx, native)
	if t.usable(p.env.Time()) && t.Access != cur.Access {
		return t
	}
	p.mu.Lock()
	p.refreshFailed[native] = t.ModTime
	p.mu.Unlock()
	return nil
}

func (p *Provider) noLoginError(toks []*token, now time.Time) error {
	for _, t := range toks {
		if t.Err == nil && t.Access != "" && t.expired(now) {
			return core.NewError(core.KindAuth, loginHint(t.Source), "Kiro login expired %s ago", ui.FormatDuration(now.Sub(t.Expiry))).At(t.label())
		}
	}
	return core.NewError(core.KindNotConfigured, "Run `kiro-cli login`, or sign in to the Kiro IDE.", "no Kiro login found")
}

// Doctor implements core.Provider.
func (p *Provider) Doctor(ctx context.Context) []core.Check {
	now := p.env.Time()
	checks := []core.Check{{Status: core.CheckInfo, Label: "source", Detail: p.cfg.Source}}
	toks := p.tokens(ctx)
	for _, t := range toks {
		label := t.Source + " login"
		chk := core.Check{Label: label, Detail: t.label()}
		kind := ""
		if t.Kind != "" {
			kind = " (" + t.Kind + ")"
		}
		switch {
		case t.Err != nil:
			chk.Status, chk.Detail = core.CheckWarn, chk.Detail+": "+t.Err.Error()
		case t.expired(now):
			chk.Status = core.CheckWarn
			chk.Detail = fmt.Sprintf("%s%s: token expired %s ago", chk.Detail, kind, ui.FormatDuration(now.Sub(t.Expiry)))
			if t.Source == srcCLI && p.cfg.RefreshWithCLI {
				chk.Detail += " (kiro-cli will refresh it)"
			}
		default:
			chk.Status = core.CheckOK
			chk.Detail = fmt.Sprintf("%s%s: token valid", chk.Detail, kind)
			if !t.Expiry.IsZero() {
				chk.Detail += " for " + ui.FormatDuration(t.Expiry.Sub(now))
			}
		}
		checks = append(checks, chk)
		if t.Err == nil && t.ProfileARN != "" {
			checks = append(checks, core.Check{Status: core.CheckInfo, Label: "profile", Detail: fmt.Sprintf("region %s", first(p.cfg.Region, regionFromARN(t.ProfileARN), t.Region))})
		}
	}
	if len(toks) == 0 {
		checks = append(checks, core.Check{Status: core.CheckWarn, Label: "login", Detail: "no kiro-cli database or Kiro IDE token found"})
	}
	if bin, ok := sys.FindBinary(p.cfg.Binary); ok {
		checks = append(checks, core.Check{Status: core.CheckOK, Label: "kiro-cli binary", Detail: sys.ShortPath(bin)})
	} else {
		checks = append(checks, core.Check{Status: core.CheckInfo, Label: "kiro-cli binary", Detail: p.cfg.Binary + " not found (expired kiro-cli logins cannot be refreshed)"})
	}
	return checks
}
