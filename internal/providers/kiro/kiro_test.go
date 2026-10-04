package kiro

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// Real GetUsageLimits response shape (values anonymized).
const usageJSON = `{
  "daysUntilReset": 0, "limits": [], "nextDateReset": 1793491200.0,
  "overageConfiguration": {"overageLimit": null, "overageStatus": "DISABLED"},
  "subscriptionInfo": {"overageCapability": "OVERAGE_CAPABLE", "subscriptionTitle": "KIRO PRO", "type": "Q_DEVELOPER_STANDALONE_PRO"},
  "usageBreakdownList": [{
    "bonuses": [], "currency": "USD", "currentOverages": 0, "currentOveragesWithPrecision": 0.0,
    "currentUsage": 416, "currentUsageWithPrecision": 416.11, "displayName": "Credit", "displayNamePlural": "Credits",
    "freeTrialInfo": null, "nextDateReset": 1793491200.0, "overageCap": 10000, "overageCapWithPrecision": 10000.0,
    "overageCharges": 0.0, "overageRate": 0.04, "resourceType": "CREDIT", "unit": "INVOCATIONS",
    "usageLimit": 1000, "usageLimitWithPrecision": 1000.0
  }],
  "userInfo": {"email": null, "userId": "x"}
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

func TestParseUsage(t *testing.T) {
	u, err := parseUsage([]byte(usageJSON), now)
	if err != nil {
		t.Fatal(err)
	}
	if u.Plan != "Kiro Pro" || len(u.Meters) != 1 {
		t.Fatalf("usage = %+v", u)
	}
	m := u.Meters[0]
	if m.ID != "credit" || m.Label != "Credits" || m.Unit != "credits" || *m.Used != 416.11 || *m.Limit != 1000 || !m.Headline {
		t.Errorf("meter = %+v", m)
	}
	if got := m.Pct(); got < 41.61 || got > 41.62 {
		t.Errorf("percent = %v", got)
	}
	if m.ResetsAt == nil || m.ResetsAt.Unix() != 1793491200 {
		t.Errorf("reset = %v", m.ResetsAt)
	}
}

func TestParseUsageOverageBonusTrial(t *testing.T) {
	body := `{"nextDateReset": 1793491200, "subscriptionInfo": {"subscriptionTitle": "KIRO PRO+"},
	  "overageConfiguration": {"overageStatus": "ENABLED"},
	  "usageBreakdownList": [{"resourceType": "CREDIT", "displayName": "Credit", "displayNamePlural": "Credits", "currency": "USD",
	    "currentUsageWithPrecision": 2150.5, "usageLimitWithPrecision": 2000, "currentOveragesWithPrecision": 100.5,
	    "overageCapWithPrecision": 1000, "overageCharges": 4.02, "overageRate": 0.04,
	    "bonuses": [{"bonusCode": "B1", "displayName": "Welcome bonus", "status": "ACTIVE", "expiresAt": 1791900000,
	                 "currentUsage": 50, "usageLimit": 500}],
	    "freeTrialInfo": {"freeTrialStatus": "ACTIVE", "freeTrialExpiry": 1791000000, "currentUsageWithPrecision": 10, "usageLimitWithPrecision": 100}}]}`
	u, err := parseUsage([]byte(body), now)
	if err != nil {
		t.Fatal(err)
	}
	plan := meterByID(t, u, "credit")
	if *plan.Used != 2000 { // 2150.5 total - 100.5 overage - 50 bonus
		t.Errorf("plan used = %v", *plan.Used)
	}
	bonus := meterByID(t, u, "credit.bonus")
	if bonus.Label != "Welcome bonus" || bonus.Pct() != 10 || !strings.HasPrefix(bonus.Detail, "expires in") {
		t.Errorf("bonus = %+v", bonus)
	}
	if ft := meterByID(t, u, "credit.free_trial"); ft.Pct() != 10 {
		t.Errorf("free trial = %+v", ft)
	}
	over := meterByID(t, u, "credit.overage")
	if *over.Used != 100.5 || *over.Limit != 1000 || over.Detail != "$4.02 billed at $0.04/credit" {
		t.Errorf("overage = %+v", over)
	}
}

func TestParseUsageErrors(t *testing.T) {
	if _, err := parseUsage([]byte(`{"usageBreakdownList": []}`), now); err == nil {
		t.Error("expected error for empty breakdown")
	}
	if _, err := parseUsage([]byte(`nope`), now); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestEndpointAndRegion(t *testing.T) {
	if got := endpointFor("eu-central-1"); got != "https://q.eu-central-1.amazonaws.com" {
		t.Error(got)
	}
	if got := endpointFor("ap-south-1"); got != "https://q.us-east-1.amazonaws.com" {
		t.Error(got)
	}
	if got := regionFromARN("arn:aws:codewhisperer:eu-central-1:123456789012:profile/ABC"); got != "eu-central-1" {
		t.Error(got)
	}
	if got := regionFromARN("not-an-arn"); got != "" {
		t.Error(got)
	}
	if got := profileARN(`{"arn":"arn:aws:x:us-east-1:1:profile/p","profile_name":"P"}`); got != "arn:aws:x:us-east-1:1:profile/p" {
		t.Error(got)
	}
	if got := profileARN(`"arn:plain"`); got != "arn:plain" {
		t.Error(got)
	}
}

const testARN = "arn:aws:codewhisperer:us-east-1:123456789012:profile/TESTPROFILE"

func cliToken(access string, exp time.Time) string {
	return `{"access_token":"` + access + `","expires_at":"` + exp.Format(time.RFC3339Nano) +
		`","refresh_token":"r","region":"us-east-1","start_url":"https://d-1234567890.awsapps.com/start","oauth_flow":"DeviceCode","scopes":["codewhisperer:completions"]}`
}

