package safefetch

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"time"
)

const defaultMaxBytes int64 = 2 * 1024 * 1024

type URLValidator interface {
	ValidateURL(ctx context.Context, raw string) (*url.URL, error)
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

func (f *Fetcher) Fetch(ctx context.Context, raw string) (*FetchResult, error) {
	if f.validator == nil {
		return nil, fmt.Errorf("url validator is required")
	}
	checkedURL, err := f.validator.ValidateURL(ctx, raw)
	if err != nil {
		return nil, err
	}

	client := f.httpClient()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checkedURL.String(), nil)
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

func (f *Fetcher) httpClient() *http.Client {
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           f.dialContext,
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
			_, err := f.validator.ValidateURL(req.Context(), req.URL.String())
			return err
		},
	}
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
