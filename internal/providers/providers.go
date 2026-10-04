// Package providers builds the configured usage providers.
package providers

import (
	"context"

	"github.com/cfardev/all-usage/internal/config"
	"github.com/cfardev/all-usage/internal/core"
	"github.com/cfardev/all-usage/internal/providers/codex"
	"github.com/cfardev/all-usage/internal/providers/cursor"
	"github.com/cfardev/all-usage/internal/providers/kiro"
)

// Build returns the enabled providers in display order.
func Build(cfg *config.Config, env *core.Env) []core.Provider {
	var out []core.Provider
	for _, id := range cfg.EnabledProviders() {
		var p core.Provider
		switch id {
		case config.Codex:
			p = codex.New(cfg.Codex, env)
		case config.Kiro:
			p = kiro.New(cfg.Kiro, env)
		case config.Cursor:
			p = cursor.New(cfg.Cursor, env)
		default:
			continue
		}
		if meters := cfg.Common(id).Meters; len(meters) > 0 {
			p = filtered{Provider: p, meters: meters}
		}
		out = append(out, p)
	}
	return out
}

// filtered restricts a provider's meters to the configured IDs.
type filtered struct {
	core.Provider
	meters []string
}

func (f filtered) Fetch(ctx context.Context) (*core.Usage, error) {
	u, err := f.Provider.Fetch(ctx)
	if u != nil {
		u.Meters = core.FilterMeters(u.Meters, f.meters)
	}
	return u, err
}
