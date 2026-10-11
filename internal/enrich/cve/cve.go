// Package cve resolves CVE identifiers to their CVSS vector, CWE ids and
// metadata, from the NVD REST API 2.0 or a local NVD JSON mirror, backed by a
// pluggable cache (docs/DESIGN-concurrent-orchestration-and-cve-cvss.md §B.5).
//
// It is deliberately independent of the database package: callers inject a Cache
// implementation. Outbound requests are HTTPS-only.
package cve

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Record is a resolved CVE. Times are best-effort and may be zero.
type Record struct {
	CVEID       string
	Description string
	CVSSVersion string
	CVSSVector  string
	CVSSScore   float64
	CWEIDs      []string
	Published   time.Time
	Modified    time.Time
	Source      string // nvd | local_mirror
}

// Cache abstracts the persistence layer (the cve_cache table). A miss returns
// (nil, false). Fresh records short-circuit the network.
type Cache interface {
	Get(cveID string) (*Record, bool)
	Put(r *Record)
}

// Source selects where uncached lookups go.
type Source string

const (
	SourceNVD         Source = "nvd"
	SourceLocalMirror Source = "local_mirror"
	SourceOff         Source = "off"
)

// Client resolves CVEs. The zero value is not usable; use New.
type Client struct {
	source     Source
	apiKey     string
	mirrorPath string
	cache      Cache
	http       *http.Client
}

// Options configures a Client.
type Options struct {
	Source     Source
	APIKey     string
	MirrorPath string
	Cache      Cache
	HTTPClient *http.Client
}

// New builds a Client. A nil HTTPClient gets a 20s-timeout default.
func New(opts Options) *Client {
	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	src := opts.Source
	if src == "" {
		src = SourceOff
	}
	return &Client{
		source:     src,
		apiKey:     opts.APIKey,
		mirrorPath: opts.MirrorPath,
		cache:      opts.Cache,
		http:       hc,
	}
}

var cveIDPattern = regexp.MustCompile(`(?i)CVE-\d{4}-\d{4,}`)

// ExtractCVEIDs pulls distinct, upper-cased CVE identifiers from free text.
func ExtractCVEIDs(text string) []string {
	matches := cveIDPattern.FindAllString(text, -1)
	seen := map[string]bool{}
	var out []string
	for _, m := range matches {
		u := strings.ToUpper(m)
		if !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}
	return out
}

// Lookup resolves a single CVE. Cache is consulted first; on a miss it queries
// the configured source and populates the cache. When the source is "off" it
// returns (nil, nil) — not an error — so enrichment degrades gracefully.
func (c *Client) Lookup(ctx context.Context, cveID string) (*Record, error) {
	cveID = strings.ToUpper(strings.TrimSpace(cveID))
	if !cveIDPattern.MatchString(cveID) {
		return nil, fmt.Errorf("invalid CVE id %q", cveID)
	}

	if c.cache != nil {
		if r, ok := c.cache.Get(cveID); ok && r != nil {
			return r, nil
		}
	}

	var (
		rec *Record
		err error
	)
	switch c.source {
	case SourceNVD:
		rec, err = c.fetchNVD(ctx, cveID)
	case SourceLocalMirror:
		rec, err = c.fetchMirror(cveID)
	default: // off
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if rec != nil && c.cache != nil {
		c.cache.Put(rec)
	}
	return rec, nil
}

// --- NVD REST API 2.0 ---

func (c *Client) fetchNVD(ctx context.Context, cveID string) (*Record, error) {
	endpoint := "https://services.nvd.nist.gov/rest/json/cves/2.0"
	u, _ := url.Parse(endpoint)
	if u.Scheme != "https" {
		return nil, fmt.Errorf("refusing non-HTTPS NVD endpoint")
	}
	q := u.Query()
	q.Set("cveId", cveID)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		req.Header.Set("apiKey", c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("NVD request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("NVD returned status %d for %s", resp.StatusCode, cveID)
	}

	var payload nvdResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode NVD response: %w", err)
	}
	if len(payload.Vulnerabilities) == 0 {
		return nil, nil // not found; not an error
	}
	rec := parseNVDCVE(payload.Vulnerabilities[0].CVE)
	rec.Source = string(SourceNVD)
	return rec, nil
}

// fetchMirror reads <mirror>/<CVE-ID>.json, which must contain a single NVD
// "cve" object (same shape as the API's cve field). Supports air-gapped runs.
func (c *Client) fetchMirror(cveID string) (*Record, error) {
	if c.mirrorPath == "" {
		return nil, fmt.Errorf("local_mirror source selected but local_mirror_path is empty")
	}
	path := filepath.Join(c.mirrorPath, cveID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // not in mirror; degrade gracefully
		}
		return nil, err
	}
	var cveObj nvdCVE
	if err := json.Unmarshal(data, &cveObj); err != nil {
		// Also tolerate a full API-shaped file.
		var full nvdResponse
		if err2 := json.Unmarshal(data, &full); err2 == nil && len(full.Vulnerabilities) > 0 {
			rec := parseNVDCVE(full.Vulnerabilities[0].CVE)
			rec.Source = string(SourceLocalMirror)
			return rec, nil
		}
		return nil, fmt.Errorf("parse mirror file %s: %w", path, err)
	}
	rec := parseNVDCVE(cveObj)
	rec.Source = string(SourceLocalMirror)
	return rec, nil
}
