package k8sclient

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

func TestFetchNodes_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()

	_, err := FetchNodes(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected an error for malformed JSON, got nil")
	}
}

func TestFetchNodes_HTTP500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := FetchNodes(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected an error for HTTP 500, got nil")
	}
}

func TestFetchNodes_ConnectionRefused(t *testing.T) {
	// Bind a listener to get a genuinely free port, then close it
	// immediately so nothing is listening — guarantees connection refused
	// rather than racing a real service for the port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	_, err = FetchNodes(context.Background(), "http://"+addr)
	if err == nil {
		t.Fatal("expected an error for connection refused, got nil")
	}
}

func TestFetchNodes_TimeoutBoundsTheCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Write([]byte(`{"items": []}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := FetchNodes(ctx, srv.URL)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if elapsed > time.Second {
		t.Fatalf("FetchNodes did not respect the context timeout: took %v", elapsed)
	}
}
