// Package fanout implements concurrent breadth+depth orchestration: a fast recon
// pass emits a structured asset inventory, then one depth agent per asset runs
// under a bounded, rate-limited worker pool, and findings fan in for enrichment
// and correlation (docs/DESIGN-concurrent-orchestration-and-cve-cvss.md Part A).
package fanout

import (
	"encoding/json"
	"regexp"
	"strings"
)

// AssetType classifies an asset so a depth role can be auto-selected.
type AssetType string

const (
	AssetHost  AssetType = "host"
	AssetWeb   AssetType = "web"
	AssetCloud AssetType = "cloud"
	AssetAPI   AssetType = "api"
)

// Asset is a single discovered target for a depth agent.
type Asset struct {
	ID       string            `json:"id"`
	Type     AssetType         `json:"type"`
	Value    string            `json:"value"` // host, URL, ARN, etc.
	Ports    []int             `json:"ports,omitempty"`
	Services []string          `json:"services,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// AssetInventory is the structured output of the recon/breadth pass.
type AssetInventory struct {
	Assets []Asset `json:"assets"`
}

// Normalize fills in missing ids/types and drops empty assets so the dispatcher
// can rely on every asset having an id, type and value.
func (inv *AssetInventory) Normalize() {
	cleaned := inv.Assets[:0]
	for i := range inv.Assets {
		a := inv.Assets[i]
		a.Value = strings.TrimSpace(a.Value)
		if a.Value == "" {
			continue
		}
		if a.Type == "" {
			a.Type = classify(a.Value)
		}
		a.Type = AssetType(strings.ToLower(string(a.Type)))
		if a.ID == "" {
			a.ID = slug(a.Value)
		}
		cleaned = append(cleaned, a)
	}
	inv.Assets = cleaned
}

// Dedupe removes assets with duplicate (type,value) pairs, preserving order.
func (inv *AssetInventory) Dedupe() {
	seen := map[string]bool{}
	out := inv.Assets[:0]
	for _, a := range inv.Assets {
		key := string(a.Type) + "\x1e" + strings.ToLower(a.Value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, a)
	}
	inv.Assets = out
}

var fencedJSON = regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*?\\}|\\[.*?\\])\\s*```")

// ParseInventory extracts an AssetInventory from recon agent output. It accepts,
// in order of preference: a fenced ```json block, a bare top-level JSON object
// with an "assets" array, or a bare JSON array of assets. Returns an empty (not
// nil) inventory when nothing parses, so the caller can fall back to a
// line-based heuristic.
func ParseInventory(text string) AssetInventory {
	var inv AssetInventory

	candidates := []string{}
	for _, m := range fencedJSON.FindAllStringSubmatch(text, -1) {
		if len(m) > 1 {
			candidates = append(candidates, m[1])
		}
	}
	// Also try the largest bare JSON object/array in the text.
	if obj := extractBraced(text, '{', '}'); obj != "" {
		candidates = append(candidates, obj)
	}
	if arr := extractBraced(text, '[', ']'); arr != "" {
		candidates = append(candidates, arr)
	}

	for _, c := range candidates {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if c[0] == '{' {
			var tmp AssetInventory
			if err := json.Unmarshal([]byte(c), &tmp); err == nil && len(tmp.Assets) > 0 {
				inv.Assets = append(inv.Assets, tmp.Assets...)
				continue
			}
		}
		if c[0] == '[' {
			var arr []Asset
			if err := json.Unmarshal([]byte(c), &arr); err == nil && len(arr) > 0 {
				inv.Assets = append(inv.Assets, arr...)
			}
		}
	}

	inv.Normalize()
	inv.Dedupe()
	return inv
}

// FallbackInventory builds an inventory from newline/comma/space-separated target
// tokens (URLs, hosts, IPs) when no structured output is available. This keeps
// fanout usable even if the recon agent returns prose.
func FallbackInventory(text string) AssetInventory {
	var inv AssetInventory
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ' ' || r == '\t' || r == ';'
	})
	for _, f := range fields {
		f = strings.Trim(f, "`'\"()[]<>")
		if looksLikeTarget(f) {
			inv.Assets = append(inv.Assets, Asset{Value: f})
		}
	}
	inv.Normalize()
	inv.Dedupe()
	return inv
}

var (
	urlRe    = regexp.MustCompile(`^https?://`)
	ipRe     = regexp.MustCompile(`^\d{1,3}(\.\d{1,3}){3}(:\d+)?$`)
	domainRe = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}(:\d+)?$`)
	slugRe   = regexp.MustCompile(`[^a-zA-Z0-9]+`)
)

func looksLikeTarget(s string) bool {
	if s == "" || len(s) > 2048 {
		return false
	}
	return urlRe.MatchString(s) || ipRe.MatchString(s) || domainRe.MatchString(s)
}

func classify(value string) AssetType {
	v := strings.ToLower(value)
	switch {
	case strings.Contains(v, "/api") || strings.Contains(v, "graphql") || strings.HasSuffix(v, "/openapi.json") || strings.Contains(v, "swagger"):
		return AssetAPI
	case urlRe.MatchString(v):
		return AssetWeb
	case strings.HasPrefix(v, "arn:") || strings.Contains(v, ".amazonaws.com") || strings.Contains(v, ".blob.core.windows.net") || strings.Contains(v, ".storage.googleapis.com"):
		return AssetCloud
	default:
		return AssetHost
	}
}

func slug(value string) string {
	s := slugRe.ReplaceAllString(strings.ToLower(value), "-")
	s = strings.Trim(s, "-")
	if len(s) > 48 {
		s = s[:48]
	}
	if s == "" {
		return "asset"
	}
	return s
}

// extractBraced returns the substring from the first open rune to the matching
// last close rune (greedy), or "" if not balanced-ish. Good enough to feed the
// JSON decoder, which does the real validation.
func extractBraced(text string, open, close byte) string {
	start := strings.IndexByte(text, open)
	end := strings.LastIndexByte(text, close)
	if start < 0 || end < 0 || end <= start {
		return ""
	}
	return text[start : end+1]
}
