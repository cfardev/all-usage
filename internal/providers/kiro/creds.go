package kiro

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

	"github.com/cfardev/all-usage/internal/jsonx"
	"github.com/cfardev/all-usage/internal/sys"
)

// Sources of a Kiro login.
const (
	srcCLI = "kiro-cli"
	srcIDE = "Kiro IDE"
	srcEnv = "env"
)

// token is a Kiro bearer token with the metadata needed to call the API.
type token struct {
	Access     string
	Expiry     time.Time
	Region     string // OIDC region stored with the token
	ProfileARN string
	Source     string
	Path       string
	Kind       string // "IAM Identity Center", "Builder ID", "GitHub"...
	ModTime    time.Time
	Err        error
}

func (t *token) expired(now time.Time) bool {
	// Same safety margin kiro-cli uses before refreshing.
	return !t.Expiry.IsZero() && !t.Expiry.After(now.Add(time.Minute))
}

func (t *token) usable(now time.Time) bool { return t.Err == nil && t.Access != "" && !t.expired(now) }

func (t *token) label() string {
	if t.Path == "" {
		return "$ALL_USAGE_KIRO_TOKEN"
	}
	return sys.ShortPath(t.Path)
}

// kiro-cli keeps its login in auth_kv; key names changed across versions.
var tokenKeys = []string{
	"kirocli:social:token", "kirocli:odic:token", "kirocli:oidc:token",
	"codewhisperer:odic:token", "codewhisperer:oidc:token",
}

const profileKey = "api.codewhisperer.profile"

func (p *Provider) dbPaths() []string {
	if p.cfg.DBPath != "" {
		return []string{sys.ExpandPath(p.cfg.DBPath)}
	}
	paths := []string{filepath.Join(sys.LocalDataDir(), "kiro-cli", "data.sqlite3")}
	for _, w := range p.env.WindowsHomes {
		paths = append(paths, filepath.Join(w, "AppData", "Local", "kiro-cli", "data.sqlite3"))
	}
	return paths
}

func (p *Provider) ideTokenPaths() []string {
	if p.cfg.IDETokenFile != "" {
		return []string{sys.ExpandPath(p.cfg.IDETokenFile)}
	}
	paths := []string{filepath.Join(sys.Home(), ".aws", "sso", "cache", "kiro-auth-token.json")}
	for _, w := range p.env.WindowsHomes {
		paths = append(paths, filepath.Join(w, ".aws", "sso", "cache", "kiro-auth-token.json"))
	}
	return paths
}

// tokens returns every discovered login, best candidate first.
func (p *Provider) tokens(ctx context.Context) []*token {
	var out []*token
	if v := os.Getenv("ALL_USAGE_KIRO_TOKEN"); v != "" {
		out = append(out, &token{Access: v, Source: srcEnv, ProfileARN: os.Getenv("ALL_USAGE_KIRO_PROFILE_ARN")})
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
	if p.cfg.Source != "ide" {
		for _, path := range p.dbPaths() {
			add(path, func(path string) *token { return readCLIToken(ctx, path) })
		}
	}
	if p.cfg.Source != "cli" {
		for _, path := range p.ideTokenPaths() {
			add(path, readIDEToken)
		}
	}
	now := p.env.Time()
	sort.SliceStable(found, func(i, j int) bool { return found[i].usable(now) && !found[j].usable(now) })
	return append(out, found...)
}

func readCLIToken(ctx context.Context, path string) *token {
	t := &token{Source: srcCLI, Path: path}
	if st, err := os.Stat(path); err == nil {
		t.ModTime = st.ModTime()
	}
	kv, err := sys.ReadKV(ctx, path, "auth_kv", tokenKeys...)
	if err != nil {
		t.Err = err
		return t
	}
	var raw, key string
	for _, k := range tokenKeys {
		if v := kv[k]; v != "" {
			raw, key = v, k
			break
		}
	}
	if raw == "" {
		t.Err = errors.New("not signed in")
		return t
	}
	var f struct {
		AccessToken  string       `json:"access_token"`
		AccessToken2 string       `json:"accessToken"`
		ExpiresAt    jsonx.String `json:"expires_at"`
		ExpiresAt2   jsonx.String `json:"expiresAt"`
		Region       string       `json:"region"`
		StartURL     string       `json:"start_url"`
		ProfileARN   string       `json:"profile_arn"`
		ProfileARN2  string       `json:"profileArn"`
		Provider     string       `json:"provider"`
	}
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		t.Err = fmt.Errorf("invalid token record: %w", err)
		return t
	}
	t.Access = first(f.AccessToken, f.AccessToken2)
	t.Expiry, _ = jsonx.ParseTime(first(f.ExpiresAt.V, f.ExpiresAt2.V))
	t.Region = f.Region
	t.ProfileARN = first(f.ProfileARN, f.ProfileARN2)
	switch {
	case strings.Contains(key, ":social:"):
		t.Kind = first(f.Provider, "social login")
	case strings.Contains(f.StartURL, "view.awsapps.com/start"):
		t.Kind = "Builder ID"
	case f.StartURL != "":
		t.Kind = "IAM Identity Center"
	}
	if t.ProfileARN == "" {
		if st, err := sys.ReadKV(ctx, path, "state", profileKey); err == nil {
			t.ProfileARN = profileARN(st[profileKey])
		}
	}
	if t.Access == "" {
		t.Err = errors.New("token record has no access token")
	}
	return t
}

func readIDEToken(path string) *token {
	t := &token{Source: srcIDE, Path: path}
	st, err := os.Stat(path)
	if err != nil {
		t.Err = err
		return t
	}
	t.ModTime = st.ModTime()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Err = err
		return t
	}
	var f struct {
		AccessToken string       `json:"accessToken"`
		ExpiresAt   jsonx.String `json:"expiresAt"`
		Region      string       `json:"region"`
		ProfileARN  string       `json:"profileArn"`
		AuthMethod  string       `json:"authMethod"`
		Provider    string       `json:"provider"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Err = fmt.Errorf("invalid JSON: %w", err)
		return t
	}
	t.Access, t.Region, t.ProfileARN = f.AccessToken, f.Region, f.ProfileARN
	t.Expiry, _ = jsonx.ParseTime(f.ExpiresAt.V)
	switch strings.ToLower(f.AuthMethod) {
	case "idc":
		t.Kind = "IAM Identity Center"
	case "social":
		t.Kind = first(f.Provider, "social login")
	default:
		t.Kind = f.Provider
	}
	if t.Access == "" {
		t.Err = errors.New("no access token in file")
	}
	return t
}

// profileARN extracts the ARN from `{"arn": "...", "profile_name": "..."}` or a bare string.
func profileARN(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var obj struct {
		ARN string `json:"arn"`
	}
	if json.Unmarshal([]byte(raw), &obj) == nil && obj.ARN != "" {
		return obj.ARN
	}
	var s string
	if json.Unmarshal([]byte(raw), &s) == nil {
		return s
	}
	return raw
}

// regionFromARN returns the region field of an ARN (arn:aws:svc:REGION:acct:res).
func regionFromARN(arn string) string {
	parts := strings.SplitN(arn, ":", 6)
	if len(parts) == 6 && parts[0] == "arn" {
		return parts[3]
	}
	return ""
}

func first(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
