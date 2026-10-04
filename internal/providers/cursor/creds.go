package cursor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cfardev/all-usage/internal/sys"
)

// Sources of a Cursor login.
const (
	srcIDE = "Cursor IDE"
	srcCLI = "cursor-agent"
	srcEnv = "env"
)

// token is a Cursor session JWT.
type token struct {
	Value      string
	Source     string
	Path       string
	Expiry     time.Time
	Sub        string
	Email      string
	Membership string // stripeMembershipType cached by the IDE
	Err        error
}

func (t *token) expired(now time.Time) bool {
	return !t.Expiry.IsZero() && !t.Expiry.After(now.Add(30*time.Second))
}

func (t *token) usable(now time.Time) bool { return t.Err == nil && t.Value != "" && !t.expired(now) }

func (t *token) label() string {
	if t.Path == "" {
		return "$ALL_USAGE_CURSOR_TOKEN"
	}
	return sys.ShortPath(t.Path)
}

// userID is the WorkOS user id ("user_...") from the JWT subject ("github|user_...").
func (t *token) userID() string {
	if i := strings.LastIndex(t.Sub, "|"); i >= 0 {
		return t.Sub[i+1:]
	}
	return t.Sub
}

func (p *Provider) stateDBPaths() []string {
	if p.cfg.StateDB != "" {
		return []string{sys.ExpandPath(p.cfg.StateDB)}
	}
	var paths []string
	for _, d := range sys.AppDataDirs() {
		paths = append(paths, filepath.Join(d, "Cursor", "User", "globalStorage", "state.vscdb"))
	}
	for _, w := range p.env.WindowsHomes {
		paths = append(paths, filepath.Join(w, "AppData", "Roaming", "Cursor", "User", "globalStorage", "state.vscdb"))
	}
	return paths
}

func (p *Provider) cliAuthPaths() []string {
	if p.cfg.AuthFile != "" {
		return []string{sys.ExpandPath(p.cfg.AuthFile)}
	}
	return []string{filepath.Join(sys.XDGConfigHome(), "cursor", "auth.json")}
}

// tokens returns every discovered login, best candidate first.
func (p *Provider) tokens(ctx context.Context) []*token {
	var out []*token
	if v := os.Getenv("ALL_USAGE_CURSOR_TOKEN"); v != "" {
		t := &token{Value: v, Source: srcEnv}
		fillClaims(t)
		out = append(out, t)
	}
	var found []*token
	seen := map[string]bool{}
	add := func(path string, read func(string) *token) {
		real, err := filepath.EvalSymlinks(path)
		if err != nil || seen[real] {
			return
		}
		seen[real] = true
		found = append(found, read(path))
	}
	if p.cfg.Source != "cli" {
		for _, path := range p.stateDBPaths() {
			add(path, func(path string) *token { return readIDEToken(ctx, path) })
		}
	}
	if p.cfg.Source != "ide" {
		for _, path := range p.cliAuthPaths() {
			add(path, readCLIToken)
		}
	}
	now := p.env.Time()
	sort.SliceStable(found, func(i, j int) bool {
		a, b := found[i], found[j]
		if a.usable(now) != b.usable(now) {
			return a.usable(now)
		}
		return a.Expiry.After(b.Expiry)
	})
	return append(out, found...)
}

const (
	keyAccessToken = "cursorAuth/accessToken"
	keyEmail       = "cursorAuth/cachedEmail"
	keyMembership  = "cursorAuth/stripeMembershipType"
)

func readIDEToken(ctx context.Context, path string) *token {
	t := &token{Source: srcIDE, Path: path}
	kv, err := sys.ReadKV(ctx, path, "ItemTable", keyAccessToken, keyEmail, keyMembership)
	if err != nil {
		t.Err = err
		return t
	}
	t.Value = unquote(kv[keyAccessToken])
	t.Email = unquote(kv[keyEmail])
	t.Membership = unquote(kv[keyMembership])
	if t.Value == "" {
		t.Err = errors.New("not signed in")
		return t
	}
	fillClaims(t)
	return t
}

func readCLIToken(path string) *token {
	t := &token{Source: srcCLI, Path: path}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Err = err
		return t
	}
	var f struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Err = fmt.Errorf("invalid JSON: %w", err)
		return t
	}
	if f.AccessToken == "" {
		t.Err = errors.New("not signed in")
		return t
	}
	t.Value = f.AccessToken
	fillClaims(t)
	return t
}

func fillClaims(t *token) {
	claims, err := sys.JWTClaims(t.Value)
	if err != nil {
		return
	}
	t.Sub, _ = claims["sub"].(string)
	if exp, ok := claims["exp"].(float64); ok && exp > 0 {
		t.Expiry = time.Unix(int64(exp), 0)
	}
}

// unquote strips JSON string quoting that some storage versions add.
func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' {
		var out string
		if json.Unmarshal([]byte(s), &out) == nil {
			return out
		}
	}
	return s
}