func writeDB(t *testing.T, path, tokenKey, tokenJSON string) {
	t.Helper()
	testutil.KVDB(t, path, map[string]map[string]string{
		"auth_kv": {tokenKey: tokenJSON},
		"state":   {profileKey: `{"arn":"` + testARN + `","profile_name":"KiroProfile-us-east-1"}`},
	})
}

func newProvider(t *testing.T, cfg config.KiroConfig) *Provider {
	t.Helper()
	os.Unsetenv("ALL_USAGE_KIRO_TOKEN")
	if cfg.Source == "" {
		cfg.Source = "auto"
	}
	if cfg.DisplayName == "" {
		cfg.DisplayName = "Kiro"
	}
	if cfg.IDETokenFile == "" {
		cfg.IDETokenFile = filepath.Join(t.TempDir(), "missing.json")
	}
	return New(cfg, &core.Env{HTTP: httpx.New(5*time.Second, "test"), Now: func() time.Time { return now }})
}

func usageServer(t *testing.T, wantToken string, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		q := r.URL.Query()
		if r.URL.Path != "/getUsageLimits" || q.Get("origin") != "AI_EDITOR" || q.Get("profileArn") != testARN {
			http.Error(w, "bad request "+r.URL.String(), 400)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+wantToken {
			w.WriteHeader(403)
			_, _ = w.Write([]byte(`{"message":"The bearer token included in the request is invalid.","reason":null}`))
			return
		}
		_, _ = w.Write([]byte(usageJSON))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchWithCLIToken(t *testing.T) {
	db := filepath.Join(t.TempDir(), "data.sqlite3")
	writeDB(t, db, "kirocli:odic:token", cliToken("tok-1", now.Add(time.Hour)))
	var hits atomic.Int32
	srv := usageServer(t, "tok-1", &hits)

	p := newProvider(t, config.KiroConfig{DBPath: db, Endpoint: srv.URL})
	u, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if u.Source != "kiro-cli login" || u.Plan != "Kiro Pro" || meterByID(t, u, "credit").Unit != "credits" {
		t.Fatalf("usage = %+v", u)
	}
}

func TestFetchSocialTokenWithCamelCase(t *testing.T) {
	db := filepath.Join(t.TempDir(), "data.sqlite3")
	testutil.KVDB(t, db, map[string]map[string]string{"auth_kv": {
		"kirocli:social:token": `{"accessToken":"tok-s","expiresAt":"` + now.Add(time.Hour).Format(time.RFC3339) + `","profileArn":"` + testARN + `","provider":"GitHub"}`,
	}})
	var hits atomic.Int32
	srv := usageServer(t, "tok-s", &hits)
	p := newProvider(t, config.KiroConfig{DBPath: db, Endpoint: srv.URL})
	if _, err := p.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	toks := p.tokens(context.Background())
	if toks[0].Kind != "GitHub" {
		t.Errorf("kind = %q", toks[0].Kind)
	}
}

func TestRejectedToken(t *testing.T) {
	db := filepath.Join(t.TempDir(), "data.sqlite3")
	writeDB(t, db, "kirocli:odic:token", cliToken("stale", now.Add(time.Hour)))
	var hits atomic.Int32
	srv := usageServer(t, "other", &hits)
	p := newProvider(t, config.KiroConfig{DBPath: db, Endpoint: srv.URL, RefreshWithCLI: false})
	_, err := p.Fetch(context.Background())
	if e := core.AsError(err); e == nil || e.Kind != core.KindAuth || e.Hint == "" {
		t.Fatalf("err = %v", err)
	}
}

func TestExpiredTokenRefreshedByCLI(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "data.sqlite3")
	writeDB(t, db, "kirocli:odic:token", cliToken("old", now.Add(-time.Hour)))
	// A fake kiro-cli whose `whoami` writes a fresh token (as the real one does).
	fresh := filepath.Join(dir, "fresh.sqlite3")
	writeDB(t, fresh, "kirocli:odic:token", cliToken("new", now.Add(time.Hour)))
	bin := testutil.Script(t, dir, "kiro-cli", `[ "$1" = "whoami" ] && cp "`+fresh+`" "`+db+`"; echo '{}'`)

	var hits atomic.Int32
	srv := usageServer(t, "new", &hits)
	p := newProvider(t, config.KiroConfig{DBPath: db, Endpoint: srv.URL, RefreshWithCLI: true, Binary: bin})
	u, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if meterByID(t, u, "credit").Pct() == 0 {
		t.Fatal("expected usage after refresh")
	}
}

func TestExpiredTokenWithoutRefresh(t *testing.T) {
	db := filepath.Join(t.TempDir(), "data.sqlite3")
	writeDB(t, db, "kirocli:odic:token", cliToken("old", now.Add(-3*time.Hour)))
	dir := t.TempDir()
	counter := filepath.Join(dir, "runs")
	bin := testutil.Script(t, dir, "kiro-cli", `echo run >> "`+counter+`"`) // refresh does nothing
	p := newProvider(t, config.KiroConfig{DBPath: db, RefreshWithCLI: true, Binary: bin})
	for range 2 {
		_, err := p.Fetch(context.Background())
		if e := core.AsError(err); e.Kind != core.KindAuth || !strings.Contains(e.Msg, "expired 3h ago") {
			t.Fatalf("err = %v", err)
		}
	}
	b, _ := os.ReadFile(counter)
	if n := strings.Count(string(b), "run"); n != 1 {
		t.Errorf("kiro-cli ran %d times; a failed refresh must not be retried until the login changes", n)
	}
}

func TestIDETokenFallback(t *testing.T) {
	ide := filepath.Join(t.TempDir(), "kiro-auth-token.json")
	testutil.WriteFile(t, ide, `{"accessToken":"ide-tok","refreshToken":"r","expiresAt":"`+now.Add(time.Hour).Format(time.RFC3339)+
		`","authMethod":"social","provider":"Google","profileArn":"`+testARN+`","region":"us-east-1"}`)
	var hits atomic.Int32
	srv := usageServer(t, "ide-tok", &hits)
	p := newProvider(t, config.KiroConfig{DBPath: filepath.Join(t.TempDir(), "none.sqlite3"), IDETokenFile: ide, Endpoint: srv.URL})
	u, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if u.Source != "Kiro IDE login" {
		t.Errorf("source = %q", u.Source)
	}
}

func TestNoLogin(t *testing.T) {
	p := newProvider(t, config.KiroConfig{DBPath: filepath.Join(t.TempDir(), "none.sqlite3")})
	_, err := p.Fetch(context.Background())
	if e := core.AsError(err); e.Kind != core.KindNotConfigured {
		t.Fatalf("err = %v", err)
	}
}
