// Copyright 2026 Erst Users
// SPDX-License-Identifier: Apache-2.0

package rpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type headerMiddleware struct {
	next http.RoundTripper
}

func (m *headerMiddleware) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("X-Custom-Header", "injected")
	return m.next.RoundTrip(req)
}

func TestMiddlewareInjection(t *testing.T) {
	// Setup a mock server to check headers
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom-Header") == "injected" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"jsonrpc": "2.0", "result": {"status": "healthy"}, "id": 1}`))
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()

	// Define a custom middleware
	mw := func(next http.RoundTripper) http.RoundTripper {
		return &headerMiddleware{next: next}
	}

	// Create client with middleware
	client, err := NewClient(
		WithHorizonURL(server.URL),
		WithSorobanURL(server.URL),
		WithMiddleware(mw),
	)
	assert.NoError(t, err)

	// Test a call that uses the HTTP client
	ctx := context.Background()
	resp, err := client.GetHealth(ctx)

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "healthy", resp.Result.Status)
}

// TestCreateHTTPClient_MiddlewareAppliedOnce verifies that each middleware is wrapped
// into the transport chain exactly once.  Before the Protocol V2 standardization fix,
// createHTTPClient applied the middleware slice twice (once before RetryTransport and
// once after), causing every middleware to intercept each request twice.
func TestCreateHTTPClient_MiddlewareAppliedOnce(t *testing.T) {
	wrapCount := 0
	mw := func(next http.RoundTripper) http.RoundTripper {
		wrapCount++
		return next
	}

	_ = createHTTPClient("token", 5*time.Second, mw)

	if wrapCount != 1 {
		t.Errorf("middleware should be applied exactly once, got %d applications", wrapCount)
	}
}

// TestCreateHTTPClient_MultipleMiddlewaresAppliedOnceEach verifies the invariant holds
// when more than one middleware is provided.
func TestCreateHTTPClient_MultipleMiddlewaresAppliedOnceEach(t *testing.T) {
	counts := make([]int, 3)
	mws := make([]Middleware, 3)
	for i := range mws {
		i := i
		mws[i] = func(next http.RoundTripper) http.RoundTripper {
			counts[i]++
			return next
		}
	}

	_ = createHTTPClient("", 5*time.Second, mws...)

	for i, c := range counts {
		if c != 1 {
			t.Errorf("middleware[%d] should be applied exactly once, got %d", i, c)
		}
	}
}

func BenchmarkMiddleware(b *testing.B) {
	// Simple middleware that does nothing
	mw := func(next http.RoundTripper) http.RoundTripper {
		return next
	}

	client, _ := NewClient(WithMiddleware(mw))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Just creating the client or doing something light
		_ = client.getHTTPClient()
	}
}
