package k8sclient

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
)

func TestFetchNodes_Valid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"kind": "NodeList",
			"apiVersion": "v1",
			"items": [
				{"metadata": {"name": "node-a"}},
				{"metadata": {"name": "node-b"}}
			]
		}`))
	}))
	defer srv.Close()

	list, err := FetchNodes(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list.Items) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(list.Items))
	}
	if list.Items[0].Name != "node-a" {
		t.Errorf("unexpected first node name: %q", list.Items[0].Name)
	}
}

// TestFetchNodes_ErrorContract pins the error contract from
// docs/research/bubbletea-fault-isolation.md: on deadline exceeded, dial
// failure, non-200, or decode failure, FetchNodes returns (nil, err) — never
// panics, never blocks past its context timeout.
func TestFetchNodes_ErrorContract(t *testing.T) {
	cases := []struct {
		name string
		// setup returns the endpoint to fetch plus a func that stops any
		// server it started, called when the row completes.
		setup func(t *testing.T) (endpoint string, teardown func())
		// ctxTTL bounds the fetch; 0 means no deadline.
		ctxTTL time.Duration
		// maxLatency is the elapsed-time bound the row must meet. It is
		// always <= ctxTTL's intent: failures must come back promptly, and
		// the hung-handler row must return near its deadline rather than
		// waiting out the handler.
		maxLatency time.Duration
		wantErrSub string
	}{
		{
			name: "non-200 response",
			setup: func(t *testing.T) (string, func()) {
				return serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}))
			},
			ctxTTL:     5 * time.Second,
			maxLatency: 2 * time.Second,
			wantErrSub: "HTTP 500",
		},
		{
			name: "decode failure",
			setup: func(t *testing.T) (string, func()) {
				return serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Write([]byte(`{not valid json`))
				}))
			},
			ctxTTL:     5 * time.Second,
			maxLatency: 2 * time.Second,
			wantErrSub: "decode",
		},
		{
			name: "connection refused",
			setup: func(t *testing.T) (string, func()) {
				// Bind a listener to get a genuinely free port, then close it
				// immediately so nothing is listening — guarantees connection
				// refused rather than racing a real service for the port.
				ln, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				addr := ln.Addr().String()
				ln.Close()
				return "http://" + addr, func() {}
			},
			ctxTTL:     5 * time.Second,
			maxLatency: 2 * time.Second,
			wantErrSub: "connection refused",
		},
		{
			name: "hung handler past the deadline",
			setup: func(t *testing.T) (string, func()) {
				// The handler never writes within the client's window; it
				// also watches the request context so server teardown doesn't
				// wait out the full sleep once the client is gone.
				return serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					select {
					case <-time.After(5 * time.Second):
						w.Write([]byte(`{"items": []}`))
					case <-r.Context().Done():
					}
				}))
			},
			ctxTTL:     100 * time.Millisecond,
			maxLatency: 1 * time.Second,
			wantErrSub: "context deadline exceeded",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			endpoint, teardown := tc.setup(t)
			defer teardown()

			ctx := context.Background()
			if tc.ctxTTL > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tc.ctxTTL)
				defer cancel()
			}

			var (
				list     *corev1.NodeList
				err      error
				panicked any
			)
			start := time.Now()
			func() {
				defer func() { panicked = recover() }()
				list, err = FetchNodes(ctx, endpoint)
			}()
			elapsed := time.Since(start)

			if panicked != nil {
				t.Fatalf("FetchNodes panicked: %v", panicked)
			}
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if list != nil {
				t.Errorf("expected nil result alongside the error, got %+v", list)
			}
			if !strings.Contains(err.Error(), tc.wantErrSub) {
				t.Errorf("error %q does not mention %q", err, tc.wantErrSub)
			}
			if elapsed > tc.maxLatency {
				t.Errorf("call took %v, exceeding the %v bound", elapsed, tc.maxLatency)
			}
		})
	}
}

// serve starts an httptest.Server for one table row and returns its URL
// along with the teardown func.
func serve(t *testing.T, h http.Handler) (string, func()) {
	t.Helper()
	srv := httptest.NewServer(h)
	return srv.URL, srv.Close
}
