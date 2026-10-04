package jsonx

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNumber(t *testing.T) {
	var v struct {
		A, B, C, D Number
		E          Number `json:"e"`
	}
	if err := json.Unmarshal([]byte(`{"A": 1.5, "B": "1793544890000", "C": null, "D": ""}`), &v); err != nil {
		t.Fatal(err)
	}
	if v.A != Num(1.5) || v.B.V != 1793544890000 || v.C.Valid || v.D.Valid || v.E.Valid {
		t.Fatalf("got %+v", v)
	}
	if v.C.Or(7) != 7 || v.A.Or(7) != 1.5 {
		t.Error("Or")
	}
	if err := json.Unmarshal([]byte(`{"A": "abc"}`), &v); err == nil {
		t.Error("expected error")
	}
	b, _ := json.Marshal(struct{ X, Y Number }{Num(2), Number{}})
	if string(b) != `{"X":2,"Y":null}` {
		t.Errorf("marshal = %s", b)
	}
}

func TestString(t *testing.T) {
	var v struct{ A, B, C, D String }
	if err := json.Unmarshal([]byte(`{"A": "x", "B": 12.5, "C": null, "D": true}`), &v); err != nil {
		t.Fatal(err)
	}
	if v.A.V != "x" || v.B.V != "12.5" || v.C.Valid || v.D.V != "true" {
		t.Fatalf("got %+v", v)
	}
	if err := json.Unmarshal([]byte(`{"A": {"k": 1}}`), &v); err == nil {
		t.Error("objects must not decode into String")
	}
}

func TestTimes(t *testing.T) {
	if !UnixTime(1793491200).Equal(time.Unix(1793491200, 0)) {
		t.Error("seconds")
	}
	if !UnixTime(1793544890000).Equal(time.UnixMilli(1793544890000)) {
		t.Error("milliseconds")
	}
	if !UnixTime(0).IsZero() || !UnixTime(-5).IsZero() {
		t.Error("non-positive must be zero")
	}
	for in, want := range map[string]int64{
		"2026-11-01T14:54:50.000Z": 1793544890, "2026-10-03T23:08:35.670121148Z": 1791068915, "1793544890000": 1793544890,
		"1793491200.0": 1793491200, "2026-11-01": 1793491200,
	} {
		got, ok := ParseTime(in)
		if !ok || got.Unix() != want {
			t.Errorf("ParseTime(%q) = %v %v, want %d", in, got.Unix(), ok, want)
		}
	}
	if _, ok := ParseTime("soon"); ok {
		t.Error("expected failure")
	}
}
