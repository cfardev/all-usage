package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrorKind classifies provider failures so the UI can explain them.
type ErrorKind string

// Error kinds.
const (
	KindNotConfigured ErrorKind = "not_configured" // no credentials found
	KindAuth          ErrorKind = "auth"           // expired or rejected credentials
	KindNetwork       ErrorKind = "network"        // transport error or timeout
	KindAPI           ErrorKind = "api"            // unexpected HTTP status
	KindParse         ErrorKind = "parse"          // unexpected response shape
	KindUnsupported   ErrorKind = "unsupported"    // account type has no usage data
	KindInternal      ErrorKind = "internal"
)

// Error is a provider failure with a user-facing message and a hint.
type Error struct {
	Kind ErrorKind `json:"kind"`
	Msg  string    `json:"message"`
	// Where is the credential location involved (a file path), if any.
	Where string `json:"where,omitempty"`
	Hint  string `json:"hint,omitempty"`
	Err   error  `json:"-"`
}

func (e *Error) Error() string {
	s := e.Full()
	if e.Err != nil && !strings.Contains(s, e.Err.Error()) {
		s += ": " + e.Err.Error()
	}
	return s
}

func (e *Error) Unwrap() error { return e.Err }

// Full returns the message followed by the location, for detailed outputs.
func (e *Error) Full() string {
	if e.Where == "" {
		return e.Msg
	}
	return e.Msg + " (" + e.Where + ")"
}

// At records the credential location involved and returns e.
func (e *Error) At(where string) *Error {
	e.Where = where
	return e
}

// Title is a short label for the error kind.
func (e *Error) Title() string {
	switch e.Kind {
	case KindNotConfigured:
		return "Not signed in"
	case KindAuth:
		return "Session expired"
	case KindNetwork:
		return "Network error"
	case KindUnsupported:
		return "Not available"
	case KindParse:
		return "Unexpected response"
	default:
		return "Error"
	}
}

// NewError builds an *Error.
func NewError(kind ErrorKind, hint, format string, args ...any) *Error {
	return &Error{Kind: kind, Msg: fmt.Sprintf(format, args...), Hint: hint}
}

// Wrap builds an *Error around err.
func Wrap(kind ErrorKind, err error, hint, format string, args ...any) *Error {
	return &Error{Kind: kind, Msg: fmt.Sprintf(format, args...), Hint: hint, Err: err}
}

// NetworkError describes a transport failure.
func NetworkError(err error, host string) *Error {
	if errors.Is(err, context.DeadlineExceeded) {
		return &Error{Kind: KindNetwork, Msg: "request to " + host + " timed out", Hint: "Check your connection, or raise `timeout` in the config.", Err: err}
	}
	if errors.Is(err, context.Canceled) {
		return &Error{Kind: KindNetwork, Msg: "request canceled", Err: err}
	}
	return &Error{Kind: KindNetwork, Msg: "cannot reach " + host, Hint: "Check your internet connection or proxy settings (HTTPS_PROXY).", Err: err}
}

// HTTPError describes an unexpected HTTP status, including a short excerpt of
// the server's error message when one can be found.
func HTTPError(status int, body []byte, host string) *Error {
	msg := fmt.Sprintf("%s returned HTTP %d", host, status)
	if detail := errorDetail(body); detail != "" {
		msg += ": " + detail
	}
	hint := ""
	if status == 429 {
		hint = "Rate limited; it will retry on the next refresh."
	} else if status >= 500 {
		hint = "The service may be having issues; it will retry on the next refresh."
	}
	return &Error{Kind: KindAPI, Msg: msg, Hint: hint}
}

// errorDetail extracts a human message from common JSON error shapes.
func errorDetail(body []byte) string {
	var v map[string]any
	if json.Unmarshal(body, &v) != nil {
		return ""
	}
	pick := func(m map[string]any, keys ...string) string {
		for _, k := range keys {
			if s, ok := m[k].(string); ok && s != "" {
				return s
			}
		}
		return ""
	}
	if s := pick(v, "message", "detail", "error_description", "description"); s != "" {
		return truncate(s, 160)
	}
	if e, ok := v["error"].(map[string]any); ok {
		if s := pick(e, "message", "code"); s != "" {
			return truncate(s, 160)
		}
	}
	if s := pick(v, "error", "code"); s != "" {
		return truncate(s, 160)
	}
	return ""
}

func truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

// AsError converts any error into an *Error.
func AsError(err error) *Error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &Error{Kind: KindNetwork, Msg: "timed out", Hint: "Raise `timeout` in the config if this keeps happening.", Err: err}
	}
	return &Error{Kind: KindInternal, Msg: err.Error(), Err: err}
}

// IsAuth reports whether err is an authentication failure.
func IsAuth(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Kind == KindAuth
}
