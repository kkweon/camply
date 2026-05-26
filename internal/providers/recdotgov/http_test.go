package recdotgov

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// shrinkBackoff makes retry sleeps negligible for tests and restores them afterward.
func shrinkBackoff(t *testing.T) {
	t.Helper()
	origBase, origMax, origAge := retryBaseDelay, retryMaxDelay, retryMaxAge
	retryBaseDelay = time.Millisecond
	retryMaxDelay = time.Millisecond
	retryMaxAge = time.Second
	t.Cleanup(func() {
		retryBaseDelay, retryMaxDelay, retryMaxAge = origBase, origMax, origAge
	})
}

func TestGetJSON_RetriesOn429(t *testing.T) {
	shrinkBackoff(t)

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	p := &Provider{client: server.Client()}

	var out struct {
		OK bool `json:"ok"`
	}
	if err := p.getJSON(context.Background(), server.URL, nil, &out); err != nil {
		t.Fatalf("expected success after retry, got %v", err)
	}
	if !out.OK {
		t.Errorf("expected decoded ok=true, got %+v", out)
	}
	if got := hits.Load(); got != 2 {
		t.Errorf("expected 2 requests (429 then 200), got %d", got)
	}
}

func TestGetJSON_TerminalOn403(t *testing.T) {
	shrinkBackoff(t)

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	p := &Provider{client: server.Client()}

	var out map[string]any
	if err := p.getJSON(context.Background(), server.URL, nil, &out); err == nil {
		t.Fatal("expected error on 403, got nil")
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("expected 403 to be terminal (1 request), got %d", got)
	}
}

func TestGetJSON_SetsBrowserHeaders(t *testing.T) {
	var gotUA, gotReferer, gotAPIKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotReferer = r.Header.Get("Referer")
		gotAPIKey = r.Header.Get("apikey")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	p := &Provider{client: server.Client()}
	var out map[string]any

	// recreation.gov call carries a browser UA + Referer.
	if err := p.getJSON(context.Background(), server.URL, recdotgovHeaders, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(gotUA, "Mozilla/5.0") {
		t.Errorf("expected a browser-like User-Agent, got %q", gotUA)
	}
	if gotReferer != "https://www.recreation.gov/" {
		t.Errorf("expected recreation.gov Referer, got %q", gotReferer)
	}

	// RIDB call carries the apikey.
	if err := p.getJSON(context.Background(), server.URL, ridbHeaders, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAPIKey != ridbApiKey {
		t.Errorf("expected apikey %q, got %q", ridbApiKey, gotAPIKey)
	}
}
