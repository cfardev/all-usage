package claude

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/cfardev/all-usage/internal/config"
	"github.com/cfardev/all-usage/internal/core"
	"github.com/cfardev/all-usage/internal/httpx"
	"github.com/cfardev/all-usage/internal/testutil"
)

var now = time.Date(2026, 10, 27, 3, 0, 0, 0, time.UTC)

// Real response shape (trimmed codename fields).
const usageJSON = `{
  "five_hour": {"utilization": 2.0, "resets_at": "2026-10-27T07:59:59.655027+00:00", "limit_dollars": null},
  "seven_day": {"utilization": 41.0, "resets_at": "2026-10-29T17:59:59.655043+00:00"},
  "seven_day_oauth_apps": null, "seven_day_opus": null,
  "seven_day_sonnet": {"utilization": 12.5, "resets_at": "2026-10-29T17:59:59+00:00"},
  "extra_usage": {"is_enabled": true, "monthly_limit": 5000, "used_credits": 1250, "utilization": 25.0,
    "currency": null, "decimal_places": null},
  "limits": [{"kind": "session", "percent": 2}]
}`

func credsFile(t *testing.T, token string, exp time.Time) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), ".credentials.json")
	testutil.WriteFile(t, p, fmt.Sprintf(`{"claudeAiOauth": {"accessToken": %q, "refreshToken": "r", "expiresAt": %d,
		"scopes": ["user:inference"], "subscriptionType": "max", "rateLimitTier": "default_claude_max_5x"}}`, token, exp.UnixMilli()))
	return p
}

func newProvider(t *testing.T, base, file string) *Provider {
	t.Helper()
	t.Setenv("ALL_USAGE_CLAUDE_TOKEN", "")
	cfg := config.ClaudeConfig{DisplayName: "Claude", APIBase: base, CredentialsFile: file}
	return New(cfg, &core.Env{HTTP: httpx.New(5*time.Second, "test"), Now: func() time.Time { return now }})
}

func server(t *testing.T, token string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/oauth/usage" || r.Header.Get("anthropic-beta") != "oauth-2025-04-20" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"authentication_error"}}`))
			return
		}
		_, _ = w.Write([]byte(usageJSON))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFetch(t *testing.T) {
	srv := server(t, "tok")
	u, err := newProvider(t, srv.URL, credsFile(t, "tok", now.Add(time.Hour))).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if u.Plan != "Max 5x" || u.Provider != "claude" || u.Source != "Claude Code login" {
		t.Errorf("plan=%q provider=%q source=%q", u.Plan, u.Provider, u.Source)
	}
	var ids []string
	for _, m := range u.Meters {
		ids = append(ids, m.ID)
	}
	if fmt.Sprint(ids) != "[five_hour seven_day seven_day_sonnet extra_usage]" {
		t.Fatalf("meters = %v", ids)
	}
	fh, wk, x := u.Meters[0], u.Meters[1], u.Meters[3]
	if fh.Pct() != 2 || !fh.Headline || fh.Short != "5h" || fh.WindowSeconds != 18000 || fh.ResetsAt == nil || fh.ResetsAt.Hour() != 7 {
		t.Errorf("five_hour = %+v", fh)
	}
	if wk.Pct() != 41 || !wk.Headline || wk.Short != "wk" {
		t.Errorf("seven_day = %+v", wk)
	}
	if x.Pct() != 25 || *x.Used != 12.5 || *x.Limit != 50 || x.Unit != "usd" || x.Headline {
		t.Errorf("extra_usage = %+v", x)
	}
}

func TestExpiredLogin(t *testing.T) {
	srv := server(t, "tok")
	_, err := newProvider(t, srv.URL, credsFile(t, "tok", now.Add(-2*time.Hour))).Fetch(context.Background())
	if !core.IsAuth(err) {
		t.Fatalf("expected auth error, got %v", err)
	}
}

func TestRejectedLogin(t *testing.T) {
	srv := server(t, "other")
	_, err := newProvider(t, srv.URL, credsFile(t, "tok", now.Add(time.Hour))).Fetch(context.Background())
	if !core.IsAuth(err) {
		t.Fatalf("expected auth error, got %v", err)
	}
}

func TestNoLogin(t *testing.T) {
	_, err := newProvider(t, "http://unused", filepath.Join(t.TempDir(), "missing.json")).Fetch(context.Background())
	if e := core.AsError(err); e == nil || e.Kind != core.KindNotConfigured {
		t.Fatalf("expected not-configured error, got %v", err)
	}
}

func TestEnvToken(t *testing.T) {
	srv := server(t, "envtok")
	p := newProvider(t, srv.URL, filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("ALL_USAGE_CLAUDE_TOKEN", "envtok")
	u, err := p.Fetch(context.Background())
	if err != nil || u.Source != "env token" {
		t.Fatalf("u=%+v err=%v", u, err)
	}
}
