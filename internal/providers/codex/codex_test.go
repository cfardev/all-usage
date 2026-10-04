package codex

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cfardev/all-usage/internal/config"
	"github.com/cfardev/all-usage/internal/core"
	"github.com/cfardev/all-usage/internal/httpx"
	"github.com/cfardev/all-usage/internal/testutil"
)

var now = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

const whamJSON = `{
  "plan_type": "team",
  "rate_limit": {
    "allowed": true, "limit_reached": false,
    "primary_window":   {"used_percent": 22, "limit_window_seconds": 18000,  "reset_after_seconds": 3600, "reset_at": 1791043200},
    "secondary_window": {"used_percent": 56, "limit_window_seconds": 604800, "reset_after_seconds": 100,  "reset_at": 0}
  },
  "credits": {"has_credits": true, "unlimited": false, "balance": "12.5"},
  "additional_rate_limits": [
    {"limit_name": "GPT-5.5 Codex", "metered_feature": "codex_gpt55",
     "rate_limit": {"primary_window": {"used_percent": 10, "limit_window_seconds": 18000, "reset_after_seconds": 60, "reset_at": 1791040000}}}
  ],
  "rate_limit_reached_type": null
}`

func meterByID(t *testing.T, u *core.Usage, id string) core.Meter {
	t.Helper()
	for _, m := range u.Meters {
		if m.ID == id {
			return m
		}
	}
	t.Fatalf("meter %q not found in %+v", id, u.Meters)
	return core.Meter{}
}

func TestParseWham(t *testing.T) {
	s, err := parseWham([]byte(whamJSON), now)
	if err != nil {
		t.Fatal(err)
	}
	u := s.toUsage("codex", "Codex", now, false)
	if u.Plan != "Team" {
		t.Errorf("plan = %q", u.Plan)
	}
	p := meterByID(t, u, "primary")
	if p.Label != "5-hour limit" || p.Short != "5h" || p.Pct() != 22 || !p.Headline || p.ResetsAt.Unix() != 1791043200 {
		t.Errorf("primary = %+v", p)
	}
	w := meterByID(t, u, "secondary")
	if w.Label != "Weekly limit" || w.Pct() != 56 || !w.ResetsAt.Equal(now.Add(100*time.Second)) {
		t.Errorf("secondary = %+v (reset %v)", w, w.ResetsAt)
	}
	extra := meterByID(t, u, "codex_gpt55.primary")
	if extra.Label != "GPT-5.5 Codex · 5-hour" || extra.Headline || extra.Pct() != 10 {
		t.Errorf("additional = %+v", extra)
	}
	if c := meterByID(t, u, "credits"); c.Value != "12.5" || c.HasPercent() {
		t.Errorf("credits = %+v", c)
	}
}

func TestParseWhamRejectsGarbage(t *testing.T) {
	if _, err := parseWham([]byte(`{"plan_type":"free"}`), now); err == nil {
		t.Error("expected error for a payload without limits")
	}
	if _, err := parseWham([]byte(`<html>`), now); err == nil {
		t.Error("expected error for non-JSON")
	}
}

const rpcJSON = `{
  "rateLimits": {"limitId": "codex", "limitName": null,
    "primary": {"usedPercent": 7, "windowDurationMins": 300, "resetsAt": 1790668325},
    "secondary": {"usedPercent": 6, "windowDurationMins": 10080, "resetsAt": 1791062416},
    "credits": {"hasCredits": false, "unlimited": false, "balance": null},
    "planType": "plus", "rateLimitReachedType": "rate_limit_reached"},
  "rateLimitsByLimitId": {
    "codex": {"limitId": "codex", "primary": {"usedPercent": 7, "windowDurationMins": 300, "resetsAt": 1790668325}},
    "codex_other": {"limitId": "codex_other", "limitName": "Other model",
      "primary": {"usedPercent": 3, "windowDurationMins": 300, "resetsAt": 1790668325}, "secondary": null}
  }
}`

func TestParseRateLimitsRPC(t *testing.T) {
	s, err := parseRateLimits([]byte(rpcJSON))
	if err != nil {
		t.Fatal(err)
	}
	u := s.toUsage("codex", "Codex", now, false)
	if u.Plan != "Plus" || len(u.Notes) != 1 || u.Notes[0] != "Rate limit reached" {
		t.Errorf("plan=%q notes=%v", u.Plan, u.Notes)
	}
	if w := meterByID(t, u, "secondary"); w.Label != "Weekly limit" || w.Pct() != 6 {
		t.Errorf("secondary = %+v", w)
	}
	if o := meterByID(t, u, "codex_other.primary"); o.Label != "Other model · 5-hour" {
		t.Errorf("other = %+v", o)
	}
	for _, m := range u.Meters {
		if m.ID == "credits" {
			t.Error("no credits meter expected when the account has none")
		}
	}
}

