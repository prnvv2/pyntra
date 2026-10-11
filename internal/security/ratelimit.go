package security

import (
	"context"
	"net"
	"net/url"
	"strings"
	"sync"

	"golang.org/x/time/rate"
)

// HostRateLimiter enforces a per-host request rate so that concurrent depth
// agents (fanout mode) cannot aggregate into an accidental DoS or trip a WAF
// (docs/DESIGN-concurrent-orchestration-and-cve-cvss.md §A.3). It is keyed by
// host: every worker targeting the same host shares one token bucket.
type HostRateLimiter struct {
	rps   float64
	burst int
	mu    sync.Mutex
	hosts map[string]*rate.Limiter
}

// NewHostRateLimiter builds a limiter at rps requests/second per host. A rps <= 0
// disables limiting (Wait returns immediately). Burst defaults to max(1, ceil(rps)).
func NewHostRateLimiter(rps float64) *HostRateLimiter {
	burst := 1
	if rps > 1 {
		burst = int(rps + 0.999)
	}
	return &HostRateLimiter{
		rps:   rps,
		burst: burst,
		hosts: map[string]*rate.Limiter{},
	}
}

// HostKey extracts a normalized host key from a target string, which may be a
// URL, host:port, bare host or IP. Returns "" when nothing host-like is found.
func HostKey(target string) string {
	t := strings.TrimSpace(target)
	if t == "" {
		return ""
	}
	if strings.Contains(t, "://") {
		if u, err := url.Parse(t); err == nil && u.Host != "" {
			if h, _, err := net.SplitHostPort(u.Host); err == nil {
				return strings.ToLower(h)
			}
			return strings.ToLower(u.Hostname())
		}
	}
	if h, _, err := net.SplitHostPort(t); err == nil {
		return strings.ToLower(h)
	}
	return strings.ToLower(t)
}

// Wait blocks until the host's bucket allows one request, or ctx is done. A zero
// host or disabled limiter returns nil immediately.
func (l *HostRateLimiter) Wait(ctx context.Context, host string) error {
	if l == nil || l.rps <= 0 || host == "" {
		return nil
	}
	return l.limiterFor(host).Wait(ctx)
}

func (l *HostRateLimiter) limiterFor(host string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	lim, ok := l.hosts[host]
	if !ok {
		lim = rate.NewLimiter(rate.Limit(l.rps), l.burst)
		l.hosts[host] = lim
	}
	return lim
}
