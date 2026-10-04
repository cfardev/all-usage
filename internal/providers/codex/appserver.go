package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/cfardev/all-usage/internal/buildinfo"
	"github.com/cfardev/all-usage/internal/core"
	"github.com/cfardev/all-usage/internal/jsonx"
	"github.com/cfardev/all-usage/internal/sys"
)

// Types of `codex app-server` responses (codex-rs/app-server-protocol v2).
type rpcWindow struct {
	UsedPercent        float64      `json:"usedPercent"`
	WindowDurationMins jsonx.Number `json:"windowDurationMins"`
	ResetsAt           jsonx.Number `json:"resetsAt"`
}

type rpcSnapshot struct {
	LimitID   jsonx.String `json:"limitId"`
	LimitName jsonx.String `json:"limitName"`
	Primary   *rpcWindow   `json:"primary"`
	Secondary *rpcWindow   `json:"secondary"`
	Credits   *struct {
		HasCredits bool         `json:"hasCredits"`
		Unlimited  bool         `json:"unlimited"`
		Balance    jsonx.String `json:"balance"`
	} `json:"credits"`
	PlanType jsonx.String    `json:"planType"`
	Reached  json.RawMessage `json:"rateLimitReachedType"`
}

type rpcRateLimits struct {
	RateLimits *rpcSnapshot            `json:"rateLimits"`
	ByLimitID  map[string]*rpcSnapshot `json:"rateLimitsByLimitId"`
}

type rpcAccount struct {
	Account *struct {
		Type     string `json:"type"`
		Email    string `json:"email"`
		PlanType string `json:"planType"`
	} `json:"account"`
	RequiresOpenaiAuth bool `json:"requiresOpenaiAuth"`
}

func (w *rpcWindow) toWindow() *window {
	if w == nil {
		return nil
	}
	out := &window{UsedPercent: w.UsedPercent, Length: time.Duration(w.WindowDurationMins.Or(0)) * time.Minute}
	if w.ResetsAt.Or(0) > 0 {
		out.ResetsAt = jsonx.UnixTime(w.ResetsAt.V)
	}
	return out
}

func parseRateLimits(raw json.RawMessage) (*snapshot, error) {
	var r rpcRateLimits
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	if r.RateLimits == nil {
		return nil, errors.New("no rateLimits in response")
	}
	m := r.RateLimits
	s := &snapshot{Plan: m.PlanType.V, Reached: parseReached(m.Reached)}
	if m.Credits != nil {
		s.Credits = &credits{HasCredits: m.Credits.HasCredits, Unlimited: m.Credits.Unlimited, Balance: m.Credits.Balance.V}
	}
	mainID := m.LimitID.V
	if mainID == "" {
		mainID = "codex"
	}
	s.Groups = append(s.Groups, limitGroup{ID: mainID, Primary: m.Primary.toWindow(), Secondary: m.Secondary.toWindow()})
	ids := make([]string, 0, len(r.ByLimitID))
	for id := range r.ByLimitID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		o := r.ByLimitID[id]
		if o == nil || id == mainID {
			continue
		}
		g := limitGroup{ID: id, Name: o.LimitName.V, Primary: o.Primary.toWindow(), Secondary: o.Secondary.toWindow()}
		if g.Name == "" {
			g.Name = id
		}
		if g.Primary != nil || g.Secondary != nil {
			s.Groups = append(s.Groups, g)
		}
	}
	return s, nil
}

// rpcConn speaks the app-server's newline-delimited JSON-RPC over stdio.
type rpcConn struct {
	w  io.Writer
	sc *bufio.Scanner
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("app-server error %d: %s", e.Code, e.Message) }

func (c *rpcConn) send(msg map[string]any) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = c.w.Write(append(b, '\n'))
	return err
}

func (c *rpcConn) call(id int, method string, params any) (json.RawMessage, error) {
	msg := map[string]any{"id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	if err := c.send(msg); err != nil {
		return nil, err
	}
	for c.sc.Scan() {
		var resp struct {
			ID     *json.RawMessage `json:"id"`
			Method string           `json:"method"`
			Result json.RawMessage  `json:"result"`
			Error  *rpcError        `json:"error"`
		}
		if json.Unmarshal(c.sc.Bytes(), &resp) != nil || resp.ID == nil || resp.Method != "" {
			continue // notification, server request or noise
		}
		if strings.TrimSpace(string(*resp.ID)) != fmt.Sprint(id) {
			continue
		}
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	}
	if err := c.sc.Err(); err != nil {
		return nil, err
	}
	return nil, io.ErrUnexpectedEOF
}

