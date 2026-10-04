package sys

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// JWTClaims decodes the payload of a JWT without verifying its signature.
// Tokens are only inspected locally (expiry, plan, subject); they are never
// trusted for authorization decisions.
func JWTClaims(token string) (map[string]any, error) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 {
		return nil, errors.New("not a JWT")
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return nil, err
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		return nil, err
	}
	return claims, nil
}

// JWTExpiry returns the "exp" claim of a JWT.
func JWTExpiry(token string) (time.Time, bool) {
	claims, err := JWTClaims(token)
	if err != nil {
		return time.Time{}, false
	}
	exp, ok := claims["exp"].(float64)
	if !ok || exp <= 0 {
		return time.Time{}, false
	}
	return time.Unix(int64(exp), 0), true
}

// ClaimString returns a nested string claim, e.g. ClaimString(c, "https://api.openai.com/auth", "chatgpt_plan_type").
func ClaimString(claims map[string]any, path ...string) string {
	var cur any = claims
	for _, p := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = m[p]
	}
	s, _ := cur.(string)
	return s
}

// FindBinary resolves an executable given as a name or path. Besides $PATH it
// checks common per-user install locations, because GUI-launched terminals do
// not always include them.
func FindBinary(name string) (string, bool) {
	name = ExpandPath(name)
	if name == "" {
		return "", false
	}
	if strings.ContainsRune(name, os.PathSeparator) || strings.Contains(name, "/") {
		if isExecutable(name) {
			return name, true
		}
		return "", false
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, true
	}
	dirs := []string{
		filepath.Join(Home(), ".local", "bin"),
		filepath.Join(Home(), "bin"),
		filepath.Join(Home(), ".npm-global", "bin"),
		filepath.Join(Home(), ".bun", "bin"),
		"/opt/homebrew/bin",
		"/usr/local/bin",
	}
	for _, d := range dirs {
		p := filepath.Join(d, name)
		if runtime.GOOS == "windows" {
			p += ".exe"
		}
		if isExecutable(p) {
			return p, true
		}
	}
	return "", false
}

func isExecutable(p string) bool {
	st, err := os.Stat(p)
	if err != nil || st.IsDir() {
		return false
	}
	return runtime.GOOS == "windows" || st.Mode()&0o111 != 0
}
