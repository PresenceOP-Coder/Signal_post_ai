package fetch

import (
	"context"
	"strings"
	"sync"

	"golang.org/x/time/rate"
)

// HostLimiter manages per-host rate limiters using token buckets.
type HostLimiter struct {
	mu           sync.RWMutex
	limiters     map[string]*rate.Limiter
	defaultRate  rate.Limit
	defaultBurst int
	specialRates map[string]rate.Limit
}

// NewHostLimiter initializes a rate limiter with default and known host rates.
func NewHostLimiter() *HostLimiter {
	return &HostLimiter{
		limiters:     make(map[string]*rate.Limiter),
		defaultRate:  rate.Limit(2.0), // 2 req/sec for unknown domains
		defaultBurst: 4,
		specialRates: map[string]rate.Limit{
			"data.brreg.no": rate.Limit(8.0), // 8 req/sec for official brreg API
		},
	}
}

// getLimiter returns or creates a rate limiter for the given host.
func (hl *HostLimiter) getLimiter(host string) *rate.Limiter {
	host = strings.ToLower(strings.TrimSpace(host))
	// strip port if present
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}

	hl.mu.RLock()
	lim, exists := hl.limiters[host]
	hl.mu.RUnlock()
	if exists {
		return lim
	}

	hl.mu.Lock()
	defer hl.mu.Unlock()
	if lim, exists := hl.limiters[host]; exists {
		return lim
	}

	r := hl.defaultRate
	burst := hl.defaultBurst
	if special, ok := hl.specialRates[host]; ok {
		r = special
		burst = int(special * 1.5)
		if burst < 5 {
			burst = 5
		}
	}

	lim = rate.NewLimiter(r, burst)
	hl.limiters[host] = lim
	return lim
}

// Wait blocks until the host limiter permits an outbound request or ctx is cancelled.
func (hl *HostLimiter) Wait(ctx context.Context, host string) error {
	lim := hl.getLimiter(host)
	return lim.Wait(ctx)
}
