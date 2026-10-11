// Package enrich ties the CVSS calculator, CWE mapper and CVE resolver into a
// single finding-enrichment step (docs/DESIGN-concurrent-orchestration-and-cve-cvss.md
// Part B). It is the first stage of fan-in: raw findings go in, scored and
// prioritized findings come out.
//
// Guiding principle: the CVSS score is ALWAYS computed in code from a vector,
// never a number trusted from a tool or an LLM.
package enrich

import (
	"context"
	"regexp"
	"strings"
	"time"

	"pyntra/internal/config"
	"pyntra/internal/database"
	"pyntra/internal/enrich/cve"
	"pyntra/internal/enrich/cvss"
	"pyntra/internal/enrich/cwe"

	"go.uber.org/zap"
)

// Enricher enriches findings with CVE/CWE/CVSS data. All collaborators are
// optional; a zero-featured Enricher still computes CVSS from any vector it can
// find in the finding text.
type Enricher struct {
	cveClient      *cve.Client
	cweMapper      *cwe.Mapper
	defaultVersion string
	kev            *KEVCatalog
	epss           *EPSSClient
	logger         *zap.Logger
}

// Options configures an Enricher.
type Options struct {
	CVEClient      *cve.Client
	CWEMapper      *cwe.Mapper
	DefaultVersion string // "3.1" | "4.0"
	KEV            *KEVCatalog
	EPSS           *EPSSClient
	Logger         *zap.Logger
}

// New constructs an Enricher.
func New(opts Options) *Enricher {
	ver := opts.DefaultVersion
	if ver == "" {
		ver = "3.1"
	}
	lg := opts.Logger
	if lg == nil {
		lg = zap.NewNop()
	}
	return &Enricher{
		cveClient:      opts.CVEClient,
		cweMapper:      opts.CWEMapper,
		defaultVersion: ver,
		kev:            opts.KEV,
		epss:           opts.EPSS,
		logger:         lg,
	}
}

var vectorPattern = regexp.MustCompile(`CVSS:[34]\.[0-9]/[A-Za-z0-9:/.]+`)

// extractVector returns the first well-formed CVSS vector found in text.
func extractVector(text string) string {
	m := vectorPattern.FindString(text)
	return strings.TrimRight(m, "/ ,;)")
}

// findingText concatenates the human-readable fields of a finding for reference
// extraction.
func findingText(v *database.Vulnerability) string {
	return strings.Join([]string{v.Type, v.Title, v.Description, v.Proof, v.Recommendation}, "\n")
}

// Enrich fills in the CVE/CWE/CVSS fields of a finding in place. It never returns
// a hard error for an individual finding — enrichment is best-effort and degrades
// gracefully (offline, no CVE, etc.); problems are logged and the finding is left
// as scored-as-far-as-possible. The returned error is reserved for programmer
// errors (nil finding).
func (e *Enricher) Enrich(ctx context.Context, v *database.Vulnerability) error {
	if v == nil {
		return nil
	}
	text := findingText(v)

	// 1. Direct references already in the finding text (highest confidence:
	//    structured tool metadata such as nuclei classification).
	cveIDs := mergeUnique(v.CVEIDs, cve.ExtractCVEIDs(text))
	directVector := v.CVSSVector
	if directVector == "" {
		directVector = extractVector(text)
	}

	// 2. Resolve each CVE (cache -> NVD/mirror) to obtain an authoritative vector
	//    and CWE list. The first CVE that yields a vector seeds the score.
	var cweIDs []string = append([]string(nil), v.CWEIDs...)
	if e.cveClient != nil {
		for _, id := range cveIDs {
			rec, err := e.cveClient.Lookup(ctx, id)
			if err != nil {
				e.logger.Debug("cve lookup failed", zap.String("cve", id), zap.Error(err))
				continue
			}
			if rec == nil {
				continue
			}
			if directVector == "" && rec.CVSSVector != "" {
				directVector = rec.CVSSVector
			}
			cweIDs = mergeUnique(cweIDs, rec.CWEIDs)
		}
	}

	// 3. CWE: seeded type->CWE table as a fallback when no CWE came from tools/CVEs.
	if len(cweIDs) == 0 && e.cweMapper != nil {
		if mapped := e.cweMapper.Lookup(v.Type, v.Title); len(mapped) > 0 {
			cweIDs = mapped
		}
	}

	// 4. Compute CVSS deterministically from whatever vector we have.
	if directVector != "" {
		if parsed, err := cvss.ParseVector(directVector); err == nil {
			score, band := cvss.BaseScore(parsed)
			v.CVSSVector = parsed.Raw
			v.CVSSVersion = string(parsed.Version)
			v.CVSSBaseScore = score
			v.CVSSSeverity = string(band)
		} else {
			e.logger.Debug("invalid CVSS vector in finding", zap.String("vector", directVector), zap.Error(err))
		}
	}

	v.CVEIDs = cveIDs
	v.CWEIDs = cwe.NormalizeIDs(cweIDs)

	// 5. Optional prioritization signals.
	if e.kev != nil && len(cveIDs) > 0 {
		for _, id := range cveIDs {
			if e.kev.IsKnown(ctx, id) {
				v.KEV = true
				break
			}
		}
	}
	if e.epss != nil && len(cveIDs) > 0 {
		if s, p, ok := e.epss.Score(ctx, cveIDs[0]); ok {
			v.EPSSScore = s
			v.EPSSPercentile = p
		}
	}

	return nil
}

