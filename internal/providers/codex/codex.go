// Package codex reports OpenAI Codex (ChatGPT plan) rate limits: the 5-hour
// and weekly windows, per-model limits and credits.
//
// Sources, in "auto" order:
//  1. GET https://chatgpt.com/backend-api/wham/usage with the local login
//     (~/.codex/auth.json, $CODEX_HOME, Windows homes under WSL);
//  2. the official `codex app-server` (JSON-RPC `account/rateLimits/read`),
//     which manages and refreshes credentials itself;
//  3. the last rate-limit snapshot recorded in ~/.codex/sessions logs (stale).
//
// all-usage never refreshes or writes Codex credentials itself.
package codex

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cfardev/all-usage/internal/config"
	"github.com/cfardev/all-usage/internal/core"
	"github.com/cfardev/all-usage/internal/sys"
	"github.com/cfardev/all-usage/internal/ui"
)

// Provider implements core.Provider for Codex.
type Provider struct {
	cfg config.CodexConfig
	env *core.Env

	mu sync.Mutex
	// rejected remembers logins the API refused (keyed by auth.json path,
	// with the file's mtime), so they are retried only after the file changes.
	rejected map[string]rejection
	// appServerFail caches an app-server auth failure for the same auth state.
	appServerFail struct {
		at  time.Time
		key string
		err error
	}
}

type rejection struct {
	mod time.Time
	err error
}

// New returns a Codex provider.
func New(cfg config.CodexConfig, env *core.Env) *Provider {
	return &Provider{cfg: cfg, env: env, rejected: map[string]rejection{}}
}

// ID implements core.Provider.
func (p *Provider) ID() string { return config.Codex }

// Name implements core.Provider.
func (p *Provider) Name() string { return p.cfg.DisplayName }

// Fetch implements core.Provider.
func (p *Provider) Fetch(ctx context.Context) (*core.Usage, error) {
	now := p.env.Time()
	var snap *snapshot
	var err error
	switch p.cfg.Source {
	case "api":
		snap, err = p.viaAPI(ctx)
	case "app-server":
		snap, err = p.viaAppServer(ctx)
	case "sessions":
		snap, err = p.viaSessions(now)
	default:
		return p.fetchAuto(ctx, now)
	}
	if err != nil {
		return nil, err
	}
	return snap.toUsage(p.ID(), p.Name(), now, p.env.ShowAccount), nil
}

func (p *Provider) fetchAuto(ctx context.Context, now time.Time) (*core.Usage, error) {
	snap, liveErr := p.viaAPI(ctx)
	if liveErr == nil {
		return snap.toUsage(p.ID(), p.Name(), now, p.env.ShowAccount), nil
	}
	if p.cfg.UseAppServer && !isKind(liveErr, core.KindNetwork) && ctx.Err() == nil {
		s, err := p.viaAppServer(ctx)
		if err == nil {
			return s.toUsage(p.ID(), p.Name(), now, p.env.ShowAccount), nil
		}
		liveErr = moreRelevant(liveErr, err)
	}
	if p.cfg.SessionsFallback {
		if s, err := p.viaSessions(now); err == nil {
			u := s.toUsage(p.ID(), p.Name(), now, p.env.ShowAccount)
			u.Warning = core.AsError(liveErr)
			return u, nil
		}
	}
	return nil, liveErr
}

// viaAPI tries the usage API with each usable login, best first.
func (p *Provider) viaAPI(ctx context.Context) (*snapshot, error) {
	creds := p.credentials()
	if len(creds) == 0 {
		return nil, core.NewError(core.KindNotConfigured, "Install Codex and run `codex login`.", "no Codex login found")
	}
	now := p.env.Time()
	var best error
	for _, c := range creds {
		var err error
		switch {
		case c.APIKeyOnly:
			err = apiKeyError()
		case c.Err != nil:
			err = core.Wrap(core.KindNotConfigured, c.Err, loginHint, "cannot read Codex login").At(c.label())
		case c.expired(now):
			err = core.NewError(core.KindAuth, loginHint, "Codex login expired %s ago", ui.FormatDuration(now.Sub(c.Expiry))).At(c.label())
		default:
			if rerr := p.rejectedErr(c); rerr != nil {
				err = rerr
				break
			}
			snap, ferr := p.fetchAPI(ctx, c)
			if ferr == nil {
				return snap, nil
			}
			if !core.IsAuth(ferr) {
				return nil, ferr // service/network problem: don't try other logins
			}
			p.markRejected(c, ferr)
			err = ferr
		}
		best = moreRelevant(best, err)
	}
	return nil, best
}

func (p *Provider) viaAppServer(ctx context.Context) (*snapshot, error) {
	bin, ok := sys.FindBinary(p.cfg.Binary)
	if !ok {
		return nil, core.NewError(core.KindNotConfigured, "Install Codex or set `codex.binary` in the config.", "%s not found in PATH", p.cfg.Binary)
	}
	key := p.authStateKey()
	p.mu.Lock()
	cached := p.appServerFail
	p.mu.Unlock()
	if cached.err != nil && cached.key == key && time.Since(cached.at) < 15*time.Minute {
		return nil, cached.err
	}
	snap, err := p.fetchAppServer(ctx, bin)
	if err != nil && (core.IsAuth(err) || isKind(err, core.KindNotConfigured) || isKind(err, core.KindUnsupported)) {
		p.mu.Lock()
		p.appServerFail.at, p.appServerFail.key, p.appServerFail.err = time.Now(), key, err
		p.mu.Unlock()
	}
	return snap, err
}

