package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/cfardev/all-usage/internal/buildinfo"
	"github.com/cfardev/all-usage/internal/config"
	"github.com/cfardev/all-usage/internal/core"
	"github.com/cfardev/all-usage/internal/providers"
	"github.com/cfardev/all-usage/internal/sys"
	"github.com/cfardev/all-usage/internal/ui"
)

func newDoctorCmd(o *options) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Explain which credentials were found and test each provider",
		Long: `Check the configuration, show where each provider's login was found (never
printing secrets), then fetch usage once and report the result with a hint to
fix any problem. Exit status is 1 if any provider fails.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := o.load(cmd)
			if err != nil {
				return err
			}
			return runDoctor(cmd, cfg, o)
		},
	}
}

func runDoctor(cmd *cobra.Command, cfg *config.Config, o *options) error {
	w := cmd.OutOrStdout()
	t := ui.NewTheme(cfg.UI.Theme, cfg.UI.Colors)
	bold := lipgloss.NewStyle().Bold(true).Foreground(t.Text)
	muted := lipgloss.NewStyle().Foreground(t.Muted)
	marks := map[core.CheckStatus]string{
		core.CheckOK:   lipgloss.NewStyle().Foreground(t.OK).Render("✓"),
		core.CheckWarn: lipgloss.NewStyle().Foreground(t.Warn).Render("!"),
		core.CheckFail: lipgloss.NewStyle().Foreground(t.Critical).Render("✗"),
		core.CheckInfo: muted.Render("·"),
	}
	line := func(c core.Check) {
		fmt.Fprintf(w, "  %s %-19s %s\n", marks[c.Status], c.Label, c.Detail)
	}

	env := newEnv(cfg)
	platform := runtime.GOOS + "/" + runtime.GOARCH
	if sys.IsWSL() {
		platform += " (WSL)"
	}
	fmt.Fprintf(w, "%s %s · %s\n\n", bold.Render("all-usage"), buildinfo.String(), platform)

	fmt.Fprintln(w, bold.Render("Config"))
	if cfg.Path != "" {
		line(core.Check{Status: core.CheckOK, Label: "file", Detail: sys.ShortPath(cfg.Path)})
	} else {
		path, _ := config.ResolvePath(o.configPath)
		line(core.Check{Status: core.CheckInfo, Label: "file", Detail: sys.ShortPath(path) + " (not found; using defaults — run `all-usage init`)"})
	}
	for _, warn := range cfg.Warnings {
		line(core.Check{Status: core.CheckWarn, Label: "warning", Detail: warn})
	}
	if sys.IsWSL() || cfg.WindowsHome != "" {
		if len(env.WindowsHomes) > 0 {
			line(core.Check{Status: core.CheckInfo, Label: "windows homes", Detail: strings.Join(env.WindowsHomes, ", ")})
		} else {
			line(core.Check{Status: core.CheckInfo, Label: "windows homes", Detail: "none (set windows_home to search one)"})
		}
	}
	line(core.Check{Status: core.CheckInfo, Label: "providers", Detail: strings.Join(cfg.EnabledProviders(), ", ")})

	ps := providers.Build(cfg, env)
	results := core.FetchAll(cmd.Context(), ps, cfg.Timeout.D())
	failed := false
	for i, p := range ps {
		fmt.Fprintln(w)
		fmt.Fprintln(w, bold.Render(p.Name()))
		for _, c := range p.Doctor(cmd.Context()) {
			line(c)
		}
		r := results[i]
		switch {
		case r.OK():
			line(core.Check{Status: core.CheckOK, Label: "usage", Detail: summarize(r)})
			if wr := r.Usage.Warning; wr != nil {
				line(core.Check{Status: core.CheckWarn, Label: "live data", Detail: wr.Full()})
				if wr.Hint != "" {
					line(core.Check{Status: core.CheckInfo, Label: "", Detail: "→ " + wr.Hint})
				}
			}
		default:
			failed = true
			line(core.Check{Status: core.CheckFail, Label: "usage", Detail: r.Err.Title() + ": " + r.Err.Full()})
			if r.Err.Hint != "" {
				line(core.Check{Status: core.CheckInfo, Label: "", Detail: "→ " + r.Err.Hint})
			}
		}
	}
	if failed {
		return exitError{code: 1}
	}
	return nil
}

func summarize(r core.Result) string {
	u := r.Usage
	var parts []string
	for _, m := range u.Meters {
		if m.HasPercent() {
			parts = append(parts, m.Label+" "+ui.FormatPercent(m.Pct()))
		} else {
			parts = append(parts, m.Label+" "+m.Value)
		}
		if len(parts) == 3 {
			break
		}
	}
	s := strings.Join(parts, ", ")
	if u.Plan != "" {
		s = u.Plan + " · " + s
	}
	return fmt.Sprintf("%s (%s, %.1fs)", s, u.Source, r.Duration.Seconds())
}

func newInitCmd(o *options) *cobra.Command {
	var force, stdout bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a commented config file with every setting",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if stdout {
				_, err := io.WriteString(cmd.OutOrStdout(), config.Template)
				return err
			}
			path, _ := config.ResolvePath(o.configPath)
			if err := config.WriteTemplate(path, force); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created %s\nEdit it with `all-usage config edit`, then run `all-usage`.\n", sys.ShortPath(path))
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing config file")
	cmd.Flags().BoolVar(&stdout, "stdout", false, "print the template instead of writing it")
	return cmd
}

func newConfigCmd(o *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show, locate or edit the configuration",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "path",
		Short: "Print the config file path",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, _ := config.ResolvePath(o.configPath)
			fmt.Fprintln(cmd.OutOrStdout(), path)
			if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
				fmt.Fprintln(cmd.ErrOrStderr(), "(not created yet: run `all-usage init`)")
			}
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Print the effective configuration (defaults + file + env + flags)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := o.load(cmd)
			if err != nil {
				return err
			}
			s, err := cfg.Encode()
			if err != nil {
				return err
			}
			_, err = io.WriteString(cmd.OutOrStdout(), s)
			return err
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "edit",
		Short: "Open the config file in $VISUAL/$EDITOR (creating it if needed)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, _ := config.ResolvePath(o.configPath)
			if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
				if err := config.WriteTemplate(path, false); err != nil {
					return err
				}
			}
			editor := firstNonEmpty(os.Getenv("VISUAL"), os.Getenv("EDITOR"))
			if editor == "" {
				editor = "vi"
				if runtime.GOOS == "windows" {
					editor = "notepad"
				} else if _, ok := sys.FindBinary("nano"); ok {
					editor = "nano"
				}
			}
			args := strings.Fields(editor)
			c := exec.CommandContext(cmd.Context(), args[0], append(args[1:], path)...)
			c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
			if err := c.Run(); err != nil {
				return fmt.Errorf("editor %q: %w", editor, err)
			}
			if _, err := config.Load(path); err != nil {
				return fmt.Errorf("the config has problems:\n%w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Saved %s (valid)\n", sys.ShortPath(path))
			return nil
		},
	})
	return cmd
}

func newThemesCmd(o *options) *cobra.Command {
	return &cobra.Command{
		Use:   "themes",
		Short: "Preview the available color themes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := o.load(cmd)
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			for _, name := range config.Themes {
				t := ui.NewTheme(name, cfg.UI.Colors)
				label := lipgloss.NewStyle().Bold(true).Foreground(t.Accent).Render(fmt.Sprintf("%-11s", name))
				bars := []string{
					t.Bar(35, 10, cfg.UI.BarStyle, t.OK),
					t.Bar(75, 10, cfg.UI.BarStyle, t.Warn),
					t.Bar(95, 10, cfg.UI.BarStyle, t.Critical),
				}
				mark := ""
				if name == cfg.UI.Theme {
					mark = lipgloss.NewStyle().Foreground(t.Muted).Render("  (current)")
				}
				fmt.Fprintf(w, "%s %s%s\n", label, strings.Join(bars, " "), mark)
			}
			fmt.Fprintln(w, "\nUse one with --theme NAME, ALL_USAGE_THEME or `theme` in the [ui] config section.")
			return nil
		},
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
