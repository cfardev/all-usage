package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/cfardev/all-usage/internal/config"
	"github.com/cfardev/all-usage/internal/core"
	"github.com/cfardev/all-usage/internal/providers"
	"github.com/cfardev/all-usage/internal/render"
	"github.com/cfardev/all-usage/internal/ui"
)

const showLong = `Print current usage once and exit.

Formats (--format):
  table   bars, percentages and reset times (default)
  short   one line for status bars, e.g. "Codex 5h 22% · wk 56% | Kiro 42%"
  json    machine-readable output

--template renders a Go text/template instead. Data: .Now and .Providers,
each with .ID .Name .Plan .OK .Stale .Error .Hint .Meters .Headline. Meters
have .ID .Label .Short .Value .Detail .Percent .Used .Limit .Unit .ResetsAt.
Helpers: pct, used, left, reset, bar <meter> <width>, money, num, upper,
lower, join.

Exit status is 1 when no provider could be fetched.`

func newShowCmd(o *options) *cobra.Command {
	var format, tmpl string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Print current usage once and exit",
		Long:  showLong,
		Example: `  all-usage show
  all-usage show -f short -p codex
  all-usage show --json | jq '.providers[].usage.meters'
  all-usage show -t '{{range .Providers}}{{.Name}}:{{range .Headline}} {{pct .}}{{end}} {{end}}'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := o.load(cmd)
			if err != nil {
				return err
			}
			if asJSON {
				format = "json"
			}
			if tmpl != "" {
				format = "template"
			}
			return runShow(cmd, cfg, format, tmpl)
		},
	}
	cmd.Flags().StringVarP(&format, "format", "f", "table", "output format: table, short, json")
	cmd.Flags().StringVarP(&tmpl, "template", "t", "", "custom Go template (see help)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "shorthand for --format json")
	return cmd
}

func renderOptions(cfg *config.Config) render.Options {
	colors := map[string]string{}
	for _, id := range config.AllProviders {
		colors[id] = cfg.Common(id).Color
	}
	return render.Options{
		UI: ui.Options{
			Remaining:   cfg.UI.Percent == "remaining",
			ResetFormat: cfg.UI.ResetFormat,
			Clock:       cfg.UI.Clock,
			WarnAt:      cfg.UI.WarnAt,
			CriticalAt:  cfg.UI.CriticalAt,
			BarStyle:    cfg.UI.BarStyle,
		},
		Theme:      ui.NewTheme(cfg.UI.Theme, cfg.UI.Colors),
		Now:        time.Now(),
		ShowSource: cfg.UI.ShowSource,
		Colors:     colors,
	}
}

func runShow(cmd *cobra.Command, cfg *config.Config, format, tmpl string) error {
	switch format {
	case "table", "short", "json", "template":
	default:
		return fmt.Errorf("unknown format %q (use table, short or json)", format)
	}
	ps := providers.Build(cfg, newEnv(cfg))
	if len(ps) == 0 {
		return errors.New("no providers enabled (check `order`/`enabled` in the config or --providers)")
	}
	for _, w := range cfg.Warnings {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning:", w)
	}
	results := core.FetchAll(cmd.Context(), ps, cfg.Timeout.D())
	out := cmd.OutOrStdout()
	opts := renderOptions(cfg)
	switch format {
	case "short":
		render.Short(out, results, opts)
	case "json":
		if err := render.JSON(out, results, opts.Now); err != nil {
			return err
		}
	case "template":
		if err := render.Template(out, tmpl, results, opts); err != nil {
			return err
		}
	default:
		render.Table(out, results, opts)
	}
	for _, r := range results {
		if r.OK() {
			return nil
		}
	}
	return exitError{code: 1}
}
