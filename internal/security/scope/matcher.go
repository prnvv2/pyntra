// Package scope enforces that an authorized engagement's target scope is not
// exceeded by the agent's tools. It is deliberately conservative about blocking:
// an empty scope allows everything (pre-engagement behavior), and only a target
// that can be clearly identified AND clearly falls outside scope is denied.
package scope

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Matcher decides whether a target is within an engagement scope.
type Matcher struct {
	domains    []string // lower-case host suffixes (example.com matches sub.example.com)
	cidrs      []*net.IPNet
	urlHosts   []string
	exclusions []string // hosts/domains explicitly out of scope
	empty      bool
}

// NewMatcher builds a Matcher from scope lists. If every list is empty the
// matcher permits all targets.
func NewMatcher(domains, cidrs, urls, exclusions []string) *Matcher {
	m := &Matcher{}
	for _, d := range domains {
		if d = strings.ToLower(strings.TrimSpace(d)); d != "" {
			m.domains = append(m.domains, strings.TrimPrefix(d, "*."))
		}
	}
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if _, n, err := net.ParseCIDR(c); err == nil {
			m.cidrs = append(m.cidrs, n)
		} else if ip := net.ParseIP(c); ip != nil {
			// bare IP -> /32 or /128
			bits := 32
			if ip.To4() == nil {
				bits = 128
			}
			if _, n, err := net.ParseCIDR(fmt.Sprintf("%s/%d", c, bits)); err == nil {
				m.cidrs = append(m.cidrs, n)
			}
		}
	}
	for _, u := range urls {
		if h := hostOf(u); h != "" {
			m.urlHosts = append(m.urlHosts, h)
		}
	}
	for _, x := range exclusions {
		if x = strings.ToLower(strings.TrimSpace(x)); x != "" {
			m.exclusions = append(m.exclusions, strings.TrimPrefix(x, "*."))
		}
	}
	m.empty = len(m.domains) == 0 && len(m.cidrs) == 0 && len(m.urlHosts) == 0
	return m
}

// IsEmpty reports whether the scope permits everything.
func (m *Matcher) IsEmpty() bool { return m == nil || m.empty }

// Allow reports whether a single target (url/host/ip) is in scope.
func (m *Matcher) Allow(target string) (bool, string) {
	if m == nil || m.empty {
		return true, ""
	}
	host := hostOf(target)
	if host == "" {
		// Not a network target we can evaluate (e.g. a local file path) -> allow.
		return true, ""
	}
	// Exclusions win.
	for _, x := range m.exclusions {
		if host == x || strings.HasSuffix(host, "."+x) {
			return false, "target " + host + " is explicitly excluded from scope"
		}
	}
	ip := net.ParseIP(host)
	if ip != nil {
		for _, n := range m.cidrs {
			if n.Contains(ip) {
				return true, ""
			}
		}
	}
	for _, d := range m.domains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true, ""
		}
	}
	for _, h := range m.urlHosts {
		if host == h {
			return true, ""
		}
	}
	return false, "target " + host + " is outside the authorized engagement scope"
}

// AllowAll checks every extracted target, denying if any is out of scope.
func (m *Matcher) AllowAll(targets []string) (bool, string) {
	for _, t := range targets {
		if ok, reason := m.Allow(t); !ok {
			return false, reason
		}
	}
	return true, ""
}

// targetArgKeys are the argument names that commonly carry a target.
var targetArgKeys = []string{"target", "url", "host", "hostname", "domain", "ip", "address", "rhost", "rhosts", "targets", "u", "H"}

// ExtractTargets pulls candidate targets out of tool arguments.
func ExtractTargets(args map[string]interface{}) []string {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		for _, tok := range strings.Fields(strings.ReplaceAll(s, ",", " ")) {
			tok = strings.TrimSpace(tok)
			if tok != "" && !seen[tok] {
				seen[tok] = true
				out = append(out, tok)
			}
		}
	}
	for _, k := range targetArgKeys {
		if v, ok := args[k]; ok {
			if s, ok := v.(string); ok {
				add(s)
			}
		}
	}
	return out
}

// hostOf extracts a bare host from a url, host:port, or bare host/ip string.
func hostOf(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if strings.Contains(s, "://") {
		if u, err := url.Parse(s); err == nil && u.Host != "" {
			s = u.Host
		}
	}
	if h, _, err := net.SplitHostPort(s); err == nil {
		s = h
	}
	s = strings.Trim(s, "[]")
	s = strings.ToLower(s)
	// strip any path remnants
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	return s
}
