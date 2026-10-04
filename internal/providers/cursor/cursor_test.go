package cursor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cfardev/all-usage/internal/config"
	"github.com/cfardev/all-usage/internal/core"
	"github.com/cfardev/all-usage/internal/httpx"
	"github.com/cfardev/all-usage/internal/testutil"
)

var now = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

// Real response shapes (values anonymized).
const summaryJSON = `{
  "billingCycleStart": "2026-10-01T14:54:50.000Z", "billingCycleEnd": "2026-11-01T14:54:50.000Z",
  "membershipType": "pro_plus", "limitType": "user", "isUnlimited": false,
  "individualUsage": {
    "plan": {"enabled": true, "used": 7000, "limit": 7000, "remaining": 0,
      "breakdown": {"included": 7000, "bonus": 7866, "total": 14866},
      "autoPercentUsed": 11.94, "apiPercentUsed": 19.42, "totalPercentUsed": 12.11},
    "onDemand": {"enabled": true, "used": 1250, "limit": 5000, "remaining": 3750}
  },
  "teamUsage": {}
}`

const periodJSON = `{
  "billingCycleStart": "1790866490000", "billingCycleEnd": "1793544890000",
  "planUsage": {"totalSpend": 14866, "includedSpend": 7000, "bonusSpend": 7866, "limit": 7000,
    "autoPercentUsed": 11.94, "apiPercentUsed": 19.42, "totalPercentUsed": 12.11},
  "spendLimitUsage": {"limitType": "user"}, "enabled": true
}`

const planJSON = `{"planInfo": {"planName": "Pro+", "price": "$60/mo", "billingCycleEnd": "1793544890000"}}`

const legacyJSON = `{"gpt-4": {"numRequests": 0, "numRequestsTotal": 0, "numTokens": 0, "maxTokenUsage": null, "maxRequestUsage": null},
  "startOfMonth": "2026-10-01T14:54:50.000Z"}`

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

func sessionJWT(exp time.Time) string {
	return testutil.JWT(map[string]any{"sub": "github|user_01TESTUSER", "exp": exp.Unix(), "type": "session"})
}

type fakeCursor struct {
	token          string
	summaryStatus  int
	legacy         string
	gotCookie      string
	periodRequests int
}

func (f *fakeCursor) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bearerOK := r.Header.Get("Authorization") == "Bearer "+f.token
		switch r.URL.Path {
		case "/api/usage-summary":
			f.gotCookie = r.Header.Get("Cookie")
			if f.summaryStatus != 0 {
				w.WriteHeader(f.summaryStatus)
				_, _ = w.Write([]byte(`{"error":"not_authenticated"}`))
				return
			}
			if f.gotCookie != "WorkosCursorSessionToken=user_01TESTUSER%3A%3A"+f.token {
				w.WriteHeader(401)
				return
			}
			_, _ = w.Write([]byte(summaryJSON))
		case "/aiserver.v1.DashboardService/GetCurrentPeriodUsage":
			f.periodRequests++
			if !bearerOK || r.Method != http.MethodPost {
				w.WriteHeader(401)
				_, _ = w.Write([]byte(`{"code":"unauthenticated","message":"Error"}`))
				return
			}
			_, _ = w.Write([]byte(periodJSON))
		case "/aiserver.v1.DashboardService/GetPlanInfo":
			if !bearerOK {
				w.WriteHeader(401)
				return
			}
			_, _ = w.Write([]byte(planJSON))
		case "/auth/usage":
			if !bearerOK {
				w.WriteHeader(401)
				return
			}
			_, _ = w.Write([]byte(f.legacy))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newProvider(t *testing.T, cfg config.CursorConfig, base string) *Provider {
	t.Helper()
	os.Unsetenv("ALL_USAGE_CURSOR_TOKEN")
	cfg.APIBase, cfg.WebBase = base, base
	if cfg.Source == "" {
		cfg.Source = "auto"
	}
	if cfg.DisplayName == "" {
		cfg.DisplayName = "Cursor"
	}
	if cfg.StateDB == "" {
		cfg.StateDB = filepath.Join(t.TempDir(), "missing.vscdb")
	}
	if cfg.AuthFile == "" {
		cfg.AuthFile = filepath.Join(t.TempDir(), "missing.json")
	}
	return New(cfg, &core.Env{HTTP: httpx.New(5*time.Second, "test"), Now: func() time.Time { return now }})
}

func ideDB(t *testing.T, token string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "state.vscdb")
	testutil.KVDB(t, p, map[string]map[string]string{"ItemTable": {
		keyAccessToken: token,
		keyEmail:       "dev@example.com",
		keyMembership:  "pro_plus",
		"other/key":    "{}",
	}})
	return p
}

