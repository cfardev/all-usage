package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cfardev/all-usage/internal/config"
	"github.com/cfardev/all-usage/internal/testutil"
)

// isolate points every credential location at an empty temp home.
func isolate(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for k, v := range map[string]string{
		"HOME": home, "USERPROFILE": home, "XDG_CONFIG_HOME": filepath.Join(home, ".config"),
		"XDG_DATA_HOME": filepath.Join(home, ".local", "share"), "CODEX_HOME": filepath.Join(home, ".codex"),
		"APPDATA": filepath.Join(home, "AppData", "Roaming"), "LOCALAPPDATA": filepath.Join(home, "AppData", "Local"),
		"ALL_USAGE_CONFIG": "", "ALL_USAGE_PROVIDERS": "", "ALL_USAGE_REFRESH": "", "ALL_USAGE_THEME": "",
		"ALL_USAGE_TIMEOUT": "", "ALL_USAGE_WINDOWS_HOME": "", "ALL_USAGE_CODEX_TOKEN": "", "ALL_USAGE_CODEX_ACCOUNT_ID": "",
		"ALL_USAGE_KIRO_TOKEN": "", "ALL_USAGE_KIRO_PROFILE_ARN": "", "ALL_USAGE_CURSOR_TOKEN": "", "NO_COLOR": "1",
	} {
		t.Setenv(k, v)
	}
	return home
}

func fakeServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		switch r.URL.Path {
		case "/codex/wham/usage":
			if auth != "Bearer codex-token" {
				w.WriteHeader(401)
				return
			}
			_, _ = w.Write([]byte(`{"plan_type":"pro","rate_limit":{"primary_window":{"used_percent":22,"limit_window_seconds":18000,"reset_at":0,"reset_after_seconds":3600},"secondary_window":{"used_percent":56,"limit_window_seconds":604800,"reset_after_seconds":86400}}}`))
		case "/kiro/getUsageLimits":
			if auth != "Bearer kiro-token" {
				w.WriteHeader(403)
				return
			}
			_, _ = w.Write([]byte(`{"subscriptionInfo":{"subscriptionTitle":"KIRO PRO"},"usageBreakdownList":[{"resourceType":"CREDIT","displayNamePlural":"Credits","currentUsageWithPrecision":250,"usageLimitWithPrecision":1000,"nextDateReset":1793491200}]}`))
		case "/cursor/api/usage-summary":
			w.WriteHeader(401) // force the bearer RPC fallback
		case "/cursor/aiserver.v1.DashboardService/GetCurrentPeriodUsage":
			_, _ = w.Write([]byte(`{"billingCycleEnd":"1793544890000","planUsage":{"totalPercentUsed":12.5,"autoPercentUsed":10,"apiPercentUsed":20}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func writeConfig(t *testing.T, dir, base string) string {
	t.Helper()
	p := filepath.Join(dir, "config.toml")
	testutil.WriteFile(t, p, `
scan_windows = false
timeout = "5s"
[codex]
use_app_server = false
sessions_fallback = false
base_url = "`+base+`/codex"
[kiro]
refresh_with_cli = false
endpoint = "`+base+`/kiro"
[cursor]
api_base = "`+base+`/cursor"
web_base = "`+base+`/cursor"
`)
	return p
}

func run(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := newRoot()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(args)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := cmd.ExecuteContext(ctx)
	return out.String(), errb.String(), err
}

func TestShowAllFormats(t *testing.T) {
	home := isolate(t)
	cfg := writeConfig(t, home, fakeServer(t).URL)
	t.Setenv("ALL_USAGE_CODEX_TOKEN", "codex-token")
	t.Setenv("ALL_USAGE_KIRO_TOKEN", "kiro-token")
	t.Setenv("ALL_USAGE_CURSOR_TOKEN", testutil.JWT(map[string]any{"sub": "auth0|user_X", "exp": time.Now().Add(time.Hour).Unix()}))

	out, _, err := run(t, "show", "-c", cfg)
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	for _, want := range []string{"Codex  Pro", "5-hour limit", "22%", "Kiro  Kiro Pro", "250 / 1,000 credits", "Cursor", "12.5%"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q:\n%s", want, out)
		}
	}

	out, _, err = run(t, "show", "-c", cfg, "-f", "short", "-p", "kiro,codex")
	if err != nil || out != "Kiro 25% | Codex 5h 22% · wk 56%\n" {
		t.Errorf("short = %q, %v", out, err)
	}

	out, _, err = run(t, "show", "-c", cfg, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Providers []struct {
			ID string `json:"id"`
			OK bool   `json:"ok"`
		} `json:"providers"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil || len(doc.Providers) != 3 {
		t.Fatalf("json: %v\n%s", err, out)
	}
	for _, p := range doc.Providers {
		if !p.OK {
			t.Errorf("%s not ok:\n%s", p.ID, out)
		}
	}

	// Without a terminal, the root command prints the table once.
	out, _, err = run(t, "-c", cfg, "-p", "cursor")
	if err != nil || !strings.Contains(out, "Total usage") {
		t.Errorf("root (non-tty) = %q, %v", out, err)
	}

	out, _, err = run(t, "show", "-c", cfg, "-p", "codex", "-t", `{{range .Providers}}{{.Plan}} {{range .Headline}}{{pct .}} {{end}}{{end}}`)
	if err != nil || out != "Pro 22% 56% \n" {
		t.Errorf("template = %q, %v", out, err)
	}
}

func TestShowFailsWhenNothingWorks(t *testing.T) {
	home := isolate(t)
	cfg := writeConfig(t, home, fakeServer(t).URL)
	out, _, err := run(t, "show", "-c", cfg)
	var ee exitError
	if !errors.As(err, &ee) || ee.code != 1 {
		t.Fatalf("expected exit code 1, got %v", err)
	}
	if !strings.Contains(out, "Not signed in") || !strings.Contains(out, "codex login") {
		t.Errorf("expected actionable errors:\n%s", out)
	}
}

func TestDoctor(t *testing.T) {
	home := isolate(t)
	cfg := writeConfig(t, home, fakeServer(t).URL)
	t.Setenv("ALL_USAGE_KIRO_TOKEN", "kiro-token")
	out, _, err := run(t, "doctor", "-c", cfg, "-p", "kiro,cursor")
	var ee exitError
	if !errors.As(err, &ee) || ee.code != 1 {
		t.Fatalf("cursor is not signed in, doctor must fail: %v", err)
	}
	for _, want := range []string{"Config", "file", "Kiro", "✓ usage", "Kiro Pro", "Cursor", "✗ usage"} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "kiro-token") {
		t.Fatal("doctor must never print secrets")
	}
}

func TestInitAndConfigCommands(t *testing.T) {
	isolate(t)
	out, _, err := run(t, "init")
	if err != nil {
		t.Fatal(err)
	}
	path := config.DefaultPath()
	if _, err := os.Stat(path); err != nil || !strings.Contains(out, "Created") {
		t.Fatalf("init did not create %s: %v %q", path, err, out)
	}
	if _, _, err := run(t, "init"); err == nil {
		t.Error("init must not overwrite without --force")
	}
	if out, _, err := run(t, "config", "path"); err != nil || strings.TrimSpace(out) != path {
		t.Errorf("config path = %q, %v", out, err)
	}
	out, _, err = run(t, "config", "show", "--theme", "nord", "-p", "cursor")
	if err != nil || !strings.Contains(out, `theme = "nord"`) || !strings.Contains(out, `order = ["cursor"]`) {
		t.Errorf("config show:\n%s %v", out, err)
	}
	if out, _, err := run(t, "init", "--stdout"); err != nil || !strings.HasPrefix(out, "# all-usage configuration") {
		t.Errorf("init --stdout = %q, %v", out[:min(len(out), 40)], err)
	}
	if out, _, err := run(t, "themes"); err != nil || !strings.Contains(out, "catppuccin") {
		t.Errorf("themes = %q, %v", out, err)
	}
}

func TestFlagValidation(t *testing.T) {
	isolate(t)
	if _, _, err := run(t, "show", "-p", "copilot"); err == nil || !strings.Contains(err.Error(), "unknown provider") {
		t.Errorf("bad provider: %v", err)
	}
	if _, _, err := run(t, "show", "--theme", "neon"); err == nil || !strings.Contains(err.Error(), "unknown theme") {
		t.Errorf("bad theme: %v", err)
	}
	if _, _, err := run(t, "-r", "soon"); err == nil || !strings.Contains(err.Error(), "--refresh") {
		t.Errorf("bad refresh: %v", err)
	}
	if _, _, err := run(t, "show", "-f", "xml"); err == nil || !strings.Contains(err.Error(), "unknown format") {
		t.Errorf("bad format: %v", err)
	}
	if _, _, err := run(t, "show", "-c", "/nonexistent/config.toml"); err == nil {
		t.Error("missing explicit config must fail")
	}
}
