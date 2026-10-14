package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"sync/atomic"
	"time"

	"signalpost/internal/model"
)

var (
	ErrHardCapExceeded = errors.New("outbound request limit hard cap exceeded")
	ErrRequestFailed   = errors.New("request failed after retries")
)

type Response struct {
	StatusCode int
	Body       []byte
	FinalURL   string
	FetchedAt  time.Time
	FromCache  bool
}

type Options struct {
	UserAgent   string
	MaxRequests int64
	Timeout     time.Duration
	CacheDir    string
	Report      *model.RunReport
}

type Client struct {
	httpClient  *http.Client
	cache       *DiskCache
	limiter     *HostLimiter
	hardCap     int64
	reqCounter  atomic.Int64
	userAgent   string
	report      *model.RunReport
	hardCapHit  atomic.Bool
}

func NewClient(opts Options) (*Client, error) {
	if opts.UserAgent == "" {
		opts.UserAgent = "SignalpostAgent/1.0 (+https://github.com/builderr-hackathon; contact: agent@signalpost.local)"
	}
	if opts.MaxRequests <= 0 {
		opts.MaxRequests = 1800
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}

	diskCache, err := NewDiskCache(opts.CacheDir)
	if err != nil {
		return nil, fmt.Errorf("failed to init cache: %w", err)
	}

	httpClient := &http.Client{
		Timeout: opts.Timeout,
	}

	return &Client{
		httpClient: httpClient,
		cache:      diskCache,
		limiter:    NewHostLimiter(),
		hardCap:    opts.MaxRequests,
		userAgent:  opts.UserAgent,
		report:     opts.Report,
	}, nil
}

// RequestCount returns the current count of outbound network requests made.
func (c *Client) RequestCount() int64 {
	return c.reqCounter.Load()
}

// HardCapReached returns whether the hard cap was triggered.
func (c *Client) HardCapReached() bool {
	return c.hardCapHit.Load()
}

// Get performs an HTTP GET with caching, rate limiting, atomic request capping, and retries.
func (c *Client) Get(ctx context.Context, targetURL string, ttl time.Duration) (*Response, error) {
	// 1. Check disk cache first
	if c.cache != nil && ttl > 0 {
		if entry, found := c.cache.Get(targetURL); found {
			if c.report != nil {
				c.report.RecordCacheHit()
			}
			return &Response{
				StatusCode: entry.StatusCode,
				Body:       entry.Body,
				FinalURL:   entry.FinalURL,
				FetchedAt:  entry.FetchedAt,
				FromCache:  true,
			}, nil
		}
	}

	// 2. Parse target URL to extract hostname for rate limiting
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL %q: %w", targetURL, err)
	}
	host := parsed.Host

	// 3. Check hard cap before attempting outbound network request
	if c.reqCounter.Load() >= c.hardCap {
		c.hardCapHit.Store(true)
		if c.report != nil {
			c.report.HardCapReached = true
		}
		return nil, ErrHardCapExceeded
	}

	// 4. Retry loop (up to 2 retries for transient 429 / 5xx)
	maxRetries := 2
	var lastErr error
	var resp *http.Response
	var finalURL string

	for attempt := 0; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Check hard cap again
		currentCount := c.reqCounter.Add(1)
		if currentCount > c.hardCap {
			c.hardCapHit.Store(true)
			if c.report != nil {
				c.report.HardCapReached = true
			}
			return nil, ErrHardCapExceeded
		}

		if c.report != nil {
			c.report.RecordRequest(host)
		}

		// Wait on per-host token bucket limiter
		if err := c.limiter.Wait(ctx, host); err != nil {
			return nil, fmt.Errorf("rate limiter error: %w", err)
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		httpReq.Header.Set("User-Agent", c.userAgent)
		httpReq.Header.Set("Accept", "application/json, text/html, */*")

		fetchedAt := time.Now().UTC()
		resp, err = c.httpClient.Do(httpReq)
		if err != nil {
			lastErr = err
			// Backoff and retry
			if attempt < maxRetries {
				sleepWithJitter(ctx, attempt, 0)
				continue
			}
			break
		}

		finalURL = resp.Request.URL.String()

		// If 429 Too Many Requests or 5xx Server Error, retry
		if resp.StatusCode == http.StatusTooManyRequests || (resp.StatusCode >= 500 && resp.StatusCode <= 599) {
			retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("server returned status %d", resp.StatusCode)
			if attempt < maxRetries {
				sleepWithJitter(ctx, attempt, retryAfter)
				continue
			}
			return nil, fmt.Errorf("%w: status %d", ErrRequestFailed, resp.StatusCode)
		}

		// Read response body
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("failed to read response body: %w", readErr)
		}

		res := &Response{
			StatusCode: resp.StatusCode,
			Body:       body,
			FinalURL:   finalURL,
			FetchedAt:  fetchedAt,
			FromCache:  false,
		}

		// Cache successful responses and authoritative 404/410 registry responses
		if c.cache != nil && ttl > 0 && (resp.StatusCode == 200 || resp.StatusCode == 404 || resp.StatusCode == 410) {
			_ = c.cache.Put(targetURL, resp.StatusCode, body, finalURL, fetchedAt, ttl)
		}

		return res, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("%w: %v", ErrRequestFailed, lastErr)
	}
	return nil, ErrRequestFailed
}

func parseRetryAfter(val string) time.Duration {
	if val == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(val); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if t, err := http.ParseTime(val); err == nil {
		dur := time.Until(t)
		if dur > 0 {
			return dur
		}
	}
	return 0
}

func sleepWithJitter(ctx context.Context, attempt int, minDuration time.Duration) {
	dur := minDuration
	if dur == 0 {
		// base backoff: 300ms, 600ms, 1200ms + jitter
		baseMs := int64(300 * (1 << attempt))
		jitter := rand.Int63n(150)
		dur = time.Duration(baseMs+jitter) * time.Millisecond
	}
	select {
	case <-time.After(dur):
	case <-ctx.Done():
	}
}
