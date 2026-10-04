// Package core defines the provider-agnostic usage model shared by the
// providers, the TUI and the plain-text/JSON renderers.
package core

import (
	"context"
	"time"

	"github.com/cfardev/all-usage/internal/httpx"
)

// Meter is one usage gauge (a rate-limit window, a credit pool, a spend cap...).
//
// Ratio meters have Percent set (and usually Used/Limit). Value meters only
// carry a preformatted Value (e.g. a credit balance) and render without a bar.
type Meter struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Short is a compact tag used by one-line outputs ("5h", "wk").
	Short string `json:"short,omitempty"`
	// Headline meters summarize the provider in one-line outputs.
	Headline bool `json:"headline,omitempty"`

	Percent *float64 `json:"percent,omitempty"` // 0..100 (may exceed 100)
	Used    *float64 `json:"used,omitempty"`
	Limit   *float64 `json:"limit,omitempty"`
	// Unit of Used/Limit: "credits", "usd", "requests"... Empty for pure percentages.
	Unit string `json:"unit,omitempty"`
	// Value is the display value of a value meter ("$12.40", "unlimited").
	Value string `json:"value,omitempty"`

	ResetsAt      *time.Time `json:"resets_at,omitempty"`
	WindowSeconds int64      `json:"window_seconds,omitempty"`
	// Detail is an extra, provider-specific line ("incl. $78.66 bonus").
	Detail string `json:"detail,omitempty"`
}

// HasPercent reports whether the meter is a ratio meter.
func (m Meter) HasPercent() bool { return m.Percent != nil }

// Pct returns the percentage (0 when not a ratio meter).
func (m Meter) Pct() float64 {
	if m.Percent == nil {
		return 0
	}
	return *m.Percent
}

// Usage is a provider's usage snapshot.
type Usage struct {
	Provider string `json:"id"`
	Name     string `json:"name"`
	Plan     string `json:"plan,omitempty"`
	Account  string `json:"account,omitempty"`
	// Source describes where the data came from ("chatgpt.com API").
	Source string   `json:"source,omitempty"`
	Meters []Meter  `json:"meters"`
	Notes  []string `json:"notes,omitempty"`

	FetchedAt time.Time `json:"fetched_at"`
	// AsOf is set when the data is older than FetchedAt (e.g. read from logs).
	AsOf *time.Time `json:"as_of,omitempty"`
	// Stale marks data that may be outdated.
	Stale bool `json:"stale,omitempty"`
	// Warning explains why live data could not be fetched when stale data is shown.
	Warning *Error `json:"warning,omitempty"`
}

// Result is the outcome of fetching one provider.
type Result struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Usage    *Usage        `json:"usage,omitempty"`
	Err      *Error        `json:"error,omitempty"`
	Duration time.Duration `json:"-"`
}

// OK reports whether usage was fetched.
func (r Result) OK() bool { return r.Err == nil && r.Usage != nil }

// CheckStatus is the outcome of a doctor check.
type CheckStatus int

// Doctor check outcomes.
const (
	CheckInfo CheckStatus = iota
	CheckOK
	CheckWarn
	CheckFail
)

// Check is one line of `all-usage doctor` output.
type Check struct {
	Status CheckStatus
	Label  string
	Detail string
}

// Provider fetches usage for one service.
type Provider interface {
	ID() string
	Name() string
	// Fetch returns the current usage. Errors should be *Error values with
	// actionable hints.
	Fetch(ctx context.Context) (*Usage, error)
	// Doctor reports how credentials are discovered, without fetching usage.
	Doctor(ctx context.Context) []Check
}

// Env carries shared runtime services for providers.
type Env struct {
	HTTP *httpx.Client
	// WindowsHomes are Windows profile dirs reachable from WSL (may be empty).
	WindowsHomes []string
	// ShowAccount allows providers to include account identifiers (emails).
	ShowAccount bool
	// Now returns the current time (overridable in tests).
	Now func() time.Time
}

// Time returns the current time.
func (e *Env) Time() time.Time {
	if e != nil && e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

// F64 returns a pointer to v (helper for optional meter fields).
func F64(v float64) *float64 { return &v }

// TimePtr returns a pointer to t, or nil for the zero time.
func TimePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
