// Package cwe maps vulnerability types to CWE identifiers using a seeded,
// file-backed table (docs/DESIGN-concurrent-orchestration-and-cve-cvss.md §B.3).
//
// Precedence in the overall pipeline: structured tool metadata (nuclei cwe-id) >
// this seeded table > LLM-assisted validated lookup. This package implements the
// middle tier and provides CWE-id syntax validation for the last tier.
package cwe

import (
	"os"
	"regexp"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Mapper resolves vulnerability types/titles to CWE identifiers.
type Mapper struct {
	mu    sync.RWMutex
	table map[string][]string
}

type mapFile struct {
	Map map[string][]string `yaml:"map"`
}

var cweIDPattern = regexp.MustCompile(`^CWE-\d+$`)

// New returns an empty mapper (no entries). Use Load to populate it from a file.
func New() *Mapper {
	return &Mapper{table: map[string][]string{}}
}

// Load reads a YAML map file. A missing path is not an error — the mapper simply
// stays empty and the pipeline falls through to its other tiers.
func Load(path string) (*Mapper, error) {
	m := New()
	if strings.TrimSpace(path) == "" {
		return m, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return m, nil
		}
		return m, err
	}
	var mf mapFile
	if err := yaml.Unmarshal(data, &mf); err != nil {
		return m, err
	}
	norm := make(map[string][]string, len(mf.Map))
	for k, v := range mf.Map {
		norm[strings.ToLower(strings.TrimSpace(k))] = NormalizeIDs(v)
	}
	m.mu.Lock()
	m.table = norm
	m.mu.Unlock()
	return m, nil
}

// Lookup resolves CWE ids for a finding, trying an exact type match first, then a
// substring match of each table key against the combined type+title text. Returns
// nil when nothing matches.
func (m *Mapper) Lookup(vulnType, title string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := strings.ToLower(strings.TrimSpace(vulnType))
	if key != "" {
		if ids, ok := m.table[key]; ok {
			return append([]string(nil), ids...)
		}
	}

	hay := strings.ToLower(vulnType + " " + title)
	// Prefer longer keys first so "sql-injection" beats a hypothetical "sql".
	for _, k := range m.keysByLengthDesc() {
		// Match on the human-readable form too (dashes <-> spaces).
		if strings.Contains(hay, k) || strings.Contains(hay, strings.ReplaceAll(k, "-", " ")) {
			return append([]string(nil), m.table[k]...)
		}
	}
	return nil
}

func (m *Mapper) keysByLengthDesc() []string {
	keys := make([]string, 0, len(m.table))
	for k := range m.table {
		keys = append(keys, k)
	}
	// simple length-descending sort (table is small)
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if len(keys[j]) > len(keys[i]) {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

// Size reports the number of loaded entries (for diagnostics/logging).
func (m *Mapper) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.table)
}

// ValidID reports whether s is a syntactically valid CWE identifier (CWE-<n>).
// Used to validate LLM-proposed CWE ids before they are trusted.
func ValidID(s string) bool {
	return cweIDPattern.MatchString(strings.ToUpper(strings.TrimSpace(s)))
}

// NormalizeIDs upper-cases, trims, prefixes bare numbers with "CWE-", and drops
// anything that is not a valid CWE id. Order is preserved and duplicates removed.
func NormalizeIDs(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, raw := range in {
		s := strings.ToUpper(strings.TrimSpace(raw))
		if s == "" {
			continue
		}
		if !strings.HasPrefix(s, "CWE-") {
			if _, err := parseUint(s); err == nil {
				s = "CWE-" + s
			}
		}
		if !ValidID(s) || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func parseUint(s string) (int, error) {
	n := 0
	if s == "" {
		return 0, errEmpty
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errNaN
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

var (
	errEmpty = strErr("empty")
	errNaN   = strErr("not a number")
)

type strErr string

func (e strErr) Error() string { return string(e) }
