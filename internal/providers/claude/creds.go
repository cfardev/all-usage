package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/cfardev/all-usage/internal/sys"
	"github.com/cfardev/all-usage/internal/ui"
)

// Sources of a Claude login.
const (
	srcFile     = "Claude Code"
	srcKeychain = "Claude Code (Keychain)"
	srcEnv      = "env"
)

const keychainService = "Claude Code-credentials"

// credential is a Claude Code OAuth login.
type credential struct {
	AccessToken  string
	Source       string
	Path         string
	Expiry       time.Time
	Subscription string // subscriptionType: "pro", "max"...
	Tier         string // rateLimitTier: "default_claude_max_5x"...
	Err          error
}

func (c *credential) expired(now time.Time) bool {
	return !c.Expiry.IsZero() && !c.Expiry.After(now.Add(30*time.Second))
}

func (c *credential) usable(now time.Time) bool {
	return c.Err == nil && c.AccessToken != "" && !c.expired(now)
}

func (c *credential) label() string {
	switch c.Source {
	case srcEnv:
		return "$ALL_USAGE_CLAUDE_TOKEN"
	case srcKeychain:
		return "Keychain " + keychainService
	}
	return sys.ShortPath(c.Path)
}

var tierRe = regexp.MustCompile(`(\d+x)$`)

// plan is the display name of the subscription ("Max 5x").
func (c *credential) plan() string {
	s := strings.ToLower(strings.TrimSpace(c.Subscription))
	if s == "" {
		return ""
	}
	name := ui.TitleCase(s)
	if m := tierRe.FindString(c.Tier); m != "" {
		name += " " + m
	}
	return name
}

func (p *Provider) credentialPaths() []string {
	if p.cfg.CredentialsFile != "" {
		return []string{sys.ExpandPath(p.cfg.CredentialsFile)}
	}
	dir := filepath.Join(sys.Home(), ".claude")
	if v := os.Getenv("CLAUDE_CONFIG_DIR"); v != "" {
		dir = sys.ExpandPath(v)
	}
	paths := []string{filepath.Join(dir, ".credentials.json")}
	for _, w := range p.env.WindowsHomes {
		paths = append(paths, filepath.Join(w, ".claude", ".credentials.json"))
	}
	return paths
}

// credentials returns every discovered login, best candidate first.
func (p *Provider) credentials(ctx context.Context) []*credential {
	var out []*credential
	if v := os.Getenv("ALL_USAGE_CLAUDE_TOKEN"); v != "" {
		out = append(out, &credential{AccessToken: v, Source: srcEnv})
	}
	var found []*credential
	seen := map[string]bool{}
	for _, path := range p.credentialPaths() {
		real, err := filepath.EvalSymlinks(path)
		if err != nil || seen[real] {
			continue
		}
		seen[real] = true
		c := &credential{Source: srcFile, Path: path}
		if b, err := os.ReadFile(path); err != nil {
			c.Err = err
		} else {
			parseCredentials(c, b)
		}
		found = append(found, c)
	}
	// Claude Code keeps the macOS login in the Keychain instead of a file.
	if runtime.GOOS == "darwin" && p.cfg.CredentialsFile == "" {
		if b, err := exec.CommandContext(ctx, "security", "find-generic-password", "-s", keychainService, "-w").Output(); err == nil {
			c := &credential{Source: srcKeychain}
			parseCredentials(c, b)
			found = append(found, c)
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

func parseCredentials(c *credential, b []byte) {
	var f struct {
		OAuth *struct {
			AccessToken      string `json:"accessToken"`
			ExpiresAt        int64  `json:"expiresAt"` // unix ms
			SubscriptionType string `json:"subscriptionType"`
			RateLimitTier    string `json:"rateLimitTier"`
		} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		c.Err = fmt.Errorf("invalid JSON: %w", err)
		return
	}
	if f.OAuth == nil || f.OAuth.AccessToken == "" {
		c.Err = errors.New("not signed in with a Claude subscription")
		return
	}
	c.AccessToken = f.OAuth.AccessToken
	c.Subscription, c.Tier = f.OAuth.SubscriptionType, f.OAuth.RateLimitTier
	if f.OAuth.ExpiresAt > 0 {
		c.Expiry = time.UnixMilli(f.OAuth.ExpiresAt)
	}
}
