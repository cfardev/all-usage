package codex

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/cfardev/all-usage/internal/sys"
)

// credential is a ChatGPT login usable against the usage API.
type credential struct {
	Path        string // auth.json path ("" for the env token)
	Home        string
	Native      bool // the home the local `codex` binary uses
	AccessToken string
	AccountID   string
	Expiry      time.Time
	Plan        string
	Email       string
	APIKeyOnly  bool
	ModTime     time.Time
	Err         error
}

func (c credential) expired(now time.Time) bool {
	return !c.Expiry.IsZero() && !c.Expiry.After(now.Add(30*time.Second))
}

func (c credential) usable(now time.Time) bool {
	return c.Err == nil && c.AccessToken != "" && !c.expired(now)
}

func (c credential) label() string {
	if c.Path == "" {
		return "$ALL_USAGE_CODEX_TOKEN"
	}
	return sys.ShortPath(c.Path)
}

type home struct {
	Dir    string
	Native bool
}

// homes lists Codex home directories to inspect.
func (p *Provider) homes() []home {
	if p.cfg.Home != "" {
		return []home{{Dir: sys.ExpandPath(p.cfg.Home), Native: true}}
	}
	native := filepath.Join(sys.Home(), ".codex")
	if v := os.Getenv("CODEX_HOME"); v != "" {
		native = sys.ExpandPath(v)
	}
	hs := []home{{Dir: native, Native: true}}
	for _, w := range p.env.WindowsHomes {
		hs = append(hs, home{Dir: filepath.Join(w, ".codex")})
	}
	return hs
}

// credentials returns known logins, best candidate first: the env token, then
// auth.json files with a valid token (latest expiry first), then the rest.
func (p *Provider) credentials() []credential {
	var env []credential
	if tok := os.Getenv("ALL_USAGE_CODEX_TOKEN"); tok != "" {
		c := credential{AccessToken: tok, AccountID: os.Getenv("ALL_USAGE_CODEX_ACCOUNT_ID")}
		fillFromJWT(&c, tok, "")
		env = append(env, c)
	}
	var files []credential
	seen := map[string]bool{}
	for _, h := range p.homes() {
		path := filepath.Join(h.Dir, "auth.json")
		real, err := filepath.EvalSymlinks(path)
		if err != nil || seen[real] {
			continue
		}
		seen[real] = true
		c := readAuthFile(path)
		c.Home, c.Native = h.Dir, h.Native
		files = append(files, c)
	}
	now := p.env.Time()
	sort.SliceStable(files, func(i, j int) bool {
		a, b := files[i], files[j]
		if a.usable(now) != b.usable(now) {
			return a.usable(now)
		}
		if !a.Expiry.Equal(b.Expiry) {
			return a.Expiry.After(b.Expiry)
		}
		return a.ModTime.After(b.ModTime)
	})
	return append(env, files...)
}

func readAuthFile(path string) credential {
	c := credential{Path: path}
	st, err := os.Stat(path)
	if err != nil {
		c.Err = err
		return c
	}
	c.ModTime = st.ModTime()
	b, err := os.ReadFile(path)
	if err != nil {
		c.Err = err
		return c
	}
	var f struct {
		APIKey *string `json:"OPENAI_API_KEY"`
		Tokens *struct {
			IDToken     string `json:"id_token"`
			AccessToken string `json:"access_token"`
			AccountID   string `json:"account_id"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		c.Err = fmt.Errorf("invalid JSON: %w", err)
		return c
	}
	if f.Tokens == nil || f.Tokens.AccessToken == "" {
		if f.APIKey != nil && *f.APIKey != "" {
			c.APIKeyOnly = true
		} else {
			c.Err = errors.New("no ChatGPT login in file")
		}
		return c
	}
	c.AccessToken = f.Tokens.AccessToken
	c.AccountID = f.Tokens.AccountID
	fillFromJWT(&c, f.Tokens.AccessToken, f.Tokens.IDToken)
	return c
}

// fillFromJWT reads expiry, plan, account and e-mail from the token claims.
func fillFromJWT(c *credential, access, id string) {
	if exp, ok := sys.JWTExpiry(access); ok {
		c.Expiry = exp
	}
	const authNS, profileNS = "https://api.openai.com/auth", "https://api.openai.com/profile"
	for _, tok := range []string{access, id} {
		claims, err := sys.JWTClaims(tok)
		if err != nil {
			continue
		}
		if c.Plan == "" {
			c.Plan = sys.ClaimString(claims, authNS, "chatgpt_plan_type")
		}
		if c.AccountID == "" {
			c.AccountID = sys.ClaimString(claims, authNS, "chatgpt_account_id")
		}
		if c.Email == "" {
			c.Email = sys.ClaimString(claims, profileNS, "email")
		}
		if c.Email == "" {
			c.Email = sys.ClaimString(claims, "email")
		}
	}
}
