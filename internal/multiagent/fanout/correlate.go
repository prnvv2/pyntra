package fanout

import (
	"fmt"
	"sort"
	"strings"

	"pyntra/internal/database"
)

// correlate performs deterministic cross-asset correlation over the findings of a
// run (design P4). It surfaces two signals that are only visible once breadth and
// depth are combined:
//
//  1. Credential/secret exposure on one asset that becomes a spray/reuse
//     opportunity against every other in-scope asset.
//  2. The same weakness class (CWE) appearing on multiple assets, which points to
//     a systemic root cause rather than isolated bugs.
//
// It returns a Markdown section, or "" when there is nothing to correlate.
func correlate(findings []*database.Vulnerability) string {
	if len(findings) < 2 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Cross-asset correlation\n\n")
	wrote := false

	// 1. Credential exposure -> spray candidates.
	credHosts := map[string][]string{} // host -> finding titles
	allHosts := map[string]bool{}
	for _, f := range findings {
		host := targetHost(f.Target)
		if host != "" {
			allHosts[host] = true
		}
		if mentionsCredential(f) && host != "" {
			credHosts[host] = append(credHosts[host], f.Title)
		}
	}
	if len(credHosts) > 0 {
		wrote = true
		b.WriteString("**Credential / secret reuse**\n\n")
		for host, titles := range credHosts {
			var targets []string
			for h := range allHosts {
				if h != host {
					targets = append(targets, h)
				}
			}
			sort.Strings(targets)
			b.WriteString(fmt.Sprintf("- Credentials/secrets exposed on `%s` (%s).", host, strings.Join(dedupeStrings(titles), "; ")))
			if len(targets) > 0 {
				b.WriteString(fmt.Sprintf(" Consider credential-spray / reuse against: %s.", strings.Join(targets, ", ")))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// 2. Shared weakness class across assets.
	cweTargets := map[string]map[string]bool{} // CWE -> set of targets
	for _, f := range findings {
		host := targetHost(f.Target)
		if host == "" {
			continue
		}
		for _, cwe := range f.CWEIDs {
			if cweTargets[cwe] == nil {
				cweTargets[cwe] = map[string]bool{}
			}
			cweTargets[cwe][host] = true
		}
	}
	var systemic []string
	for cwe, hosts := range cweTargets {
		if len(hosts) >= 2 {
			var hs []string
			for h := range hosts {
				hs = append(hs, h)
			}
			sort.Strings(hs)
			systemic = append(systemic, fmt.Sprintf("- `%s` affects %d assets: %s", cwe, len(hs), strings.Join(hs, ", ")))
		}
	}
	if len(systemic) > 0 {
		wrote = true
		sort.Strings(systemic)
		b.WriteString("**Systemic weaknesses (same CWE on multiple assets)**\n\n")
		b.WriteString(strings.Join(systemic, "\n"))
		b.WriteString("\n\n")
	}

	if !wrote {
		return ""
	}
	return b.String()
}

var credentialHints = []string{
	"credential", "password", "passwd", "secret", "api key", "api-key", "apikey",
	"access key", "private key", "token", "aws_secret", "default login",
	"hardcoded", "plaintext password", ".env", "authentication bypass",
}

func mentionsCredential(f *database.Vulnerability) bool {
	hay := strings.ToLower(strings.Join([]string{f.Title, f.Type, f.Description, f.Proof}, " "))
	for _, h := range credentialHints {
		if strings.Contains(hay, h) {
			return true
		}
	}
	return false
}

func targetHost(target string) string {
	t := strings.TrimSpace(target)
	if t == "" {
		return ""
	}
	if strings.Contains(t, "://") {
		if i := strings.Index(t, "://"); i >= 0 {
			t = t[i+3:]
		}
	}
	if i := strings.IndexAny(t, "/?#"); i >= 0 {
		t = t[:i]
	}
	if i := strings.LastIndex(t, ":"); i >= 0 {
		// strip port only if what follows is numeric
		port := t[i+1:]
		if port != "" && isAllDigits(port) {
			t = t[:i]
		}
	}
	return strings.ToLower(strings.TrimSpace(t))
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
