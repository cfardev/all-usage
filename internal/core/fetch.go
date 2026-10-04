package core

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"
)

// Fetch runs p.Fetch with a timeout, converting errors and panics into Result.Err.
func Fetch(ctx context.Context, p Provider, timeout time.Duration) (res Result) {
	start := time.Now()
	res = Result{ID: p.ID(), Name: p.Name()}
	defer func() {
		if r := recover(); r != nil {
			res.Usage = nil
			res.Err = &Error{Kind: KindInternal, Msg: fmt.Sprintf("internal error: %v", r), Hint: "Please report this bug."}
		}
		res.Duration = time.Since(start)
	}()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	u, err := p.Fetch(ctx)
	if err != nil {
		res.Err = AsError(err)
		return res
	}
	if u == nil {
		res.Err = &Error{Kind: KindInternal, Msg: "provider returned no data"}
		return res
	}
	res.Usage = u
	return res
}

// FetchAll fetches every provider concurrently, preserving order.
func FetchAll(ctx context.Context, ps []Provider, timeout time.Duration) []Result {
	out := make([]Result, len(ps))
	var wg sync.WaitGroup
	for i, p := range ps {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = Fetch(ctx, p, timeout)
		}()
	}
	wg.Wait()
	return out
}

// FilterMeters keeps only meters whose ID is in ids (all when ids is empty),
// in the order given by ids.
func FilterMeters(ms []Meter, ids []string) []Meter {
	if len(ids) == 0 {
		return ms
	}
	out := make([]Meter, 0, len(ms))
	for _, id := range ids {
		for _, m := range ms {
			if m.ID == id && !slices.ContainsFunc(out, func(x Meter) bool { return x.ID == m.ID }) {
				out = append(out, m)
			}
		}
	}
	return out
}