func (p *Provider) viaSessions(now time.Time) (*snapshot, error) {
	for _, f := range sessionFiles(p.homes(), now, 30*24*time.Hour, 5) {
		if s, err := lastRateLimits(f.Path); err == nil {
			s.Source = "session log"
			return s, nil
		}
	}
	return nil, core.NewError(core.KindNotConfigured, "Use Codex once so it records your limits, or run `codex login`.",
		"no recent Codex session logs with usage data")
}

// authStateKey fingerprints the native auth.json so cached failures are
// retried as soon as the user signs in again.
func (p *Provider) authStateKey() string {
	for _, h := range p.homes() {
		if !h.Native {
			continue
		}
		if st, err := os.Stat(filepath.Join(h.Dir, "auth.json")); err == nil {
			return fmt.Sprintf("%s@%d", h.Dir, st.ModTime().UnixNano())
		}
		return h.Dir
	}
	return ""
}

func credKey(c credential) string {
	if c.Path == "" {
		return "env"
	}
	return c.Path
}

func (p *Provider) rejectedErr(c credential) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if r, ok := p.rejected[credKey(c)]; ok && r.mod.Equal(c.ModTime) {
		return r.err
	}
	return nil
}

func (p *Provider) markRejected(c credential, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rejected[credKey(c)] = rejection{mod: c.ModTime, err: err}
}

func isKind(err error, k core.ErrorKind) bool {
	e := core.AsError(err)
	return e != nil && e.Kind == k
}

// moreRelevant returns whichever error better explains the failure to a user.
func moreRelevant(a, b error) error {
	rank := func(err error) int {
		if err == nil {
			return -1
		}
		switch core.AsError(err).Kind {
		case core.KindAuth, core.KindUnsupported:
			return 4
		case core.KindAPI, core.KindParse:
			return 3
		case core.KindNetwork:
			return 2
		case core.KindNotConfigured:
			return 1
		default:
			return 0
		}
	}
	if rank(b) > rank(a) {
		return b
	}
	return a
}

// Doctor implements core.Provider.
func (p *Provider) Doctor(ctx context.Context) []core.Check {
	now := p.env.Time()
	var checks []core.Check
	checks = append(checks, core.Check{Status: core.CheckInfo, Label: "source", Detail: p.cfg.Source})
	creds := p.credentials()
	if len(creds) == 0 {
		checks = append(checks, core.Check{Status: core.CheckWarn, Label: "login",
			Detail: "no auth.json found (not signed in, or credentials kept in the OS keyring)"})
	}
	for _, c := range creds {
		chk := core.Check{Label: "login", Detail: c.label()}
		switch {
		case c.Err != nil:
			chk.Status, chk.Detail = core.CheckFail, chk.Detail+": "+c.Err.Error()
		case c.APIKeyOnly:
			chk.Status, chk.Detail = core.CheckWarn, chk.Detail+": API key only (no plan usage limits)"
		case c.expired(now):
			chk.Status, chk.Detail = core.CheckWarn, fmt.Sprintf("%s: %s token expired %s ago", chk.Detail, planOrChatGPT(c.Plan), ui.FormatDuration(now.Sub(c.Expiry)))
		default:
			chk.Status = core.CheckOK
			chk.Detail = fmt.Sprintf("%s: %s token", chk.Detail, planOrChatGPT(c.Plan))
			if !c.Expiry.IsZero() {
				chk.Detail += ", valid for " + ui.FormatDuration(c.Expiry.Sub(now))
			}
		}
		checks = append(checks, chk)
	}
	if bin, ok := sys.FindBinary(p.cfg.Binary); ok {
		st := core.CheckOK
		detail := sys.ShortPath(bin)
		if !p.cfg.UseAppServer {
			st, detail = core.CheckInfo, detail+" (app-server source disabled)"
		}
		checks = append(checks, core.Check{Status: st, Label: "codex binary", Detail: detail})
	} else {
		checks = append(checks, core.Check{Status: core.CheckWarn, Label: "codex binary", Detail: p.cfg.Binary + " not found (app-server source unavailable)"})
	}
	if files := sessionFiles(p.homes(), now, 30*24*time.Hour, 1); len(files) > 0 {
		checks = append(checks, core.Check{Status: core.CheckInfo, Label: "session logs",
			Detail: fmt.Sprintf("latest %s (%s)", ui.FormatAgo(files[0].ModTime, now), sys.ShortPath(files[0].Path))})
	} else {
		checks = append(checks, core.Check{Status: core.CheckInfo, Label: "session logs", Detail: "none in the last 30 days"})
	}
	return checks
}

func planOrChatGPT(plan string) string {
	if n := planName(plan); n != "" {
		return "ChatGPT " + n
	}
	return "ChatGPT"
}
