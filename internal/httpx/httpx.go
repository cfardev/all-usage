// Package httpx is a thin HTTP helper shared by providers: JSON requests,
// bounded response bodies and a consistent User-Agent.
package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// maxBody caps how much of a response is read (usage payloads are small).
const maxBody = 4 << 20

// Client performs HTTP requests on behalf of providers.
type Client struct {
	hc        *http.Client
	userAgent string
}

// New returns a client. Per-request deadlines come from the caller's context;
// timeout is a safety net for requests issued without one.
func New(timeout time.Duration, userAgent string) *Client {
	return &Client{
		hc:        &http.Client{Timeout: timeout},
		userAgent: userAgent,
	}
}

// Response is a fully-read HTTP response.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

// OK reports whether the status code is 2xx.
func (r *Response) OK() bool { return r.Status >= 200 && r.Status < 300 }

// Decode unmarshals the JSON body into v.
func (r *Response) Decode(v any) error {
	if err := json.Unmarshal(r.Body, v); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// Request describes an outgoing request.
type Request struct {
	Method  string
	URL     string
	Headers map[string]string
	// JSON, when non-nil, is marshaled as the request body with
	// Content-Type: application/json (unless overridden in Headers).
	JSON any
}

// Do sends the request and reads the whole (bounded) response body. Only
// transport-level failures are returned as errors; HTTP error statuses are
// returned in Response for the caller to interpret.
func (c *Client) Do(ctx context.Context, r Request) (*Response, error) {
	var body io.Reader
	if r.JSON != nil {
		b, err := json.Marshal(r.JSON)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	method := r.Method
	if method == "" {
		method = http.MethodGet
	}
	req, err := http.NewRequestWithContext(ctx, method, r.URL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")
	if r.JSON != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range r.Headers {
		req.Header.Set(k, v)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, err
	}
	return &Response{Status: resp.StatusCode, Header: resp.Header, Body: b}, nil
}