func authJSON(access string) string {
	return `{"auth_mode":"chatgpt","OPENAI_API_KEY":null,"tokens":{"id_token":"x","access_token":"` + access +
		`","refresh_token":"r","account_id":"acct-1"},"last_refresh":"2026-10-01T00:00:00Z"}`
}

func codexJWT(exp time.Time) string {
	return testutil.JWT(map[string]any{
		"exp":                            exp.Unix(),
		"https://api.openai.com/auth":    map[string]any{"chatgpt_plan_type": "team", "chatgpt_account_id": "acct-1"},
		"https://api.openai.com/profile": map[string]any{"email": "dev@example.com"},
	})
}

func newProvider(t *testing.T, cfg config.CodexConfig) *Provider {
	t.Helper()
	t.Setenv("ALL_USAGE_CODEX_TOKEN", "")
	os.Unsetenv("ALL_USAGE_CODEX_TOKEN")
	d := config.Default().Codex
	if cfg.Source == "" {
		cfg.Source = d.Source
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://127.0.0.1:1"
	}
	if cfg.DisplayName == "" {
		cfg.DisplayName = "Codex"
	}
	env := &core.Env{HTTP: httpx.New(5*time.Second, "test"), Now: func() time.Time { return now }}
	return New(cfg, env)
}

func TestFetchAPI(t *testing.T) {
	home := t.TempDir()
	tok := codexJWT(now.Add(time.Hour))
	testutil.WriteFile(t, filepath.Join(home, "auth.json"), authJSON(tok))

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/backend-api/wham/usage" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+tok || r.Header.Get("ChatGPT-Account-Id") != "acct-1" {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"detail":"bad auth"}`))
			return
		}
		_, _ = w.Write([]byte(whamJSON))
	}))
	defer srv.Close()

	p := newProvider(t, config.CodexConfig{Source: "api", Home: home, BaseURL: srv.URL + "/backend-api"})
	p.env.ShowAccount = true
	u, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(u.Source, " API") {
		t.Errorf("source = %q", u.Source)
	}
	if u.Account != "dev@example.com" || u.Plan != "Team" || len(u.Meters) != 4 {
		t.Errorf("usage = %+v", u)
	}
}

func TestFetchAPIExpiredTokenResponse(t *testing.T) {
	home := t.TempDir()
	testutil.WriteFile(t, filepath.Join(home, "auth.json"), authJSON(codexJWT(now.Add(time.Hour))))
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":{"message":"Provided authentication token is expired.","code":"token_expired"},"status":401}`))
	}))
	defer srv.Close()

	p := newProvider(t, config.CodexConfig{Source: "api", Home: home, BaseURL: srv.URL})
	for range 3 {
		_, err := p.Fetch(context.Background())
		e := core.AsError(err)
		if e == nil || e.Kind != core.KindAuth || !strings.Contains(e.Msg, "expired") {
			t.Fatalf("err = %v", err)
		}
	}
	if hits.Load() != 1 {
		t.Errorf("a rejected login must not be retried until auth.json changes; got %d requests", hits.Load())
	}
}

func TestLocallyExpiredTokenSkipsNetwork(t *testing.T) {
	home := t.TempDir()
	testutil.WriteFile(t, filepath.Join(home, "auth.json"), authJSON(codexJWT(now.Add(-2*time.Hour))))
	p := newProvider(t, config.CodexConfig{Source: "api", Home: home}) // base URL is unreachable
	_, err := p.Fetch(context.Background())
	if e := core.AsError(err); e.Kind != core.KindAuth || !strings.Contains(e.Msg, "expired 2h ago") {
		t.Fatalf("err = %v", err)
	}
}

func TestAPIKeyOnlyLogin(t *testing.T) {
	home := t.TempDir()
	testutil.WriteFile(t, filepath.Join(home, "auth.json"), `{"OPENAI_API_KEY":"sk-test","tokens":null}`)
	p := newProvider(t, config.CodexConfig{Source: "api", Home: home})
	_, err := p.Fetch(context.Background())
	if e := core.AsError(err); e.Kind != core.KindUnsupported {
		t.Fatalf("err = %v", err)
	}
}

