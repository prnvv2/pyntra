package enrich

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

// KEVCatalog is an in-memory cache of the CISA Known Exploited Vulnerabilities
// catalog. It fetches the published JSON once, then answers membership queries
// offline. Fetch failures degrade gracefully (membership returns false).
type KEVCatalog struct {
	http     *http.Client
	url      string
	ttl      time.Duration
	mu       sync.RWMutex
	known    map[string]bool
	fetched  time.Time
	fetching sync.Mutex
}

const defaultKEVURL = "https://www.cisa.gov/sites/default/files/feeds/known_exploited_vulnerabilities.json"

// NewKEVCatalog builds a catalog client. A nil HTTP client gets a 20s default.
func NewKEVCatalog(hc *http.Client) *KEVCatalog {
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	return &KEVCatalog{
		http:  hc,
		url:   defaultKEVURL,
		ttl:   24 * time.Hour,
		known: map[string]bool{},
	}
}

type kevFeed struct {
	Vulnerabilities []struct {
		CVEID string `json:"cveID"`
	} `json:"vulnerabilities"`
}

// IsKnown reports whether a CVE is in the KEV catalog, lazily (re)fetching when
// the cache is empty or stale.
func (k *KEVCatalog) IsKnown(ctx context.Context, cveID string) bool {
	if k == nil {
		return false
	}
	k.ensure(ctx)
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.known[strings.ToUpper(strings.TrimSpace(cveID))]
}

func (k *KEVCatalog) ensure(ctx context.Context) {
	k.mu.RLock()
	fresh := len(k.known) > 0 && time.Since(k.fetched) < k.ttl
	k.mu.RUnlock()
	if fresh {
		return
	}

	// Single-flight the refresh so concurrent workers don't stampede NVD/CISA.
	k.fetching.Lock()
	defer k.fetching.Unlock()
	k.mu.RLock()
	fresh = len(k.known) > 0 && time.Since(k.fetched) < k.ttl
	k.mu.RUnlock()
	if fresh {
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, k.url, nil)
	if err != nil {
		return
	}
	resp, err := k.http.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	var feed kevFeed
	if err := json.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return
	}
	set := make(map[string]bool, len(feed.Vulnerabilities))
	for _, v := range feed.Vulnerabilities {
		set[strings.ToUpper(strings.TrimSpace(v.CVEID))] = true
	}
	if len(set) == 0 {
		return
	}
	k.mu.Lock()
	k.known = set
	k.fetched = time.Now()
	k.mu.Unlock()
}
