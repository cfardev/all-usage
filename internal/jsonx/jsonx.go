// Package jsonx provides lenient JSON value types for third-party APIs whose
// fields are not always encoded consistently (for example protobuf int64
// values serialized as strings, or numbers that are sometimes null).
package jsonx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Number is a nullable float64 that accepts JSON numbers, numeric strings and null.
type Number struct {
	V     float64
	Valid bool
}

// Num returns a valid Number holding v.
func Num(v float64) Number { return Number{V: v, Valid: true} }

// UnmarshalJSON implements json.Unmarshaler.
func (n *Number) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" || s == `""` {
		*n = Number{}
		return nil
	}
	s = strings.Trim(s, `"`)
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return fmt.Errorf("jsonx: invalid number %q", s)
	}
	*n = Number{V: f, Valid: true}
	return nil
}

// MarshalJSON implements json.Marshaler.
func (n Number) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return []byte("null"), nil
	}
	return strconv.AppendFloat(nil, n.V, 'f', -1, 64), nil
}

// Or returns the value, or def when the number is null.
func (n Number) Or(def float64) float64 {
	if !n.Valid {
		return def
	}
	return n.V
}

// String is a nullable string that also accepts JSON numbers and booleans
// (kept as their literal text).
type String struct {
	V     string
	Valid bool
}

// UnmarshalJSON implements json.Unmarshaler.
func (s *String) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if string(b) == "null" {
		*s = String{}
		return nil
	}
	var str string
	if err := json.Unmarshal(b, &str); err == nil {
		*s = String{V: str, Valid: true}
		return nil
	}
	if len(b) > 0 && (b[0] == '{' || b[0] == '[') {
		return fmt.Errorf("jsonx: expected string, got %s", string(b[:1]))
	}
	*s = String{V: string(b), Valid: true}
	return nil
}

// MarshalJSON implements json.Marshaler.
func (s String) MarshalJSON() ([]byte, error) {
	if !s.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(s.V)
}

// UnixTime converts a Unix timestamp to time.Time. Values that are clearly in
// milliseconds (after year 2286 when read as seconds) are treated as such.
// Zero or negative values yield the zero time.
func UnixTime(v float64) time.Time {
	if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return time.Time{}
	}
	if v > 1e11 { // milliseconds
		return time.UnixMilli(int64(v))
	}
	sec, frac := math.Modf(v)
	return time.Unix(int64(sec), int64(frac*1e9))
}

// ParseTime parses common timestamp encodings: RFC 3339 (with or without
// fractional seconds) and Unix seconds/milliseconds as a string.
func ParseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		t := UnixTime(f)
		return t, !t.IsZero()
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.999999999", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