func sessionLineJSON(ts, limitID string, primary, secondary float64, primaryReset, secondaryReset int64) string {
	return `{"timestamp":"` + ts + `","type":"event_msg","payload":{"type":"token_count","info":null,"rate_limits":{"limit_id":"` + limitID +
		`","limit_name":null,"primary":{"used_percent":` + ftoa(primary) + `,"window_minutes":300,"resets_at":` + itoa(primaryReset) +
		`},"secondary":{"used_percent":` + ftoa(secondary) + `,"window_minutes":10080,"resets_at":` + itoa(secondaryReset) +
		`},"credits":null,"plan_type":"team","rate_limit_reached_type":null}}}`
}

func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
func itoa(i int64) string   { return strconv.FormatInt(i, 10) }

func writeSession(t *testing.T, home, day string, lines []string, padding int, mod time.Time) string {
	t.Helper()
	path := filepath.Join(home, "sessions", day, "rollout-"+strings.ReplaceAll(day, "/", "-")+".jsonl")
	var b strings.Builder
	b.WriteString(`{"timestamp":"2026-09-26T00:00:00Z","type":"session_meta","payload":{}}` + "\n")
	for _, l := range lines {
		b.WriteString(l + "\n")
	}
	pad := `{"timestamp":"2026-09-30T00:00:00Z","type":"response_item","payload":{"text":"` + strings.Repeat("x", 1000) + `"}}` + "\n"
	for range padding {
		b.WriteString(pad)
	}
	testutil.WriteFile(t, path, b.String())
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSessionFallbackPrefersMainLimitAndHandlesBigFiles(t *testing.T) {
	home := t.TempDir()
	reset5h := now.Add(time.Hour).Unix()
	resetWk := now.Add(48 * time.Hour).Unix()
	lines := []string{
		sessionLineJSON("2026-09-29T03:00:00Z", "codex", 5, 4, reset5h, resetWk),
		sessionLineJSON("2026-09-29T03:04:00Z", "codex", 7, 6, reset5h, resetWk),
		`not json "rate_limits"`,
		sessionLineJSON("2026-09-29T03:05:00Z", "codex_other", 90, 90, reset5h, resetWk),
	}
	// ~3 MB of padding after the snapshot forces multiple backwards chunks.
	writeSession(t, home, "2026/09/26", lines, 3000, now.Add(-time.Hour))
	// An older file with different numbers must be ignored.
	writeSession(t, home, "2026/09/20", []string{sessionLineJSON("2026-09-20T00:00:00Z", "codex", 99, 99, reset5h, resetWk)}, 0, now.Add(-72*time.Hour))

	p := newProvider(t, config.CodexConfig{Source: "sessions", Home: home})
	u, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !u.Stale || u.AsOf == nil || u.AsOf.Format(time.RFC3339) != "2026-09-29T03:04:00Z" {
		t.Fatalf("stale=%v asOf=%v", u.Stale, u.AsOf)
	}
	if m := meterByID(t, u, "primary"); m.Pct() != 7 || m.ResetsAt.Unix() != reset5h {
		t.Errorf("primary = %+v", m)
	}
	if m := meterByID(t, u, "secondary"); m.Pct() != 6 {
		t.Errorf("secondary = %+v", m)
	}
}

func TestSessionSnapshotAfterReset(t *testing.T) {
	home := t.TempDir()
	past := now.Add(-time.Hour).Unix()
	future := now.Add(time.Hour).Unix()
	writeSession(t, home, "2026/10/01", []string{sessionLineJSON("2026-10-01T00:00:00Z", "codex", 80, 30, past, future)}, 0, now.Add(-time.Hour))
	p := newProvider(t, config.CodexConfig{Source: "sessions", Home: home})
	u, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if m := meterByID(t, u, "primary"); m.Pct() != 0 || m.ResetsAt != nil || m.Detail == "" {
		t.Errorf("a window that already reset must show 0%%: %+v", m)
	}
	if m := meterByID(t, u, "secondary"); m.Pct() != 30 {
		t.Errorf("secondary = %+v", m)
	}
}

func TestAutoFallsBackToSessionsWithWarning(t *testing.T) {
	home := t.TempDir()
	testutil.WriteFile(t, filepath.Join(home, "auth.json"), authJSON(codexJWT(now.Add(-time.Hour))))
	writeSession(t, home, "2026/10/02", []string{sessionLineJSON("2026-10-02T00:00:00Z", "codex", 10, 20, now.Add(time.Hour).Unix(), now.Add(time.Hour).Unix())}, 0, now.Add(-time.Hour))
	p := newProvider(t, config.CodexConfig{Source: "auto", Home: home, UseAppServer: false, SessionsFallback: true})
	u, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if u.Warning == nil || u.Warning.Kind != core.KindAuth || !u.Stale {
		t.Fatalf("expected a stale result with an auth warning, got %+v", u)
	}
}

func TestDirTooOld(t *testing.T) {
	cutoff := time.Date(2026, 9, 3, 0, 0, 0, 0, time.Local)
	for _, c := range []struct {
		parts []string
		want  bool
	}{
		{[]string{"2025"}, true}, {[]string{"2026"}, false}, {[]string{"2026", "07"}, true}, {[]string{"2026", "08"}, false},
		{[]string{"2026", "08", "20"}, true}, {[]string{"2026", "08", "28"}, false}, {[]string{"misc"}, false},
	} {
		if got := dirTooOld(c.parts, cutoff); got != c.want {
			t.Errorf("dirTooOld(%v) = %v, want %v", c.parts, got, c.want)
		}
	}
}

const fakeAppServer = `[ "$1" = "app-server" ] || exit 2
while IFS= read -r line; do
  case "$line" in
    *'"method":"initialize"'*) echo '{"id":1,"result":{"userAgent":"fake"}}' ;;
    *'"method":"initialized"'*) echo '{"method":"configWarning","params":{"summary":"noise"}}' ;;
    *'"method":"account/read"'*) echo '{"id":2,"result":{"account":{"type":"chatgpt","email":"a@example.com","planType":"pro"},"requiresOpenaiAuth":true}}' ;;
    *'"method":"account/rateLimits/read"'*) echo 'garbage line'; echo "$RATE_LIMITS_REPLY" ;;
  esac
done
`

func TestAppServer(t *testing.T) {
	bin := testutil.Script(t, t.TempDir(), "codex", fakeAppServer)
	t.Setenv("RATE_LIMITS_REPLY", `{"id":3,"result":`+strings.ReplaceAll(rpcJSON, "\n", "")+`}`)
	p := newProvider(t, config.CodexConfig{Source: "app-server", Binary: bin, Home: t.TempDir()})
	u, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if u.Source != "codex app-server" || u.Plan != "Plus" || meterByID(t, u, "primary").Pct() != 7 {
		t.Fatalf("usage = %+v", u)
	}
}

func TestAppServerAuthErrorIsCached(t *testing.T) {
	dir := t.TempDir()
	counter := filepath.Join(dir, "runs")
	bin := testutil.Script(t, dir, "codex", `echo run >> "`+counter+`"
`+fakeAppServer)
	t.Setenv("RATE_LIMITS_REPLY", `{"id":3,"error":{"code":-32603,"message":"failed to fetch codex rate limits: GET https://chatgpt.com/backend-api/wham/usage failed: 401 Unauthorized"}}`)
	p := newProvider(t, config.CodexConfig{Source: "app-server", Binary: bin, Home: t.TempDir()})
	for range 2 {
		_, err := p.Fetch(context.Background())
		if e := core.AsError(err); e.Kind != core.KindAuth {
			t.Fatalf("err = %v", err)
		}
	}
	b, _ := os.ReadFile(counter)
	if n := strings.Count(string(b), "run"); n != 1 {
		t.Errorf("app-server ran %d times; auth failures should be cached", n)
	}
}

func TestAppServerCrashShowsStderr(t *testing.T) {
	bin := testutil.Script(t, t.TempDir(), "codex", `printf '\033[31mERROR\033[0m config.toml: invalid key\n' >&2; exit 1
`)
	p := newProvider(t, config.CodexConfig{Source: "app-server", Binary: bin, Home: t.TempDir()})
	_, err := p.Fetch(context.Background())
	if e := core.AsError(err); e == nil || !strings.Contains(e.Msg, "ERROR config.toml: invalid key") || e.Hint == "" {
		t.Fatalf("err = %v", err)
	}
}

func TestCredentialsPreferValidAndEnv(t *testing.T) {
	native := t.TempDir()
	testutil.WriteFile(t, filepath.Join(native, "auth.json"), authJSON(codexJWT(now.Add(-time.Hour))))
	// Windows homes are profile dirs containing ".codex".
	win := filepath.Join(t.TempDir(), "winuser")
	testutil.WriteFile(t, filepath.Join(win, ".codex", "auth.json"), authJSON(codexJWT(now.Add(2*time.Hour))))

	p := newProvider(t, config.CodexConfig{})
	t.Setenv("CODEX_HOME", native)
	p.env.WindowsHomes = []string{win}
	creds := p.credentials()
	if len(creds) != 2 || !creds[0].usable(now) || !strings.HasPrefix(creds[0].Path, win) {
		t.Fatalf("expected the valid Windows login first, got %+v", creds)
	}
	t.Setenv("ALL_USAGE_CODEX_TOKEN", "envtoken")
	if creds := p.credentials(); creds[0].Path != "" || creds[0].AccessToken != "envtoken" {
		t.Fatalf("env token must come first: %+v", creds[0])
	}
}