// tailBuffer keeps the last bytes written to it (for stderr diagnostics).
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > 4096 {
		t.buf = t.buf[len(t.buf)-4096:]
	}
	return len(p), nil
}

// lastLine returns the last non-empty line written, without ANSI escapes.
func (t *tailBuffer) lastLine() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	lines := strings.Split(ansi.Strip(string(t.buf)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			return s
		}
	}
	return ""
}

// fetchAppServer asks the official `codex app-server` for rate limits. Codex
// handles its own credentials there (including keyring storage and refreshing
// expired logins), so this works whenever `codex` itself is signed in.
func (p *Provider) fetchAppServer(ctx context.Context, bin string) (*snapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "app-server")
	cmd.Dir = sys.Home()
	cmd.Env = os.Environ()
	if p.cfg.Home != "" {
		cmd.Env = append(cmd.Env, "CODEX_HOME="+sys.ExpandPath(p.cfg.Home))
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr := &tailBuffer{}
	cmd.Stderr = stderr
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Start(); err != nil {
		return nil, core.Wrap(core.KindNotConfigured, err, "Install Codex or set `codex.binary` in the config.", "cannot start %s app-server", bin)
	}
	defer func() {
		_ = stdin.Close() // EOF asks the server to exit
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}()

	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64<<10), 16<<20)
	rpc := &rpcConn{w: stdin, sc: sc}
	fail := func(step string, err error) error {
		if ctx.Err() != nil {
			return core.NewError(core.KindNetwork, "", "codex app-server timed out (%s)", step)
		}
		var re *rpcError
		if !errors.As(err, &re) {
			if last := stderr.lastLine(); last != "" {
				return core.Wrap(core.KindInternal, err, "Run `codex app-server` manually to see the full error.",
					"codex app-server failed during %s: %s", step, truncate(last, 160))
			}
		}
		return appServerError(step, err)
	}

	initParams := map[string]any{
		"clientInfo":   map[string]any{"name": "all-usage", "title": "all-usage", "version": buildinfo.String()},
		"capabilities": nil,
	}
	if _, err := rpc.call(1, "initialize", initParams); err != nil {
		return nil, fail("initialize", err)
	}
	if err := rpc.send(map[string]any{"method": "initialized"}); err != nil {
		return nil, fail("initialized", err)
	}
	raw, err := rpc.call(2, "account/read", map[string]any{"refreshToken": false})
	if err != nil {
		return nil, fail("account/read", err)
	}
	var acct rpcAccount
	_ = json.Unmarshal(raw, &acct)
	switch {
	case acct.Account == nil:
		return nil, core.NewError(core.KindNotConfigured, loginHint, "Codex is not signed in")
	case acct.Account.Type == "apiKey":
		return nil, apiKeyError()
	}
	raw, err = rpc.call(3, "account/rateLimits/read", nil)
	if err != nil {
		return nil, fail("account/rateLimits/read", err)
	}
	snap, err := parseRateLimits(raw)
	if err != nil {
		return nil, core.Wrap(core.KindParse, err, "", "unexpected app-server response")
	}
	snap.Source = "codex app-server"
	if snap.Plan == "" {
		snap.Plan = acct.Account.PlanType
	}
	snap.Email = acct.Account.Email
	return snap, nil
}

func appServerError(step string, err error) error {
	var re *rpcError
	if errors.As(err, &re) {
		low := strings.ToLower(re.Message)
		if strings.Contains(low, "401") || strings.Contains(low, "403") || strings.Contains(low, "unauthorized") ||
			strings.Contains(low, "sign in") || strings.Contains(low, "log in") || strings.Contains(low, "refresh") {
			return core.NewError(core.KindAuth, loginHint, "Codex login expired (codex app-server could not refresh it)")
		}
		return core.NewError(core.KindAPI, "", "codex app-server: %s", truncate(re.Message, 140))
	}
	return core.Wrap(core.KindInternal, err, "", "codex app-server failed during %s", step)
}

func apiKeyError() *core.Error {
	return core.NewError(core.KindUnsupported, "Usage limits exist only for ChatGPT sign-in: run `codex login`.",
		"Codex is using an API key")
}

func truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
