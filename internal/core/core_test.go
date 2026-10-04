package core

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeProvider struct {
	id    string
	fetch func(ctx context.Context) (*Usage, error)
}

func (f fakeProvider) ID() string                                { return f.id }
func (f fakeProvider) Name() string                              { return strings.ToUpper(f.id) }
func (f fakeProvider) Fetch(ctx context.Context) (*Usage, error) { return f.fetch(ctx) }
func (f fakeProvider) Doctor(context.Context) []Check            { return nil }

func TestFetchTimeoutPanicAndNil(t *testing.T) {
	slow := fakeProvider{id: "slow", fetch: func(ctx context.Context) (*Usage, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	r := Fetch(context.Background(), slow, 20*time.Millisecond)
	if r.Err == nil || r.Err.Kind != KindNetwork {
		t.Errorf("timeout: %+v", r.Err)
	}
	boom := fakeProvider{id: "boom", fetch: func(context.Context) (*Usage, error) { panic("kaboom") }}
	if r := Fetch(context.Background(), boom, time.Second); r.Err == nil || !strings.Contains(r.Err.Msg, "kaboom") {
		t.Errorf("panic: %+v", r.Err)
	}
	nilp := fakeProvider{id: "nil", fetch: func(context.Context) (*Usage, error) { return nil, nil }}
	if r := Fetch(context.Background(), nilp, time.Second); r.OK() || r.Err == nil {
		t.Errorf("nil usage must be an error: %+v", r)
	}
	plain := fakeProvider{id: "plain", fetch: func(context.Context) (*Usage, error) { return nil, errors.New("plain") }}
	if r := Fetch(context.Background(), plain, time.Second); r.Err.Kind != KindInternal {
		t.Errorf("plain error kind = %v", r.Err.Kind)
	}
}

func TestFetchAllKeepsOrder(t *testing.T) {
	var ps []Provider
	for i, d := range []time.Duration{30, 1, 15} {
		id := string(rune('a' + i))
		d := d
		ps = append(ps, fakeProvider{id: id, fetch: func(context.Context) (*Usage, error) {
			time.Sleep(d * time.Millisecond)
			return &Usage{Provider: id}, nil
		}})
	}
	rs := FetchAll(context.Background(), ps, time.Second)
	for i, want := range []string{"a", "b", "c"} {
		if rs[i].ID != want || rs[i].Usage.Provider != want {
			t.Fatalf("result %d = %+v", i, rs[i])
		}
	}
}

func TestFilterMeters(t *testing.T) {
	ms := []Meter{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	if got := FilterMeters(ms, nil); len(got) != 3 {
		t.Error("empty filter keeps all")
	}
	got := FilterMeters(ms, []string{"c", "a", "zzz", "a"})
	if len(got) != 2 || got[0].ID != "c" || got[1].ID != "a" {
		t.Errorf("got %+v", got)
	}
}

func TestHTTPErrorDetail(t *testing.T) {
	cases := map[string]string{
		`{"message":"The bearer token is invalid."}`:             "The bearer token is invalid.",
		`{"error":{"message":"nested message","code":"x"}}`:      "nested message",
		`{"error":"not_authenticated","description":"desc"}`:     "desc",
		`{"detail":"Could not parse your authentication token"}`: "Could not parse your authentication token",
		`<html>oops</html>`: "",
	}
	for body, want := range cases {
		e := HTTPError(500, []byte(body), "api.example.com")
		if want == "" && e.Msg != "api.example.com returned HTTP 500" {
			t.Errorf("%s -> %q", body, e.Msg)
		}
		if want != "" && !strings.HasSuffix(e.Msg, ": "+want) {
			t.Errorf("%s -> %q", body, e.Msg)
		}
		if e.Hint == "" {
			t.Errorf("5xx should have a hint")
		}
	}
}

func TestErrorHelpers(t *testing.T) {
	e := Wrap(KindAuth, errors.New("inner"), "do X", "outer %d", 1)
	if e.Error() != "outer 1: inner" || !IsAuth(e) || !errors.Is(e, e.Err) {
		t.Errorf("wrap: %v", e)
	}
	if AsError(nil) != nil {
		t.Error("AsError(nil) must be nil")
	}
	if NetworkError(context.DeadlineExceeded, "h").Kind != KindNetwork {
		t.Error("network kind")
	}
	if (&Error{Kind: KindNotConfigured}).Title() != "Not signed in" {
		t.Error("title")
	}
}
