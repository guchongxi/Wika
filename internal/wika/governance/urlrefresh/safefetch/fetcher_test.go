package safefetch

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

type fakeURLValidator struct {
	rejectContains string
	calls          []string
}

func (v *fakeURLValidator) ValidateURL(ctx context.Context, raw string) (*url.URL, error) {
	v.calls = append(v.calls, raw)
	if v.rejectContains != "" && strings.Contains(raw, v.rejectContains) {
		return nil, errors.New("blocked by validator")
	}
	return url.Parse(raw)
}

func TestFetcherReadsSafeTextWithoutProxy(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("fresh knowledge"))
	}))
	defer target.Close()

	var proxyHits atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHits.Add(1)
		http.Error(w, "proxy should not be used", http.StatusBadGateway)
	}))
	defer proxy.Close()

	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)

	dialer := net.Dialer{}
	dialContext := func(ctx context.Context, network string, addr string) (net.Conn, error) {
		if addr == proxy.Listener.Addr().String() {
			return dialer.DialContext(ctx, network, addr)
		}
		return dialer.DialContext(ctx, network, target.Listener.Addr().String())
	}

	fetcher := NewFetcher(&fakeURLValidator{}, WithMaxBytes(1024), WithDialContext(dialContext))
	got, err := fetcher.Fetch(context.Background(), "http://safe.example.com/resource")
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if got.Text != "fresh knowledge" {
		t.Fatalf("unexpected fetched text: %q", got.Text)
	}
	if got.ContentType != "text/plain; charset=utf-8" {
		t.Fatalf("unexpected content type: %q", got.ContentType)
	}
	if proxyHits.Load() != 0 {
		t.Fatalf("proxy was used %d times", proxyHits.Load())
	}
}

func TestFetcherRevalidatesRedirectTarget(t *testing.T) {
	validator := &fakeURLValidator{rejectContains: "/blocked"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/blocked", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("blocked"))
	}))
	defer server.Close()

	fetcher := NewFetcher(validator, WithMaxBytes(1024))
	if _, err := fetcher.Fetch(context.Background(), server.URL+"/start"); err == nil {
		t.Fatalf("expected redirect target to be rejected")
	}
	if len(validator.calls) < 2 {
		t.Fatalf("expected validator to be called for original URL and redirect, got %v", validator.calls)
	}
}

func TestFetcherRejectsUnsupportedContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte("binary"))
	}))
	defer server.Close()

	fetcher := NewFetcher(&fakeURLValidator{}, WithMaxBytes(1024))
	if _, err := fetcher.Fetch(context.Background(), server.URL); err == nil {
		t.Fatalf("expected unsupported content type to be rejected")
	}
}

func TestFetcherRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("0123456789abcdef"))
	}))
	defer server.Close()

	fetcher := NewFetcher(&fakeURLValidator{}, WithMaxBytes(8))
	if _, err := fetcher.Fetch(context.Background(), server.URL); err == nil {
		t.Fatalf("expected oversized response to be rejected")
	}
}