// ScoreFromMetrics builds a vector from agent-proposed base metrics, validates
// it, and computes the score — the "no published CVE" path (design §B.1/B.3). It
// returns the canonical vector, score and band, or an error if the metrics are
// invalid. The score is computed, never taken from the caller.
func (e *Enricher) ScoreFromMetrics(av, ac, pr, ui, scope, c, i, a string) (string, float64, cvss.Severity, error) {
	vector := strings.Join([]string{
		"CVSS:" + e.defaultVersionPrefix(),
		"AV:" + strings.ToUpper(av),
		"AC:" + strings.ToUpper(ac),
		"PR:" + strings.ToUpper(pr),
		"UI:" + strings.ToUpper(ui),
		"S:" + strings.ToUpper(scope),
		"C:" + strings.ToUpper(c),
		"I:" + strings.ToUpper(i),
		"A:" + strings.ToUpper(a),
	}, "/")
	parsed, err := cvss.ParseVector(vector)
	if err != nil {
		return "", 0, cvss.SeverityNone, err
	}
	score, band := cvss.BaseScore(parsed)
	return parsed.Raw, score, band, nil
}

func (e *Enricher) defaultVersionPrefix() string {
	if e.defaultVersion == "4.0" {
		return "4.0"
	}
	return "3.1"
}

// mergeUnique appends b into a, preserving order and dropping case-insensitive
// duplicates (ids are upper-cased).
func mergeUnique(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range append(append([]string(nil), a...), b...) {
		u := strings.ToUpper(strings.TrimSpace(s))
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	return out
}

// NewFromConfig wires an Enricher from configuration and a DB-backed CVE cache.
// It always returns a usable Enricher: even with CVE lookups off, a CVSS-only
// enricher still scores findings from any vector present in their text.
func NewFromConfig(cfg config.EnrichmentConfig, cache cve.Cache, logger *zap.Logger) *Enricher {
	mapper, err := cwe.Load(cfg.CWE.MapFile)
	if err != nil && logger != nil {
		logger.Warn("failed to load CWE map", zap.String("file", cfg.CWE.MapFile), zap.Error(err))
	}

	var cveClient *cve.Client
	source := cfg.CVESourceEffective()
	if source != "off" {
		cveClient = cve.New(cve.Options{
			Source:     cve.Source(source),
			APIKey:     cfg.CVE.NVDAPIKey,
			MirrorPath: cfg.CVE.LocalMirrorPath,
			Cache:      cache,
		})
	}

	var kev *KEVCatalog
	if cfg.KEV.Enabled {
		kev = NewKEVCatalog(nil)
	}
	var epss *EPSSClient
	if cfg.EPSS.Enabled {
		epss = NewEPSSClient(nil)
	}

	return New(Options{
		CVEClient:      cveClient,
		CWEMapper:      mapper,
		DefaultVersion: cfg.CVSSDefaultVersionEffective(),
		KEV:            kev,
		EPSS:           epss,
		Logger:         logger,
	})
}

// DBCacheAdapter adapts the database CVE cache to the cve.Cache interface. TTL
// controls freshness.
type DBCacheAdapter struct {
	DB  *database.DB
	TTL time.Duration
}

// Get implements cve.Cache.
func (a DBCacheAdapter) Get(cveID string) (*cve.Record, bool) {
	entry, err := a.DB.GetCVECache(cveID)
	if err != nil || entry == nil {
		return nil, false
	}
	if a.TTL > 0 && !entry.Fresh(a.TTL) {
		return nil, false
	}
	return &cve.Record{
		CVEID:       entry.CVEID,
		Description: entry.Description,
		CVSSVersion: entry.CVSSVersion,
		CVSSVector:  entry.CVSSVector,
		CVSSScore:   entry.CVSSScore,
		CWEIDs:      entry.CWEIDs,
		Published:   entry.Published,
		Modified:    entry.Modified,
		Source:      entry.Source,
	}, true
}

// Put implements cve.Cache.
func (a DBCacheAdapter) Put(r *cve.Record) {
	if r == nil {
		return
	}
	_ = a.DB.UpsertCVECache(&database.CVECacheEntry{
		CVEID:       r.CVEID,
		Description: r.Description,
		CVSSVersion: r.CVSSVersion,
		CVSSVector:  r.CVSSVector,
		CVSSScore:   r.CVSSScore,
		CWEIDs:      r.CWEIDs,
		Published:   r.Published,
		Modified:    r.Modified,
		Source:      r.Source,
	})
}