func TestFetchFromIDE(t *testing.T) {
	tok := sessionJWT(now.Add(24 * time.Hour))
	f := &fakeCursor{token: tok, legacy: legacyJSON}
	srv := f.server(t)
	p := newProvider(t, config.CursorConfig{StateDB: ideDB(t, tok)}, srv.URL)
	p.env.ShowAccount = true
	u, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if u.Plan != "Pro+" || u.Account != "dev@example.com" || u.Source != "Cursor IDE login" {
		t.Errorf("usage = %+v", u)
	}
	total := meterByID(t, u, "total")
	if total.Pct() != 12.11 || !total.Headline || total.Detail != "$148.66 spent · incl. $78.66 bonus" {
		t.Errorf("total = %+v", total)
	}
	if total.ResetsAt == nil || total.ResetsAt.Format(time.RFC3339) != "2026-11-01T14:54:50Z" {
		t.Errorf("reset = %v", total.ResetsAt)
	}
	if m := meterByID(t, u, "auto"); m.Pct() != 11.94 || m.Headline {
		t.Errorf("auto = %+v", m)
	}
	od := meterByID(t, u, "on_demand")
	if *od.Used != 12.5 || *od.Limit != 50 || od.Unit != "usd" || od.Pct() != 25 {
		t.Errorf("on-demand = %+v", od)
	}
	for _, m := range u.Meters {
		if m.ID == "requests" {
			t.Error("no request meter expected when maxRequestUsage is null")
		}
	}
}

func TestFallbackToRPCWhenCookieRejected(t *testing.T) {
	tok := sessionJWT(now.Add(24 * time.Hour))
	f := &fakeCursor{token: tok, summaryStatus: 401, legacy: legacyJSON}
	srv := f.server(t)
	p := newProvider(t, config.CursorConfig{StateDB: ideDB(t, tok)}, srv.URL)
	u, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if f.periodRequests != 1 || meterByID(t, u, "api").Pct() != 19.42 {
		t.Fatalf("usage = %+v", u)
	}
	if total := meterByID(t, u, "total"); total.ResetsAt == nil || total.ResetsAt.UnixMilli() != 1793544890000 {
		t.Errorf("reset = %v", total.ResetsAt)
	}
}

func TestLegacyRequestPlan(t *testing.T) {
	tok := sessionJWT(now.Add(24 * time.Hour))
	f := &fakeCursor{token: tok, summaryStatus: 500, legacy: `{"gpt-4":{"numRequests":125,"maxRequestUsage":500},"startOfMonth":"2026-09-20T00:00:00.000Z"}`}
	srv := f.server(t)
	p := newProvider(t, config.CursorConfig{StateDB: ideDB(t, tok)}, srv.URL)
	u, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	r := meterByID(t, u, "requests")
	if r.Pct() != 25 || r.ResetsAt.Format("2006-01-02") != "2026-10-20" {
		t.Errorf("requests = %+v", r)
	}
}

func TestCLITokenAndExpiry(t *testing.T) {
	dir := t.TempDir()
	auth := filepath.Join(dir, "auth.json")
	testutil.WriteFile(t, auth, `{"accessToken":"`+sessionJWT(now.Add(-48*time.Hour))+`","refreshToken":"r"}`)
	p := newProvider(t, config.CursorConfig{AuthFile: auth, Source: "cli"}, "http://127.0.0.1:1")
	_, err := p.Fetch(context.Background())
	if e := core.AsError(err); e.Kind != core.KindAuth || !strings.Contains(e.Msg, "expired 2d ago") {
		t.Fatalf("err = %v", err)
	}
}

func TestRejectedEverywhere(t *testing.T) {
	tok := sessionJWT(now.Add(24 * time.Hour))
	f := &fakeCursor{token: "different", legacy: legacyJSON}
	srv := f.server(t)
	p := newProvider(t, config.CursorConfig{StateDB: ideDB(t, tok)}, srv.URL)
	_, err := p.Fetch(context.Background())
	if e := core.AsError(err); e.Kind != core.KindAuth || e.Hint == "" {
		t.Fatalf("err = %v", err)
	}
}

func TestNoLogin(t *testing.T) {
	p := newProvider(t, config.CursorConfig{}, "http://127.0.0.1:1")
	_, err := p.Fetch(context.Background())
	if e := core.AsError(err); e.Kind != core.KindNotConfigured {
		t.Fatalf("err = %v", err)
	}
}

func TestTokenHelpers(t *testing.T) {
	tk := &token{Sub: "google-oauth2|user_ABC"}
	if tk.userID() != "user_ABC" {
		t.Error(tk.userID())
	}
	if unquote(`"abc"`) != "abc" || unquote("abc") != "abc" {
		t.Error("unquote")
	}
	for in, want := range map[string]string{"pro_plus": "Pro+", "free": "Free", "custom_tier": "Custom Tier", "": ""} {
		if got := membershipName(in); got != want {
			t.Errorf("membershipName(%q) = %q, want %q", in, got, want)
		}
	}
}
