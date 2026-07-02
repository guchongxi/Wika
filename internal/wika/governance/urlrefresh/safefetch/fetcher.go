package safefetch

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const defaultMaxBytes int64 = 2 * 1024 * 1024

type URLValidator interface {
	ValidateURL(ctx context.Context, raw string) (*url.URL, error)
}

type FetchTargetValidator interface {
	ValidateFetchTarget(ctx context.Context, raw string) (*FetchTarget, error)
}

type FetchResult struct {
	Text        string
	ContentType string
	FinalURL    string
	SizeBytes   int64
}

type Fetcher struct {
	validator   URLValidator
	maxBytes    int64
	timeout     time.Duration
	dialContext func(context.Context, string, string) (net.Conn, error)
}

type FetcherOption func(*Fetcher)

func WithMaxBytes(maxBytes int64) FetcherOption {
	return func(f *Fetcher) {
		if maxBytes > 0 {
			f.maxBytes = maxBytes
		}
	}
}

func WithDialContext(dialContext func(context.Context, string, string) (net.Conn, error)) FetcherOption {
	return func(f *Fetcher) {
		if dialContext != nil {
			f.dialContext = dialContext
		}
	}
}

func NewFetcher(validator URLValidator, opts ...FetcherOption) *Fetcher {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	f := &Fetcher{
		validator:   validator,
		maxBytes:    defaultMaxBytes,
		timeout:     30 * time.Second,
		dialContext: dialer.DialContext,
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

func (f *Fetcher) ValidateURL(ctx context.Context, raw string) error {
	_, err := f.validateFetchTarget(ctx, raw)
	return err
}

func (f *Fetcher) Fetch(ctx context.Context, raw string) (*FetchResult, error) {
	if f.validator == nil {
		return nil, fmt.Errorf("url validator is required")
	}
	target, err := f.validateFetchTarget(ctx, raw)
	if err != nil {
		return nil, err
	}
	pins := newPinnedTargets()
	pins.add(target)

	client := f.httpClient(pins)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.URL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch url: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	contentType := resp.Header.Get("Content-Type")
	if !isAllowedContentType(contentType) {
		return nil, fmt.Errorf("unsupported content type")
	}
	if resp.ContentLength > f.maxBytes {
		return nil, fmt.Errorf("response body is too large")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, f.maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if int64(len(body)) > f.maxBytes {
		return nil, fmt.Errorf("response body is too large")
	}
	return &FetchResult{
		Text:        string(body),
		ContentType: contentType,
		FinalURL:    resp.Request.URL.String(),
		SizeBytes:   int64(len(body)),
	}, nil
}

func (f *Fetcher) validateFetchTarget(ctx context.Context, raw string) (*FetchTarget, error) {
	if validator, ok := f.validator.(FetchTargetValidator); ok {
		return validator.ValidateFetchTarget(ctx, raw)
	}
	checkedURL, err := f.validator.ValidateURL(ctx, raw)
	if err != nil {
		return nil, err
	}
	return &FetchTarget{URL: checkedURL}, nil
}

func (f *Fetcher) httpClient(pins *pinnedTargets) *http.Client {
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           f.pinnedDialContext(pins),
		DisableKeepAlives:     true,
		ResponseHeaderTimeout: 10 * time.Second,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   f.timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			target, err := f.validateFetchTarget(req.Context(), req.URL.String())
			if err != nil {
				return err
			}
			pins.add(target)
			req.URL = target.URL
			return nil
		},
	}
}

func (f *Fetcher) pinnedDialContext(pins *pinnedTargets) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network string, addr string) (net.Conn, error) {
		ips := pins.get(addr)
		if len(ips) == 0 {
			return f.dialContext(ctx, network, addr)
		}
		_, port, err := net.SplitHostPort(addr)
		if err != nil {
			return f.dialContext(ctx, network, addr)
		}
		var lastErr error
		for _, ip := range ips {
			conn, err := f.dialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		return nil, lastErr
	}
}

type pinnedTargets struct {
	mu  sync.RWMutex
	ips map[string][]net.IP
}

func newPinnedTargets() *pinnedTargets {
	return &pinnedTargets{ips: map[string][]net.IP{}}
}

func (p *pinnedTargets) add(target *FetchTarget) {
	if target == nil || target.URL == nil || len(target.IPs) == 0 {
		return
	}
	addr := dialAddress(target.URL)
	if addr == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ips[addr] = append([]net.IP(nil), target.IPs...)
}

func (p *pinnedTargets) get(addr string) []net.IP {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return append([]net.IP(nil), p.ips[addr]...)
}

func dialAddress(urlValue *url.URL) string {
	host := urlValue.Hostname()
	if host == "" {
		return ""
	}
	port := urlValue.Port()
	if port == "" {
		switch urlValue.Scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		default:
			return ""
		}
	}
	return net.JoinHostPort(host, port)
}

func isAllowedContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	switch mediaType {
	case "text/plain", "text/html", "text/markdown", "application/json", "application/xml", "application/xhtml+xml":
		return true
	default:
		return false
	}
}
