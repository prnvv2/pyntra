package config

import (
	"net"
	"net/url"
	"os"
	"strings"
)

// IsLocalHost reports whether host is loopback, a private/link-local address, or
// a local-only name suffix (.local/.localhost/.test). These are allowed over
// plain http because traffic does not leave the machine/network.
func IsLocalHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "" {
		return false
	}
	// Strip a trailing :port if present (handles IPv6 in brackets too).
	if hostOnly, _, err := net.SplitHostPort(h); err == nil {
		h = hostOnly
	}
	h = strings.Trim(h, "[]")
	if h == "localhost" ||
		strings.HasSuffix(h, ".localhost") ||
		strings.HasSuffix(h, ".local") ||
		strings.HasSuffix(h, ".test") {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
	}
	return false
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// IsInsecureRemoteBaseURL reports whether raw is an http:// (non-TLS) endpoint
// pointing at a NON-local host. Sending an API key to such a host would transmit
// it in cleartext, so callers should refuse it. Empty, https, or local-host URLs
// return false.
func IsInsecureRemoteBaseURL(raw string) bool {
	s := strings.TrimSpace(raw)
	if s == "" {
		return false
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return false
	}
	if strings.EqualFold(u.Scheme, "http") {
		return !IsLocalHost(u.Host)
	}
	return false
}

// expandEnvRef expands a value that references an environment variable, e.g.
// "${OPENAI_API_KEY}" or "$OPENAI_API_KEY". Values without a leading "$" are
// returned unchanged so ordinary secrets are never altered.
func expandEnvRef(v string) string {
	s := strings.TrimSpace(v)
	if s == "" || !strings.HasPrefix(s, "$") {
		return v
	}
	return os.Expand(s, os.Getenv)
}

// expandEnvSecrets expands ${ENV} references in credential/endpoint fields so
// secrets can live in the environment instead of config.yaml.
func expandEnvSecrets(cfg *Config) {
	if cfg == nil {
		return
	}
	cfg.OpenAI.APIKey = expandEnvRef(cfg.OpenAI.APIKey)
	cfg.OpenAI.BaseURL = expandEnvRef(cfg.OpenAI.BaseURL)
	cfg.Auth.Password = expandEnvRef(cfg.Auth.Password)
	cfg.Knowledge.Embedding.APIKey = expandEnvRef(cfg.Knowledge.Embedding.APIKey)
	cfg.Knowledge.Embedding.BaseURL = expandEnvRef(cfg.Knowledge.Embedding.BaseURL)
}
