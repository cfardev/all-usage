// Package cli wires the cobra commands: the dashboard (default), show,
// doctor, init, config and themes.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/cfardev/all-usage/internal/buildinfo"
	"github.com/cfardev/all-usage/internal/config"
	"github.com/cfardev/all-usage/internal/core"
	"github.com/cfardev/all-usage/internal/httpx"
	"github.com/cfardev/all-usage/internal/providers"
	"github.com/cfardev/all-usage/internal/sys"
	"github.com/cfardev/all-usage/internal/tui"
	"github.com/cfardev/all-usage/internal/ui"
)

// exitError ends the program with a status code; msg (if any) goes to stderr.
type exitError struct {
	code int
	msg  string
}

func (e exitError) Error() string { return e.msg }

// options are the global flags.
type options struct {
	configPath string
	providers  []string
	theme      string
	noColor    bool
	account    bool

	refresh string
	compact bool
	once    bool
	json    bool
}

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	root := newRoot()
	if err := root.ExecuteContext(ctx); err != nil {
		var ee exitError
		if errors.As(err, &ee) {
			if ee.msg != "" {
				fmt.Fprintln(os.Stderr, ee.msg)
			}
			return ee.code
		}
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 1
	}
	return 0
}

const longHelp = `all-usage shows how much of your Codex, Kiro and Cursor subscriptions you
have used: rate-limit windows, credits and spend, with reset times.

It needs no setup: credentials are discovered from the official apps and CLIs
(Codex CLI, kiro-cli / Kiro IDE, Cursor IDE / cursor-agent), read-only. On WSL
it also finds the Windows apps' logins.

Run without arguments for the live dashboard. When stdout is not a terminal it
prints a one-shot table instead (see "all-usage show" for formats).

Configuration: "all-usage init" writes a commented config file with every
setting; "all-usage doctor" explains what was detected and how to fix problems.

Environment overrides:
  ALL_USAGE_CONFIG      config file path
  ALL_USAGE_PROVIDERS   providers to show, e.g. "codex,cursor"
  ALL_USAGE_REFRESH     refresh interval, e.g. "30s"
  ALL_USAGE_THEME       color theme
  ALL_USAGE_TIMEOUT     per-provider timeout
  ALL_USAGE_WINDOWS_HOME  Windows profile dir to search from WSL
  ALL_USAGE_CODEX_TOKEN (+ ALL_USAGE_CODEX_ACCOUNT_ID), ALL_USAGE_KIRO_TOKEN,
  ALL_USAGE_CURSOR_TOKEN  use a token instead of discovering one
  NO_COLOR              disable colors`

func newRoot() *cobra.Command {
	o := &options{}
	cmd := &cobra.Command{
		Use:           "all-usage",
		Short:         "Your Codex, Kiro and Cursor subscription usage at a glance",
		Long:          longHelp,
		Version:       buildinfo.String(),
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		Example: `  all-usage                      # live dashboard
  all-usage -p codex,kiro -r 30s # only Codex and Kiro, refresh every 30s
  all-usage show                 # print once
  all-usage show -f short        # one line for status bars
  all-usage doctor               # diagnose credential discovery`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := o.load(cmd)
			if err != nil {
				return err
			}
			switch {
			case o.json:
				return runShow(cmd, cfg, "json", "")
			case o.once || !isatty.IsTerminal(os.Stdout.Fd()) && !isatty.IsCygwinTerminal(os.Stdout.Fd()):
				return runShow(cmd, cfg, "table", "")
			}
			return runTUI(cfg)
		},
	}
	cmd.SetVersionTemplate("all-usage {{.Version}}\n")

	pf := cmd.PersistentFlags()
	pf.StringVarP(&o.configPath, "config", "c", "", "config file (default "+sys.ShortPath(config.DefaultPath())+")")
	pf.StringSliceVarP(&o.providers, "providers", "p", nil, "providers to show, in order ("+strings.Join(config.AllProviders, ",")+")")
	pf.StringVar(&o.theme, "theme", "", "color theme: "+strings.Join(config.Themes, ", "))
	pf.BoolVar(&o.noColor, "no-color", false, "disable colors")
	pf.BoolVar(&o.account, "show-account", false, "show account e-mails")

	f := cmd.Flags()
	f.StringVarP(&o.refresh, "refresh", "r", "", "auto-refresh interval, e.g. 30s or 5m (0 disables)")
	f.BoolVar(&o.compact, "compact", false, "compact cards: one line per meter")
	f.BoolVar(&o.once, "once", false, "print usage once and exit (same as the show command)")
	f.BoolVar(&o.json, "json", false, "print usage as JSON and exit")

	cmd.AddCommand(newShowCmd(o), newDoctorCmd(o), newInitCmd(o), newConfigCmd(o), newThemesCmd(o))
	return cmd
}

// load builds the effective config: file + env, then command-line flags.
func (o *options) load(cmd *cobra.Command) (*config.Config, error) {
	cfg, err := config.Load(o.configPath)
	if err != nil {
		return nil, err
	}
	if len(o.providers) > 0 {
		if err := cfg.SelectProviders(o.providers); err != nil {
			return nil, err
		}
	}
	if o.theme != "" {
		o.theme = strings.ToLower(o.theme)
		if !slices.Contains(config.Themes, o.theme) {
			return nil, fmt.Errorf("unknown theme %q (available: %s)", o.theme, strings.Join(config.Themes, ", "))
		}
		cfg.UI.Theme = o.theme
	}
	if o.noColor {
		cfg.UI.Color = "never"
	}
	if o.account {
		cfg.UI.ShowAccount = true
	}
	if f := cmd.Flags().Lookup("refresh"); f != nil && f.Changed {
		d, err := config.ParseDuration(o.refresh)
		if err != nil {
			return nil, fmt.Errorf("--refresh: %w", err)
		}
		cfg.RefreshInterval = config.Duration(d)
	}
	if o.compact {
		cfg.UI.Compact = true
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	ui.SetColorMode(cfg.UI.Color)
	return cfg, nil
}

func newEnv(cfg *config.Config) *core.Env {
	return &core.Env{
		HTTP:         httpx.New(cfg.Timeout.D(), buildinfo.UserAgent()),
		WindowsHomes: sys.WindowsHomes(cfg.WindowsHome, cfg.ScanWindows),
		ShowAccount:  cfg.UI.ShowAccount,
		Now:          time.Now,
	}
}

func runTUI(cfg *config.Config) error {
	ps := providers.Build(cfg, newEnv(cfg))
	_, err := tea.NewProgram(tui.New(cfg, ps), tea.WithAltScreen()).Run()
	return err
}
